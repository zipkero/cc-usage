package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// transcriptUsage는 assistant entry의 message.usage 필드 중 cc-usage가 소비하는
// 4개 토큰 카테고리를 보유한다.
// cache_creation의 ephemeral 분기(5m/1h)는 단가표 적용(task-005)에서 별도 단가가
// 필요하므로 여기서 분리 보존한다.
type transcriptUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"` // 5m + 1h 합산 (fallback)

	// ephemeral 분리 보존 — task-005 estimateCost 입력
	CacheCreation5mTokens int `json:"cache_creation_5m_tokens"`
	CacheCreation1hTokens int `json:"cache_creation_1h_tokens"`
}

// transcriptEntry는 tail read로 추출한 assistant entry의 최소 필드 집합이다.
// 파싱 표면을 최소화하기 위해 model·usage·cwd만 보유한다.
type transcriptEntry struct {
	Model string
	Usage transcriptUsage
	Cwd   string
}

// rawTranscriptEntry는 jsonl line에서 필요한 필드만 추출하는 파싱 전용 구조체다.
type rawTranscriptEntry struct {
	Type    string `json:"type"`
	Cwd     string `json:"cwd"`
	Message struct {
		Model string `json:"model"`
		Usage struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheCreation            struct {
				Ephemeral5mInputTokens int `json:"ephemeral_5m_input_tokens"`
				Ephemeral1hInputTokens int `json:"ephemeral_1h_input_tokens"`
			} `json:"cache_creation"`
		} `json:"usage"`
	} `json:"message"`
}

// readLastAssistantEntry는 path의 jsonl 파일에서 마지막 assistant entry를 읽어
// transcriptEntry로 반환한다.
//
// 파일 끝에서 initialWindow 바이트만 읽고, 그 안에 완전한 assistant entry가 없으면
// window를 2배로 늘려 다시 읽되 maxWindow 상한을 넘지 않는다.
// 상한 내 못 찾으면 nil을 반환한다. 파일이 window보다 작으면 전체를 읽는다.
//
// 역방향 line scan으로 type=="assistant"인 첫 매칭을 찾는다.
// tail 잘림으로 맨 앞 line이 partial일 수 있으므로 JSON 파싱 실패 시 해당 line을 skip한다.
// 파일 끝에서 append-only write가 진행 중일 수 있으므로 맨 끝 line도 partial로 간주해 skip한다.
func readLastAssistantEntry(path string, initialWindow, maxWindow int) (*transcriptEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	fileSize := fi.Size()

	for window := initialWindow; ; window *= 2 {
		if window > maxWindow {
			window = maxWindow
		}

		offset := int64(0)
		readSize := fileSize
		if int64(window) < fileSize {
			offset = fileSize - int64(window)
			readSize = int64(window)
		}

		buf := make([]byte, readSize)
		n, err := f.ReadAt(buf, offset)
		if err != nil && err != io.EOF {
			return nil, err
		}
		buf = buf[:n]

		entry := scanLastAssistantEntry(buf, offset > 0)
		if entry != nil {
			return entry, nil
		}

		// window가 이미 파일 전체를 커버했다면 더 이상 확장해도 의미 없음
		if int64(window) >= fileSize {
			return nil, nil
		}

		// 상한에 도달했으면 포기
		if window >= maxWindow {
			return nil, nil
		}
	}
}

// scanLastAssistantEntry는 buf(jsonl 데이터)를 역방향으로 스캔해 마지막
// type=="assistant" entry를 반환한다.
//
// hasPrecedingData가 true이면 첫 번째 line은 partial일 수 있으므로 skip한다.
// 마지막 line도 append-while-write로 partial일 수 있으므로 skip한다.
func scanLastAssistantEntry(buf []byte, hasPrecedingData bool) *transcriptEntry {
	lines := splitLines(buf)
	if len(lines) == 0 {
		return nil
	}

	// 마지막 line skip (append-while-write partial 가능)
	end := len(lines) - 1

	// hasPrecedingData이면 첫 line skip (tail 잘림으로 partial 가능)
	start := 0
	if hasPrecedingData {
		start = 1
	}

	// 역방향 scan: end-1 부터 start까지
	for i := end - 1; i >= start; i-- {
		line := bytes.TrimSpace(lines[i])
		if len(line) == 0 {
			continue
		}
		var raw rawTranscriptEntry
		if err := json.Unmarshal(line, &raw); err != nil {
			// JSON 파싱 실패 = partial line 또는 손상 → skip
			continue
		}
		if raw.Type != "assistant" {
			continue
		}
		if raw.Message.Model == "" {
			continue
		}

		u := raw.Message.Usage
		// ephemeral 5m/1h가 있으면 분리 보존, 없으면 cache_creation_input_tokens를 그대로 사용
		c5m := u.CacheCreation.Ephemeral5mInputTokens
		c1h := u.CacheCreation.Ephemeral1hInputTokens

		// ephemeral 합산이 top-level cache_creation_input_tokens와 다를 경우를 대비해
		// 합산값이 0이면 top-level을 5m에 귀속시킨다 (단가표 미분리 케이스 폴백)
		if c5m == 0 && c1h == 0 && u.CacheCreationInputTokens > 0 {
			c5m = u.CacheCreationInputTokens
		}

		return &transcriptEntry{
			Model: raw.Message.Model,
			Cwd:   raw.Cwd,
			Usage: transcriptUsage{
				InputTokens:              u.InputTokens,
				OutputTokens:             u.OutputTokens,
				CacheReadInputTokens:     u.CacheReadInputTokens,
				CacheCreationInputTokens: u.CacheCreationInputTokens,
				CacheCreation5mTokens:    c5m,
				CacheCreation1hTokens:    c1h,
			},
		}
	}
	return nil
}

// splitLines는 buf를 '\n' 기준으로 분리한 slice를 반환한다.
// 빈 trailing newline으로 생기는 빈 마지막 원소를 포함한 채 반환한다
// (마지막 partial line 판단에 사용).
func splitLines(buf []byte) [][]byte {
	if len(buf) == 0 {
		return nil
	}
	return bytes.Split(buf, []byte("\n"))
}

// selectTranscriptCandidate는 dir 디렉토리 안에서 *.jsonl 파일 중 가장 최근 mtime을
// 가진 파일의 전체 경로를 반환한다 (D3 규칙: newest mtime 우선, mtime 동률 시 lex sort).
//
// 디렉토리 부재, 읽기 실패, 파일 0개인 경우는 빈 문자열과 에러를 반환한다.
// 반환되는 경로는 dir + filename 형태의 절대 경로다.
func selectTranscriptCandidate(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}

	type candidate struct {
		name    string
		modTime int64 // UnixNano
	}

	var candidates []candidate
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		candidates = append(candidates, candidate{
			name:    e.Name(),
			modTime: info.ModTime().UnixNano(),
		})
	}

	if len(candidates) == 0 {
		return "", errors.New("no jsonl files found in " + dir)
	}

	// newest mtime 우선, 동률 시 lex sort(사전순 앞선 파일명 우선)
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].modTime != candidates[j].modTime {
			return candidates[i].modTime > candidates[j].modTime
		}
		return candidates[i].name < candidates[j].name
	})

	return filepath.Join(dir, candidates[0].name), nil
}

// encodeCwdToTranscriptDir는 cwd 문자열을 ~/.claude/projects/<encoded> 형태의
// 전체 경로로 변환한다. '/', '\', ':', '.' 네 구분자를 모두 '-'로 치환하는
// forward-only 함수이며 디코딩 경로는 두지 않는다.
func encodeCwdToTranscriptDir(home, cwd string) string {
	replacer := strings.NewReplacer(
		"/", "-",
		`\`, "-",
		":", "-",
		".", "-",
	)
	encoded := replacer.Replace(cwd)
	return filepath.Join(home, ".claude", "projects", encoded)
}

// needsTranscriptBackfill은 Layer 2(transcript backfill)를 발동할지 여부를 반환한다.
// model(ID/DisplayName 둘 다 빔) 또는 ContextWindowSize<=0이면 true.
// Layer 1 이후에도 여전히 비어있을 때만 Layer 2가 발동한다(중복 작업 방지).
func needsTranscriptBackfill(stdin StdinInput) bool {
	modelEmpty := stdin.Model.ID == "" && stdin.Model.DisplayName == ""
	contextEmpty := stdin.ContextWindow.ContextWindowSize <= 0
	return modelEmpty || contextEmpty
}

// applyTranscriptToStdin은 transcript entry의 model·usage·cost를 stdin에 backfill한다.
// 빈 필드만 채우며(field-local emptiness 룰) fresh stdin 값을 덮어쓰지 않는다.
//
// oneMSignal이 true이면 ContextWindowSize를 1M(1_048_576)으로, 아니면 모델 기본
// 200K(200_000)로 설정한다.
//
// cost는 단가표 lookup 성공 시 estimateCost로 채운다. lookup 실패 시 cost를 채우지
// 않고 mask.Cost=false를 반환한다 — 위젯 skip 정책(D6, D11)에 따라 cost 위젯은
// ctx.CostEstimated=true + cost<=0 조합으로 자동 skip된다.
//
// 채운 필드를 restoredFieldMask로 반환한다.
func applyTranscriptToStdin(stdin *StdinInput, entry *transcriptEntry, oneMSignal bool) restoredFieldMask {
	var mask restoredFieldMask
	if stdin == nil || entry == nil {
		return mask
	}

	// Model 채우기: 빈 경우만
	if stdin.Model.ID == "" && stdin.Model.DisplayName == "" && entry.Model != "" {
		stdin.Model.ID = entry.Model
		mask.Model = true
		debugLog("transcript", "filled Model.ID from transcript: %s", entry.Model)
	}

	// ContextWindow 채우기: ContextWindowSize가 0 이하인 경우만
	if stdin.ContextWindow.ContextWindowSize <= 0 {
		const oneM = 1_048_576
		const twoHundredK = 200_000
		if oneMSignal {
			stdin.ContextWindow.ContextWindowSize = oneM
		} else {
			stdin.ContextWindow.ContextWindowSize = twoHundredK
		}
		// usage 토큰 매핑: transcript usage → stdin.ContextWindow
		stdin.ContextWindow.TotalInputTokens = entry.Usage.InputTokens
		stdin.ContextWindow.TotalOutputTokens = entry.Usage.OutputTokens
		stdin.ContextWindow.CurrentUsage.InputTokens = entry.Usage.InputTokens
		stdin.ContextWindow.CurrentUsage.OutputTokens = entry.Usage.OutputTokens
		stdin.ContextWindow.CurrentUsage.CacheCreationInputTokens = entry.Usage.CacheCreationInputTokens
		stdin.ContextWindow.CurrentUsage.CacheReadInputTokens = entry.Usage.CacheReadInputTokens
		mask.ContextWindow = true
		debugLog("transcript", "filled ContextWindow from transcript: windowSize=%d input=%d output=%d",
			stdin.ContextWindow.ContextWindowSize, stdin.ContextWindow.TotalInputTokens, stdin.ContextWindow.TotalOutputTokens)
	}

	// Cost 채우기: 단가표 lookup 성공 시에만
	if stdin.Cost.TotalCostUsd <= 0 && entry.Model != "" {
		if p, ok := lookupPricing(entry.Model); ok {
			estimated := estimateCost(entry.Usage, p)
			if estimated > 0 {
				stdin.Cost.TotalCostUsd = estimated
				mask.Cost = true
				debugLog("transcript", "filled Cost from transcript: %.6f USD (estimated)", estimated)
			}
		} else {
			debugLog("transcript", "pricing miss for model %q: cost widget will be skipped", entry.Model)
		}
	}

	return mask
}

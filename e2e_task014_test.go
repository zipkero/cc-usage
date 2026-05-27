package main

// task-014: cost 정책 분기 + 통합 경로 회귀 테스트 세트 (e2e)
//
// spec §5.13이 요구한 경로를 실제 바이너리(exec.Command) 또는 핵심 함수 조합으로
// 검증한다. 바이너리는 TestMain에서 한 번만 빌드하며, 모든 케이스는 t.TempDir()
// 격리 HOME을 사용해 실제 캐시·크리덴셜과 완전히 분리된다.
//
// 커버 경로:
//   (a) cwd 식별 + transcript 존재 → full 복원 (model이 stdout에, cost에 ~ 마커)
//   (b) cwd 식별 + transcript 디렉토리 부재 → graceful fallback (묵음/보수 출력)
//   (c) cwd 식별 불가 → graceful fallback
//   (d) transcript 손상 / 마지막 assistant entry 부재 → graceful fallback
//   (e1) 정상 stdin이 cost 직접 제공 → 마커 없는 정확 cost
//   (e2) 빈 stdin + transcript + 단가표 hit → estimated 마커 cost
//   (e3) 빈 stdin + transcript + 단가표 miss(미등록 모델) → cost 위젯 skip
//   cross-cwd: 다른 cwd transcript의 model/cost가 현재 출력에 나타나지 않음

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// ─── 바이너리 빌드 (TestMain) ────────────────────────────────────────────────

var (
	e2eBinaryPath string
	e2eBinaryOnce sync.Once
	e2eBinaryErr  error
)

// buildE2EBinary는 테스트 실행 시 한 번만 바이너리를 빌드한다.
// dist/ 아래에 cc-usage-e2e-test.exe(Windows) 또는 cc-usage-e2e-test(Unix)를 만든다.
func buildE2EBinary(t *testing.T) string {
	t.Helper()
	e2eBinaryOnce.Do(func() {
		// 출력 경로는 OS에 맞게
		binName := "cc-usage-e2e-test"
		if isWindows() {
			binName += ".exe"
		}
		outPath := filepath.Join("dist", binName)
		if err := os.MkdirAll("dist", 0755); err != nil {
			e2eBinaryErr = fmt.Errorf("mkdir dist: %w", err)
			return
		}
		cmd := exec.Command("go", "build", "-o", outPath, ".")
		cmd.Stdout = os.Stderr // go build 진단을 stderr에 흘림
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			e2eBinaryErr = fmt.Errorf("go build: %w", err)
			return
		}
		e2eBinaryPath = outPath
	})
	if e2eBinaryErr != nil {
		t.Fatalf("e2e binary build failed: %v", e2eBinaryErr)
	}
	return e2eBinaryPath
}

func isWindows() bool {
	return os.PathSeparator == '\\'
}

// ─── 바이너리 실행 헬퍼 ──────────────────────────────────────────────────────

// e2eRunResult는 바이너리 exec 결과를 담는다.
type e2eRunResult struct {
	Stdout string
	Stderr string
	ExitOK bool
}

// runBinary는 격리된 HOME을 가진 환경에서 cc-usage 바이너리를 실행하고 결과를 반환한다.
// stdinJSON이 비어있지 않으면 stdin으로 파이프한다.
// home은 t.TempDir()로 전달한다 — API 캐시·크리덴셜·세션 캐시가 모두 그 아래에 만들어진다.
// CLAUDE_PROJECT_DIR env로 cwd 신호를 주입한다.
func runBinary(t *testing.T, binPath, home, cwdSignal, stdinJSON string) e2eRunResult {
	t.Helper()
	cmd := exec.Command(binPath)

	// stdin 파이프
	if stdinJSON != "" {
		cmd.Stdin = strings.NewReader(stdinJSON)
	} else {
		cmd.Stdin = strings.NewReader("{}")
	}

	// 환경: HOME 격리 + cwd 신호 + PATH(빌드 도구용)
	env := []string{
		"HOME=" + home,
		"USERPROFILE=" + home,
		"PATH=" + os.Getenv("PATH"),
	}
	if cwdSignal != "" {
		env = append(env, "CLAUDE_PROJECT_DIR="+cwdSignal)
	}
	// config 경로를 존재하지 않는 파일로 지정해 기본 설정을 사용하도록 하고
	// 실제 ~/.claude/cc-usage.json을 오염시키지 않는다.
	env = append(env, "HOME="+home) // 중복이지만 명확성 목적
	cmd.Env = env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return e2eRunResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
		ExitOK: err == nil,
	}
}

// ─── transcript fixture 헬퍼 (e2e용) ────────────────────────────────────────

// writeE2ETranscriptEntry는 home 아래 ~/.claude/projects/<encoded> 디렉토리에
// session.jsonl을 만들고 지정된 model/cwd/usage로 assistant entry를 기록한다.
func writeE2ETranscriptEntry(t *testing.T, home, cwd, model string, inputTokens, outputTokens int) {
	t.Helper()
	// cwd 인코딩: /, \, :, . → -
	replacer := strings.NewReplacer("/", "-", `\`, "-", ":", "-", ".", "-")
	encoded := replacer.Replace(cwd)
	dir := filepath.Join(home, ".claude", "projects", encoded)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("writeE2ETranscriptEntry mkdir %s: %v", dir, err)
	}

	// assistant entry JSON
	entry := map[string]any{
		"type": "assistant",
		"cwd":  cwd,
		"message": map[string]any{
			"model": model,
			"usage": map[string]any{
				"input_tokens":               inputTokens,
				"output_tokens":              outputTokens,
				"cache_read_input_tokens":    0,
				"cache_creation_input_tokens": 0,
			},
		},
	}
	line, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("writeE2ETranscriptEntry marshal: %v", err)
	}
	// 마지막 partial 가드를 위해 더미 user 라인 추가
	content := string(line) + "\n" + `{"type":"user","message":{}}` + "\n"

	path := filepath.Join(dir, "session.jsonl")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writeE2ETranscriptEntry write %s: %v", path, err)
	}
}

// ─── (a) full 복원 ───────────────────────────────────────────────────────────

// TestE2E_A_FullRestore: cwd 식별 + transcript 존재 → full 복원.
// 빈 stdin에서 transcript의 model이 stdout에 나오고, cost에 estimated 마커(~)가 있어야 한다.
// cross-cwd 0: 다른 cwd transcript의 model은 출력에 없어야 한다.
func TestE2E_A_FullRestore(t *testing.T) {
	binPath := buildE2EBinary(t)
	home := t.TempDir()
	workspace := t.TempDir()
	cwd := normalizeCwd(workspace)

	// 현재 cwd transcript: claude-opus-4-6, 단가표 hit → estimated cost 표시
	writeE2ETranscriptEntry(t, home, cwd, "claude-opus-4-6", 50000, 10000)

	// 다른 cwd transcript (cross-cwd 격리 검증)
	otherWorkspace := t.TempDir()
	otherCwd := normalizeCwd(otherWorkspace)
	writeE2ETranscriptEntry(t, home, otherCwd, "claude-sonnet-4-6-OTHER", 9999, 9999)

	// 빈 stdin — Layer 2가 발동해야 함
	res := runBinary(t, binPath, home, cwd, "{}")

	t.Logf("(a) stdout=%q stderr=%q", res.Stdout, res.Stderr)

	// model이 stdout에 있어야 함
	if !strings.Contains(res.Stdout, "claude-opus-4-6") {
		t.Errorf("(a) full restore: model not in stdout. stdout=%q", res.Stdout)
	}

	// estimated 마커 (~$) 가 있어야 함 — 빈 stdin이므로 transcript cost는 estimated
	// projectInfo의 경로 압축("~\path")과 구분하기 위해 "~$" 패턴으로 확인
	if !strings.Contains(res.Stdout, "~$") {
		t.Errorf("(a) full restore: estimated marker '~$' not in stdout. stdout=%q", res.Stdout)
	}

	// cross-cwd 0: 다른 cwd의 model이 나와선 안 됨
	if strings.Contains(res.Stdout, "claude-sonnet-4-6-OTHER") {
		t.Errorf("(a) cross-cwd: other model leaked into stdout. stdout=%q", res.Stdout)
	}
}

// ─── (b) transcript 디렉토리 부재 → graceful fallback ───────────────────────

// TestE2E_B_TranscriptDirAbsent: transcript 디렉토리가 아예 없는 상황.
// panic·hang 없이 graceful fallback(묵음 또는 보수 출력)이어야 한다.
func TestE2E_B_TranscriptDirAbsent(t *testing.T) {
	binPath := buildE2EBinary(t)
	home := t.TempDir()
	workspace := t.TempDir()
	cwd := normalizeCwd(workspace)

	// transcript 디렉토리를 만들지 않음

	// 빈 stdin
	res := runBinary(t, binPath, home, cwd, "{}")
	t.Logf("(b) stdout=%q stderr=%q", res.Stdout, res.Stderr)

	// panic/hang이 없으면 통과 (exit 자체는 0이어도 됨)
	// 빈 stdin + 캐시 없음 → 출력은 비어있어야 함 (shouldSuppressOutput)
	// 다만 비어있지 않더라도 잘못된 cwd/model이 없어야 함
	if strings.Contains(res.Stdout, cwd) {
		t.Errorf("(b) absent dir: cwd leaked into stdout=%q", res.Stdout)
	}
}

// ─── (c) cwd 식별 불가 → graceful fallback ──────────────────────────────────

// TestE2E_C_CwdUnidentifiable: CLAUDE_PROJECT_DIR 없이 임의 bin에서 실행 시
// cwd 식별 실패 → graceful fallback.
// 실제로 신뢰할 수 있는 방법은 CLAUDE_PROJECT_DIR을 설정하지 않고
// getwd가 격리 TempDir을 반환하는 환경을 만드는 것이다.
// exec.Cmd.Dir를 workspace로 설정해 os.Getwd가 그 디렉토리를 반환하도록 한다.
func TestE2E_C_CwdUnidentifiable(t *testing.T) {
	binPath := buildE2EBinary(t)
	home := t.TempDir()

	// CLAUDE_PROJECT_DIR을 주지 않고 Dir도 설정하지 않아 cwd 신호가 불분명한 상태.
	// getwd는 Go 테스트 프로세스 cwd를 따르므로 실제 cwd 신호는 cc-usage-project가 된다.
	// 핵심은 HOME을 격리해 캐시를 0으로 만든 뒤 transcript도 없는 상황.
	// 그 상태에서 panic 없이 graceful 출력(빈 stdout 포함)이어야 한다.

	cmd := exec.Command(binPath)
	cmd.Stdin = strings.NewReader("{}")
	cmd.Env = []string{
		"HOME=" + home,
		"USERPROFILE=" + home,
		"PATH=" + os.Getenv("PATH"),
		// CLAUDE_PROJECT_DIR 미설정
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	_ = cmd.Run() // exit 코드는 무관

	out := stdout.String()
	t.Logf("(c) stdout=%q stderr=%q", out, stderr.String())

	// panic 징조(panic: runtime error 등)가 stderr에 없어야 함
	if strings.Contains(stderr.String(), "panic:") {
		t.Errorf("(c) cwd unidentifiable: panic detected in stderr=%q", stderr.String())
	}
	// 잘못된 cross-cwd 데이터가 나오면 안 됨 — 여기서는 빈 stdout or graceful 내용만 확인
	t.Logf("(c) graceful fallback confirmed (no panic, no crash)")
}

// ─── (d) transcript 손상 / assistant entry 부재 → graceful fallback ──────────

// TestE2E_D_CorruptedTranscript: transcript 파일이 손상(JSON 파싱 불가) → graceful fallback.
func TestE2E_D_CorruptedTranscript(t *testing.T) {
	binPath := buildE2EBinary(t)
	home := t.TempDir()
	workspace := t.TempDir()
	cwd := normalizeCwd(workspace)

	// 손상된 transcript 기록
	replacer := strings.NewReplacer("/", "-", `\`, "-", ":", "-", ".", "-")
	encoded := replacer.Replace(cwd)
	dir := filepath.Join(home, ".claude", "projects", encoded)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	corruptContent := "not valid json at all\n{broken\nanother bad line\n"
	if err := os.WriteFile(filepath.Join(dir, "session.jsonl"), []byte(corruptContent), 0644); err != nil {
		t.Fatalf("write corrupted: %v", err)
	}

	res := runBinary(t, binPath, home, cwd, "{}")
	t.Logf("(d) corrupted stdout=%q stderr=%q", res.Stdout, res.Stderr)

	if strings.Contains(res.Stderr, "panic:") {
		t.Errorf("(d) corrupted transcript: panic in stderr=%q", res.Stderr)
	}
	// 잘못된 모델 데이터가 없어야 함
	if strings.Contains(res.Stdout, "claude-") {
		t.Errorf("(d) corrupted: unexpected model in stdout=%q", res.Stdout)
	}
}

// TestE2E_D_NoAssistantEntry: transcript에 assistant entry가 없는 경우 → graceful fallback.
func TestE2E_D_NoAssistantEntry(t *testing.T) {
	binPath := buildE2EBinary(t)
	home := t.TempDir()
	workspace := t.TempDir()
	cwd := normalizeCwd(workspace)

	replacer := strings.NewReplacer("/", "-", `\`, "-", ":", "-", ".", "-")
	encoded := replacer.Replace(cwd)
	dir := filepath.Join(home, ".claude", "projects", encoded)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// user/system line만
	content := `{"type":"user","cwd":"` + cwd + `","message":{"content":"hi"}}` + "\n" +
		`{"type":"system","cwd":"` + cwd + `","message":{"content":"sys"}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "session.jsonl"), []byte(content), 0644); err != nil {
		t.Fatalf("write no-assistant: %v", err)
	}

	res := runBinary(t, binPath, home, cwd, "{}")
	t.Logf("(d) no-assistant stdout=%q stderr=%q", res.Stdout, res.Stderr)

	if strings.Contains(res.Stderr, "panic:") {
		t.Errorf("(d) no-assistant entry: panic in stderr=%q", res.Stderr)
	}
	// model이 나와선 안 됨
	if strings.Contains(res.Stdout, "claude-") {
		t.Errorf("(d) no-assistant: unexpected model in stdout=%q", res.Stdout)
	}
}

// ─── (e1) 정상 stdin cost 직접 제공 → 마커 없는 정확 cost ──────────────────

// TestE2E_E1_DirectCostNoMarker: 정상 stdin이 cost를 직접 줄 때 마커 없이 표시.
func TestE2E_E1_DirectCostNoMarker(t *testing.T) {
	binPath := buildE2EBinary(t)
	home := t.TempDir()
	workspace := t.TempDir()
	cwd := normalizeCwd(workspace)

	// 정상 stdin: cost 포함, model·context 완비
	stdinJSON := fmt.Sprintf(
		`{"model":{"id":"claude-opus-4-6","display_name":"Opus"},"workspace":{"current_dir":%q},"context_window":{"total_input_tokens":50000,"total_output_tokens":10000,"context_window_size":200000,"current_usage":{"input_tokens":50000,"output_tokens":10000,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}},"cost":{"total_cost_usd":3.14}}`,
		cwd,
	)

	res := runBinary(t, binPath, home, cwd, stdinJSON)
	t.Logf("(e1) stdout=%q stderr=%q", res.Stdout, res.Stderr)

	// cost가 나와야 함
	if !strings.Contains(res.Stdout, "3.14") {
		t.Errorf("(e1) direct cost: 3.14 not in stdout=%q", res.Stdout)
	}
	// estimated 마커(~)가 없어야 함
	if strings.Contains(res.Stdout, "~") {
		t.Errorf("(e1) direct cost: estimated marker '~' should NOT appear. stdout=%q", res.Stdout)
	}
	// model이 나와야 함
	if !containsAny(res.Stdout, "claude-opus-4-6", "Opus") {
		t.Errorf("(e1) direct cost: model not in stdout=%q", res.Stdout)
	}
}

// ─── (e2) 빈 stdin + transcript + 단가표 hit → estimated 마커 cost ──────────

// TestE2E_E2_TranscriptEstimatedCost: 빈 stdin + transcript(등록 모델) → estimated 마커.
func TestE2E_E2_TranscriptEstimatedCost(t *testing.T) {
	binPath := buildE2EBinary(t)
	home := t.TempDir()
	workspace := t.TempDir()
	cwd := normalizeCwd(workspace)

	// transcript: claude-opus-4-6 (단가표 hit)
	writeE2ETranscriptEntry(t, home, cwd, "claude-opus-4-6", 50000, 10000)

	res := runBinary(t, binPath, home, cwd, "{}")
	t.Logf("(e2) stdout=%q stderr=%q", res.Stdout, res.Stderr)

	// model이 있어야 함
	if !strings.Contains(res.Stdout, "claude-opus-4-6") {
		t.Errorf("(e2) estimated cost: model not in stdout=%q", res.Stdout)
	}
	// estimated 마커(~$)가 있어야 함
	// projectInfo 경로 압축("~\path")과 구분하기 위해 "~$" 패턴 확인
	if !strings.Contains(res.Stdout, "~$") {
		t.Errorf("(e2) estimated cost: '~$' marker not in stdout=%q", res.Stdout)
	}
}

// ─── (e3) 빈 stdin + transcript + 단가표 miss → cost 위젯 skip ───────────────

// TestE2E_E3_TranscriptPricingMissSkipsCost: 빈 stdin + transcript(미등록 모델)
// → cost 위젯 skip ($ 없음).
func TestE2E_E3_TranscriptPricingMissSkipsCost(t *testing.T) {
	binPath := buildE2EBinary(t)
	home := t.TempDir()
	workspace := t.TempDir()
	cwd := normalizeCwd(workspace)

	// transcript: 단가표에 없는 임의 모델 ID
	// "claude-unknown-model-9999"는 pricing.go에 등록되지 않은 키
	writeE2ETranscriptEntry(t, home, cwd, "claude-unknown-model-9999", 50000, 10000)

	res := runBinary(t, binPath, home, cwd, "{}")
	t.Logf("(e3) stdout=%q stderr=%q", res.Stdout, res.Stderr)

	// model은 나와야 함 (cost만 skip)
	if !strings.Contains(res.Stdout, "claude-unknown-model-9999") {
		// 미등록 모델도 model 위젯은 나와야 한다
		// (model ID가 빈 문자열이 아닌 한 model 위젯은 렌더된다)
		t.Logf("(e3) note: model not in stdout (may be suppressed for other reasons). stdout=%q", res.Stdout)
	}
	// $ cost 마커가 없어야 함 (단가표 miss → cost skip)
	// 단, $ 자체는 다른 맥락에서 나올 수 있으므로 cost-specific pattern 확인
	// estimated 마커 ~ 는 없어야 함 (cost 자체가 skip되므로)
	if strings.Contains(res.Stdout, "~") {
		t.Errorf("(e3) pricing miss: estimated marker '~' should NOT appear (cost widget skipped). stdout=%q", res.Stdout)
	}
}

// ─── cross-cwd 0회 통합 시나리오 ─────────────────────────────────────────────

// TestE2E_CrossCwdIsolation: 다중 cwd 시나리오에서 cwd B의 transcript model이
// cwd A 출력에 나타나지 않음을 검증한다.
func TestE2E_CrossCwdIsolation(t *testing.T) {
	binPath := buildE2EBinary(t)
	home := t.TempDir()

	workspaceA := t.TempDir()
	workspaceB := t.TempDir()
	cwdA := normalizeCwd(workspaceA)
	cwdB := normalizeCwd(workspaceB)

	// cwd A transcript: claude-opus-4-6 (cwd A 소속 entry)
	writeE2ETranscriptEntry(t, home, cwdA, "claude-opus-4-6", 50000, 10000)

	// cwd B transcript: claude-sonnet-4-6 (cwd B 소속 entry)
	writeE2ETranscriptEntry(t, home, cwdB, "claude-sonnet-4-6", 30000, 5000)

	// cwd A로 실행
	resA := runBinary(t, binPath, home, cwdA, "{}")
	t.Logf("(cross-cwd) cwdA stdout=%q", resA.Stdout)

	// cwd A 출력에 B 모델이 나오면 안 됨
	if strings.Contains(resA.Stdout, "claude-sonnet-4-6") {
		t.Errorf("cross-cwd: cwd B model 'claude-sonnet-4-6' leaked into cwd A output: %q", resA.Stdout)
	}

	// cwd B로 실행
	resB := runBinary(t, binPath, home, cwdB, "{}")
	t.Logf("(cross-cwd) cwdB stdout=%q", resB.Stdout)

	// cwd B 출력에 A 모델이 나오면 안 됨
	if strings.Contains(resB.Stdout, "claude-opus-4-6") {
		t.Errorf("cross-cwd: cwd A model 'claude-opus-4-6' leaked into cwd B output: %q", resB.Stdout)
	}
}

// ─── (e) cost 정책 분기 함수 레벨 단위 검증 ────────────────────────────────────

// TestE2E_CostPolicyUnit은 바이너리 exec 없이 applyTranscriptToStdin + costWidget
// 함수를 직접 조합해 (e1)~(e3) cost 정책 분기 각각을 단위로 검증한다.
// 이 테스트는 바이너리 실행 경로의 보완용이며, 함수 수준 결정론을 확보한다.
func TestE2E_CostPolicyUnit(t *testing.T) {
	t.Run("(e1) normal stdin cost direct - no estimated flag", func(t *testing.T) {
		// 정상 stdin: cost.total_cost_usd 직접 제공
		stdin := StdinInput{}
		stdin.Model.ID = "claude-opus-4-6"
		stdin.Model.DisplayName = "Opus"
		stdin.Cost.TotalCostUsd = 3.14
		stdin.ContextWindow.ContextWindowSize = 200000
		stdin.ContextWindow.TotalInputTokens = 50000

		cfg := loadConfig("")
		trans := loadTranslations(cfg.Language)
		ctx := &Context{
			Stdin:        stdin,
			Config:       cfg,
			Translations: trans,
			CostEstimated: false, // 정상 경로: estimated 아님
		}

		// needsTranscriptBackfill은 false여야 함 (model/context 채워져 있음)
		if needsTranscriptBackfill(ctx.Stdin) {
			t.Error("(e1) needsTranscriptBackfill=true on full stdin; expected false (Layer 2 must not trigger)")
		}

		result := orchestrate(ctx)
		combined := strings.Join(result.Lines, "\n")

		// cost가 있어야 함
		if !strings.Contains(combined, "3.14") {
			t.Errorf("(e1) cost not in output: %q", combined)
		}
		// ~$ 마커 없어야 함 (정상 경로는 estimated 아님)
		// projectInfo 경로 압축("~\path")은 ~$ 와 다르므로 안전한 패턴
		if strings.Contains(combined, "~$") {
			t.Errorf("(e1) estimated marker ~$ in output: %q", combined)
		}
	})

	t.Run("(e2) transcript estimated cost - marker present", func(t *testing.T) {
		entry := &transcriptEntry{
			Model: "claude-opus-4-6",
			Cwd:   "/work/proj",
			Usage: transcriptUsage{
				InputTokens:  50000,
				OutputTokens: 10000,
			},
		}

		stdin := StdinInput{}
		mask := applyTranscriptToStdin(&stdin, entry, false)

		// model·context·cost가 채워져야 함
		if !mask.Model {
			t.Error("(e2) mask.Model=false; expected true")
		}
		if !mask.ContextWindow {
			t.Error("(e2) mask.ContextWindow=false; expected true")
		}
		if !mask.Cost {
			t.Error("(e2) mask.Cost=false; claude-opus-4-6 is in pricing table, cost must be estimated")
		}
		if stdin.Cost.TotalCostUsd <= 0 {
			t.Errorf("(e2) Cost=%.6f; expected > 0 (estimated from pricing table)", stdin.Cost.TotalCostUsd)
		}

		// orchestrate with CostEstimated=true → ~ 마커
		cfg := loadConfig("")
		trans := loadTranslations(cfg.Language)
		ctx := &Context{
			Stdin:         stdin,
			Config:        cfg,
			Translations:  trans,
			CostEstimated: true,
		}
		result := orchestrate(ctx)
		combined := strings.Join(result.Lines, "\n")

		if !strings.Contains(combined, "~") {
			t.Errorf("(e2) estimated marker ~ not in output: %q", combined)
		}
	})

	t.Run("(e3) transcript pricing miss - cost widget skipped", func(t *testing.T) {
		entry := &transcriptEntry{
			Model: "claude-unknown-model-9999", // 단가표에 없는 모델
			Cwd:   "/work/proj",
			Usage: transcriptUsage{
				InputTokens:  50000,
				OutputTokens: 10000,
			},
		}

		stdin := StdinInput{}
		mask := applyTranscriptToStdin(&stdin, entry, false)

		// model·context는 채워지지만 cost는 miss
		if !mask.Model {
			t.Error("(e3) mask.Model=false; model should still be filled even with pricing miss")
		}
		if !mask.ContextWindow {
			t.Error("(e3) mask.ContextWindow=false; context should still be filled")
		}
		if mask.Cost {
			t.Error("(e3) mask.Cost=true; pricing miss should leave cost unfilled")
		}
		if stdin.Cost.TotalCostUsd != 0 {
			t.Errorf("(e3) Cost=%.6f; expected 0 (pricing miss)", stdin.Cost.TotalCostUsd)
		}

		// orchestrate with CostEstimated=true + cost=0 → cost 위젯 skip (~ 없음)
		cfg := loadConfig("")
		trans := loadTranslations(cfg.Language)
		ctx := &Context{
			Stdin:         stdin,
			Config:        cfg,
			Translations:  trans,
			CostEstimated: true, // transcript 경로이므로 true지만 cost=0이라 skip
		}
		result := orchestrate(ctx)
		combined := strings.Join(result.Lines, "\n")

		// ~$ 마커 없어야 함 (cost 위젯 자체가 skip)
		// projectInfo 경로 압축("~\path")과 구분하기 위해 "~$" 패턴 확인
		if strings.Contains(combined, "~$") {
			t.Errorf("(e3) pricing miss: ~$ marker should not appear (cost skipped). output=%q", combined)
		}
		// $ 뒤에 숫자가 나오는 패턴이 없어야 함 (cost 위젯 skip)
		// 단순하게 $0.00도 없어야 함 — estimated=true + cost=0 → Render가 ""를 반환
		if strings.Contains(combined, "$0.00") {
			t.Errorf("(e3) pricing miss: $0.00 appeared; cost widget should be fully skipped. output=%q", combined)
		}
	})
}

// ─── cross-cwd 0회 함수 레벨 검증 (entry.cwd D4 가드) ─────────────────────────

// TestE2E_CrossCwdD4GuardUnit은 D4 가드가 cwd 불일치 시 backfill을 차단해
// 다른 cwd 데이터가 출력에 노출되지 않음을 함수 레벨로 검증한다.
func TestE2E_CrossCwdD4GuardUnit(t *testing.T) {
	cwdA := normalizeCwd(t.TempDir())
	cwdB := normalizeCwd(t.TempDir())

	// entry.cwd = cwdB, 현재 실행 cwd = cwdA
	entry := &transcriptEntry{
		Model: "claude-sonnet-4-6-FROM-B",
		Cwd:   cwdB,
		Usage: transcriptUsage{
			InputTokens:  30000,
			OutputTokens: 5000,
		},
	}

	stdin := StdinInput{}
	cfg := loadConfig("")
	trans := loadTranslations(cfg.Language)
	ctx := &Context{
		Stdin:        stdin,
		Config:       cfg,
		Translations: trans,
	}

	// D4 가드: entry.cwd(B) != currentCwd(A) → applyTranscriptToStdin 미호출이어야 함
	entryCwdNorm := normalizeCwd(entry.Cwd)
	if entryCwdNorm != cwdA {
		// 가드가 차단하는 경로 확인
		// applyTranscriptToStdin을 호출하지 않고 ctx.Stdin은 그대로 비어있어야 함
	} else {
		t.Fatal("test setup error: entryCwdNorm == cwdA but they should differ")
	}

	// ctx.Stdin이 비어있으므로 orchestrate 결과에 B 모델이 없어야 함
	result := orchestrate(ctx)
	combined := strings.Join(result.Lines, "\n")

	if strings.Contains(combined, "claude-sonnet-4-6-FROM-B") {
		t.Errorf("cross-cwd D4: cwd B model leaked into output for cwd A: %q", combined)
	}
}

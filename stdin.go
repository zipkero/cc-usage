package main

import (
	"encoding/json"
	"io"
	"os"
	"reflect"
	"sort"
	"strings"
)

// StdinInput represents the JSON payload from Claude Code's status line
// protocol. Only the top-level JSON needs to parse for a value to be
// produced: each top-level section (one of this struct's top-level json
// tags) is decoded independently, and a section that fails to unmarshal
// into its field type is left at its zero value instead of discarding the
// whole payload. Zero value is therefore indistinguishable from "section
// absent" — consumers cannot tell the two apart.
type StdinInput struct {
	Model struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	} `json:"model"`

	Workspace struct {
		CurrentDir string   `json:"current_dir"`
		ProjectDir string   `json:"project_dir,omitempty"`
		AddedDirs  []string `json:"added_dirs,omitempty"`

		// GitWorktree는 빈 문자열을 부재로 본다(포인터 아님) — 공식 문서상 이
		// 필드는 현재 디렉토리가 linked git worktree 안일 때만 존재한다.
		// `--worktree` 세션 전용인 기존 Worktree.Name과 달리 일반 git
		// worktree에서도 채워진다(ANALYSIS §5 D4).
		GitWorktree string `json:"git_worktree,omitempty"`

		// Repo는 부재와 값이 구분되는 포인터로 받는다 — 공식 문서상 이 필드는
		// git 저장소 안이고 origin remote가 설정돼 있을 때만 존재한다. 기존
		// workspace 섹션 안이므로 stdinSectionTable 변경은 필요 없다
		// (ANALYSIS §5 D5).
		Repo *struct {
			Host  string `json:"host"`
			Owner string `json:"owner"`
			Name  string `json:"name"`
		} `json:"repo,omitempty"`
	} `json:"workspace"`

	Worktree *struct {
		Name           string `json:"name"`
		Path           string `json:"path"`
		Branch         string `json:"branch"`
		OriginalCwd    string `json:"original_cwd"`
		OriginalBranch string `json:"original_branch"`
	} `json:"worktree,omitempty"`

	ContextWindow struct {
		TotalInputTokens    int      `json:"total_input_tokens"`
		TotalOutputTokens   int      `json:"total_output_tokens"`
		ContextWindowSize   int      `json:"context_window_size"`
		UsedPercentage      *float64 `json:"used_percentage,omitempty"`
		RemainingPercentage *float64 `json:"remaining_percentage,omitempty"`
		CurrentUsage        struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		} `json:"current_usage"`
	} `json:"context_window"`

	Cost struct {
		TotalCostUsd       float64 `json:"total_cost_usd"`
		TotalDurationMs    *int64  `json:"total_duration_ms,omitempty"`
		TotalApiDurationMs *int64  `json:"total_api_duration_ms,omitempty"`
		TotalLinesAdded    *int    `json:"total_lines_added,omitempty"`
		TotalLinesRemoved  *int    `json:"total_lines_removed,omitempty"`
	} `json:"cost"`

	RateLimits *struct {
		FiveHour *struct {
			UsedPercentage float64 `json:"used_percentage"`
			ResetsAt       int64   `json:"resets_at"`
		} `json:"five_hour,omitempty"`
		SevenDay *struct {
			UsedPercentage float64 `json:"used_percentage"`
			ResetsAt       int64   `json:"resets_at"`
		} `json:"seven_day,omitempty"`
	} `json:"rate_limits,omitempty"`

	OutputStyle *struct {
		Name string `json:"name,omitempty"`
	} `json:"output_style,omitempty"`

	// FastMode은 부재와 false를 구분해야 하므로 포인터로 받는다 — fast mode는
	// /fast로 켜는 opt-in이라 부재·false 모두 "꺼짐"과 같은 뜻이지만, 위젯이
	// 그 구분 자체를 렌더 여부 판정에 쓰지 않고 둘 다 생략으로 처리한다
	// (ANALYSIS §5 D3).
	FastMode *bool `json:"fast_mode,omitempty"`

	// Thinking도 부재와 값을 구분해야 하므로 포인터로 받는다 — fast_mode와
	// 반대로 extended thinking은 기본이 켜짐이라, 위젯은 키가 있으면 항상
	// 렌더하고 Enabled의 true/false를 그대로 구별해 보여준다(ANALYSIS §5 D3).
	Thinking *struct {
		Enabled bool `json:"enabled"`
	} `json:"thinking,omitempty"`

	// Effort는 꺼짐 개념이 없어 값이 있으면 항상 렌더한다. 포인터로 받는 이유는
	// 다른 신규 필드와 같은 부재 구분 목적이다(ANALYSIS §5 D3).
	Effort *struct {
		Level string `json:"level"`
	} `json:"effort,omitempty"`

	Exceeds200kTokens bool   `json:"exceeds_200k_tokens,omitempty"`
	TranscriptPath    string `json:"transcript_path,omitempty"`
	Version           string `json:"version"`
	SessionId         string `json:"session_id,omitempty"`
	SessionName       string `json:"session_name,omitempty"`
	PermissionMode    string `json:"permission_mode,omitempty"`

	Vim *struct {
		Mode string `json:"mode"`
	} `json:"vim,omitempty"`

	Agent *struct {
		Name string `json:"name"`
	} `json:"agent,omitempty"`

	Remote *struct {
		SessionId string `json:"session_id"`
	} `json:"remote,omitempty"`

	AgentId   string `json:"agent_id,omitempty"`
	AgentType string `json:"agent_type,omitempty"`
}

// stdinSection couples one of StdinInput's top-level json tags with a
// pointer to the field it decodes into.
type stdinSection struct {
	key    string
	target any
}

// stdinSectionTable returns the fixed, deterministic order in which stdin's
// top-level sections are decoded into input — one entry per StdinInput
// top-level json tag, pointing at the matching field of input. This order
// also governs the order corrupted sections are later reported in, so it
// must stay stable across runs (map iteration would not be).
// TestStdinSectionTableCompleteness checks this list's key set against
// StdinInput's top-level tags via reflection, in both directions, so the
// table can't silently drift from the struct.
func stdinSectionTable(input *StdinInput) []stdinSection {
	return []stdinSection{
		{"model", &input.Model},
		{"workspace", &input.Workspace},
		{"worktree", &input.Worktree},
		{"context_window", &input.ContextWindow},
		{"cost", &input.Cost},
		{"rate_limits", &input.RateLimits},
		{"output_style", &input.OutputStyle},
		{"fast_mode", &input.FastMode},
		{"thinking", &input.Thinking},
		{"effort", &input.Effort},
		{"exceeds_200k_tokens", &input.Exceeds200kTokens},
		{"transcript_path", &input.TranscriptPath},
		{"version", &input.Version},
		{"session_id", &input.SessionId},
		{"session_name", &input.SessionName},
		{"permission_mode", &input.PermissionMode},
		{"vim", &input.Vim},
		{"agent", &input.Agent},
		{"remote", &input.Remote},
		{"agent_id", &input.AgentId},
		{"agent_type", &input.AgentType},
	}
}

// assembleStdin decodes raw's known top-level sections into a StdinInput, in
// stdinSectionTable's fixed order, and returns the keys of sections that
// failed to decode, also in table order. A key absent from raw leaves its
// field at zero value. A key present but not unmarshalable into its field
// type is also left at zero value: encoding/json can partially populate a
// target before returning an error (e.g. one bad nested field inside an
// otherwise-valid object, or a nil pointer field getting allocated to a
// zero struct on a type mismatch), so the field is reset explicitly rather
// than trusting whatever partial state Unmarshal left behind. This keeps
// the unit of tolerance at the whole section, not at individual fields
// inside it. Keys not present in the table are ignored, matching the
// existing "unknown fields are dropped silently" behavior.
func assembleStdin(raw map[string]json.RawMessage) (StdinInput, []string) {
	var input StdinInput

	var broken []string
	for _, section := range stdinSectionTable(&input) {
		data, ok := raw[section.key]
		if !ok {
			continue
		}
		if err := json.Unmarshal(data, section.target); err != nil {
			broken = append(broken, section.key)
			elem := reflect.ValueOf(section.target).Elem()
			elem.Set(reflect.Zero(elem.Type()))
		}
	}

	return input, broken
}

// unknownStdinKeys returns raw's top-level keys that have no matching entry
// in stdinSectionTable, sorted for deterministic logging. raw is a map, so
// its own iteration order is random across runs — sorting is what makes the
// reported line the same on every run regardless of the order keys appeared
// in the payload.
func unknownStdinKeys(raw map[string]json.RawMessage) []string {
	known := map[string]bool{}
	for _, section := range stdinSectionTable(&StdinInput{}) {
		known[section.key] = true
	}

	var unknown []string
	for key := range raw {
		if !known[key] {
			unknown = append(unknown, key)
		}
	}
	sort.Strings(unknown)
	return unknown
}

// parseStdin reads and decodes JSON from stdin. Returns empty StdinInput on error.
func parseStdin() StdinInput {
	return parseStdinReader(os.Stdin)
}

// parseStdinReader decodes the status line JSON payload from r. If the
// top-level JSON itself doesn't parse (syntax error, a value that isn't an
// object, or no input at all), it returns an empty StdinInput — this path
// is unchanged from before. Otherwise each known top-level section is
// decoded independently via assembleStdin; a section that fails to decode
// is left at zero value in the result instead of discarding the whole
// payload. Corrupted sections and unknown top-level keys are reported to
// stderr via debugLog only — this never affects the returned StdinInput or
// stdout, it just makes a silently-dropped section or key visible when
// debugging. Corrupted sections are logged one per line in
// stdinSectionTable order (the order assembleStdin already returns them in),
// so repeated runs of the same input log identical lines in the same order.
// Split from parseStdin so tests can feed arbitrary readers.
func parseStdinReader(r io.Reader) StdinInput {
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		debugLog("stdin", "parse error: %v", err)
		return StdinInput{}
	}

	input, broken := assembleStdin(raw)
	for _, key := range broken {
		debugLog("stdin", "section %q corrupted, discarded and left at zero value", key)
	}
	if unknown := unknownStdinKeys(raw); len(unknown) > 0 {
		debugLog("stdin", "unknown top-level keys ignored: %s", strings.Join(unknown, ", "))
	}

	return input
}

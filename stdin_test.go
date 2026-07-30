package main

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// TestStdinFractionalPercentages는 Claude Code가 소수 백분율을 보낼 때
// (공식 status line 문서 스키마 예: rate_limits 23.5/41.2, context 8.4)
// 디코딩이 실패하지 않고 값이 보존되는지 검증한다. 과거 used_percentage를
// int로 모델링했을 때는 소수 입력이 전체 stdin 파싱을 실패시켜 상태줄이
// 통째로 사라졌다.
func TestStdinFractionalPercentages(t *testing.T) {
	payload := `{
		"model": {"id": "claude-opus-4-8", "display_name": "Opus"},
		"workspace": {"current_dir": "/tmp"},
		"context_window": {"context_window_size": 200000, "used_percentage": 8.4, "remaining_percentage": 91.6},
		"rate_limits": {
			"five_hour": {"used_percentage": 23.5, "resets_at": 0},
			"seven_day": {"used_percentage": 41.2, "resets_at": 0}
		}
	}`

	in := parseStdinReader(strings.NewReader(payload))

	if in.ContextWindow.UsedPercentage == nil || *in.ContextWindow.UsedPercentage != 8.4 {
		t.Fatalf("context used_percentage not preserved: %+v", in.ContextWindow.UsedPercentage)
	}
	if in.RateLimits == nil || in.RateLimits.FiveHour == nil {
		t.Fatal("rate_limits.five_hour lost after decode")
	}
	if in.RateLimits.FiveHour.UsedPercentage != 23.5 {
		t.Fatalf("five_hour used_percentage = %v, want 23.5", in.RateLimits.FiveHour.UsedPercentage)
	}
	if in.RateLimits.SevenDay.UsedPercentage != 41.2 {
		t.Fatalf("seven_day used_percentage = %v, want 41.2", in.RateLimits.SevenDay.UsedPercentage)
	}

	// 소수 입력에도 정체성 신호(model/workspace/context)가 살아있어 출력이
	// 억제되지 않아야 한다.
	if shouldSuppressOutput(in) {
		t.Fatal("fractional percentages must not blank the status line")
	}
}

// TestContextWidgetFractionalPercent는 소수 used_percentage가 정수로
// 절삭되어 위젯 데이터에 반영되는지 검증한다(문서 bash 예제의 cut -d. -f1과 동일).
func TestContextWidgetFractionalPercent(t *testing.T) {
	in := parseStdinReader(strings.NewReader(`{
		"context_window": {"context_window_size": 200000, "used_percentage": 8.9}
	}`))
	ctx := &Context{Stdin: in, Config: Config{Theme: "default"}}

	data, err := contextWidget{}.GetData(ctx)
	if err != nil || data == nil {
		t.Fatalf("GetData() = %v, %v", data, err)
	}
	if got := data.(*contextData).Percent; got != 8 {
		t.Fatalf("Percent = %d, want 8 (truncated from 8.9)", got)
	}
}

// TestStdinSectionTableCompleteness verifies stdinSectionTable's key set
// matches StdinInput's top-level json tags exactly, in both directions.
// Without this, a field added to StdinInput without a matching table entry
// would silently fall outside section isolation (ANALYSIS §5 D9).
func TestStdinSectionTableCompleteness(t *testing.T) {
	structTags := map[string]bool{}
	rt := reflect.TypeOf(StdinInput{})
	for i := 0; i < rt.NumField(); i++ {
		tag := rt.Field(i).Tag.Get("json")
		name := strings.Split(tag, ",")[0]
		if name == "" || name == "-" {
			t.Fatalf("StdinInput field %s has no usable json tag: %q", rt.Field(i).Name, tag)
		}
		structTags[name] = true
	}

	tableKeys := map[string]bool{}
	var input StdinInput
	for _, section := range stdinSectionTable(&input) {
		if tableKeys[section.key] {
			t.Fatalf("stdinSectionTable has duplicate key %q", section.key)
		}
		tableKeys[section.key] = true
	}

	for name := range structTags {
		if !tableKeys[name] {
			t.Errorf("StdinInput json tag %q has no entry in stdinSectionTable", name)
		}
	}
	for key := range tableKeys {
		if !structTags[key] {
			t.Errorf("stdinSectionTable key %q has no matching StdinInput json tag", key)
		}
	}
}

// validStdinSections holds one syntactically-and-type-valid raw value per
// StdinInput top-level section, used as the baseline for
// TestAssembleStdinSectionIsolation.
var validStdinSections = map[string]string{
	"model":               `{"id":"claude-opus-4-8","display_name":"Opus"}`,
	"workspace":           `{"current_dir":"/tmp/proj"}`,
	"worktree":            `{"name":"wt","path":"/tmp/wt","branch":"feature","original_cwd":"/tmp","original_branch":"main"}`,
	"context_window":      `{"context_window_size":200000,"total_input_tokens":50000,"total_output_tokens":10000}`,
	"cost":                `{"total_cost_usd":1.25}`,
	"rate_limits":         `{"five_hour":{"used_percentage":42,"resets_at":0},"seven_day":{"used_percentage":69,"resets_at":0}}`,
	"output_style":        `{"name":"custom"}`,
	"exceeds_200k_tokens": `true`,
	"transcript_path":     `"/tmp/transcript.jsonl"`,
	"version":             `"1.2.3"`,
	"session_id":          `"session-1"`,
	"session_name":        `"my-session"`,
	"permission_mode":     `"default"`,
	"vim":                 `{"mode":"normal"}`,
	"agent":               `{"name":"reviewer"}`,
	"remote":              `{"session_id":"remote-1"}`,
	"agent_id":            `"agent-1"`,
	"agent_type":          `"subagent"`,
}

// brokenStdinValueFor returns a raw value for key that is syntactically
// valid JSON but the wrong type for the field it would decode into. For
// "rate_limits" the mismatch is nested one level down (five_hour.
// used_percentage), the exact SPEC §5.1 scenario, so this table also proves
// a nested-field failure discards the whole section rather than leaving
// e.g. seven_day's already-parsed value in place.
func brokenStdinValueFor(key string) string {
	switch key {
	case "rate_limits":
		return `{"five_hour":{"used_percentage":"high","resets_at":0},"seven_day":{"used_percentage":69,"resets_at":0}}`
	case "exceeds_200k_tokens":
		return `"nope"`
	case "transcript_path", "version", "session_id", "session_name",
		"permission_mode", "agent_id", "agent_type":
		return `7`
	default:
		return `[1,2,3]`
	}
}

// TestAssembleStdinSectionIsolation is table-driven over every StdinInput
// top-level section: for each, it corrupts only that section's value and
// asserts assembleStdin reports exactly that key as broken while every
// other section keeps its baseline value (ANALYSIS §5 D1, D2, D5).
func TestAssembleStdinSectionIsolation(t *testing.T) {
	validRaw := map[string]json.RawMessage{}
	for key, value := range validStdinSections {
		validRaw[key] = json.RawMessage(value)
	}

	baseline, broken := assembleStdin(validRaw)
	if len(broken) != 0 {
		t.Fatalf("baseline fixture must decode with no broken sections, got %v", broken)
	}

	for _, section := range stdinSectionTable(&StdinInput{}) {
		key := section.key
		t.Run(key, func(t *testing.T) {
			raw := map[string]json.RawMessage{}
			for k, v := range validRaw {
				raw[k] = v
			}
			raw[key] = json.RawMessage(brokenStdinValueFor(key))

			got, broken := assembleStdin(raw)
			if len(broken) != 1 || broken[0] != key {
				t.Fatalf("broken = %v, want exactly [%q]", broken, key)
			}

			// want == baseline but with only this section's field reset to
			// zero value, proving every other section survived untouched.
			want := baseline
			for _, s := range stdinSectionTable(&want) {
				if s.key == key {
					elem := reflect.ValueOf(s.target).Elem()
					elem.Set(reflect.Zero(elem.Type()))
				}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("assembleStdin with %q broken =\n%+v\nwant (only %q zeroed):\n%+v", key, got, key, want)
			}
		})
	}
}

// TestParseStdinReaderTopLevelFailureForms pins SPEC §5.4: none of the four
// ways the top-level JSON itself can fail to parse are recoverable — all
// four fall back to an empty StdinInput that also passes the no-output
// suppression check (ANALYSIS §5 D1, S0).
func TestParseStdinReaderTopLevelFailureForms(t *testing.T) {
	cases := map[string]string{
		"syntax error": `{"model":{"id":"x"`,
		"non-object":   `[1,2,3]`,
		"null":         `null`,
		"empty input":  ``,
	}

	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			got := parseStdinReader(strings.NewReader(payload))
			if !reflect.DeepEqual(got, StdinInput{}) {
				t.Fatalf("parseStdinReader(%q) = %+v, want StdinInput{}", payload, got)
			}
			if !shouldSuppressOutput(got) {
				t.Fatalf("parseStdinReader(%q) result must suppress output", payload)
			}
		})
	}
}

// TestParseStdinReaderIgnoresUnknownTopLevelKey verifies a top-level key
// with no matching StdinInput section is silently dropped, matching the
// pre-existing "unknown fields ignored" behavior (spec.md §3).
func TestParseStdinReaderIgnoresUnknownTopLevelKey(t *testing.T) {
	in := parseStdinReader(strings.NewReader(`{
		"model": {"id": "claude-opus-4-8", "display_name": "Opus"},
		"some_future_field": {"nested": true}
	}`))

	if in.Model.ID != "claude-opus-4-8" {
		t.Fatalf("Model.ID = %q, want %q", in.Model.ID, "claude-opus-4-8")
	}
}

// TestParseStdinReaderDuplicateTopLevelKeyLastWins verifies the top-level
// map decode keeps the last occurrence of a repeated key, matching
// encoding/json's existing struct-decode behavior (ANALYSIS §근거).
func TestParseStdinReaderDuplicateTopLevelKeyLastWins(t *testing.T) {
	in := parseStdinReader(strings.NewReader(`{"version": "1.0", "version": "2.0"}`))
	if in.Version != "2.0" {
		t.Fatalf("Version = %q, want %q (last duplicate key wins)", in.Version, "2.0")
	}
}

// TestParseStdinReaderTrailingGarbageIgnored verifies that content after the
// first complete top-level JSON value doesn't turn into a parse failure —
// json.Decoder reads one value and never looks further (ANALYSIS §근거).
func TestParseStdinReaderTrailingGarbageIgnored(t *testing.T) {
	in := parseStdinReader(strings.NewReader(`{"version": "1.0"} trailing garbage`))
	if in.Version != "1.0" {
		t.Fatalf("Version = %q, want %q despite trailing garbage", in.Version, "1.0")
	}
}

// stderrBrokenSectionsPayload corrupts cost and rate_limits (in that reverse
// order in the payload text, to prove the reported order comes from
// stdinSectionTable and not from payload or map order) and adds three
// unknown top-level keys deliberately out of alphabetical order.
// context_window is left valid with total_input_tokens > 0 so
// FirstResponseReceived() stays true and the corrupted rate_limits section
// is dropped outright by its widgets rather than falling back to the
// session-start placeholder (ANALYSIS §2) — that placeholder path is
// task-001's concern, not this test's.
const stderrBrokenSectionsPayload = `{
	"model": {"id": "claude-opus-4-6", "display_name": "Opus"},
	"workspace": {"current_dir": "/tmp"},
	"rate_limits": {"five_hour": {"used_percentage": "high", "resets_at": 0}, "seven_day": {"used_percentage": 69, "resets_at": 0}},
	"cost": [1, 2, 3],
	"context_window": {"total_input_tokens": 50000, "total_output_tokens": 10000, "context_window_size": 200000},
	"zzz_field": true,
	"aaa_field": 1,
	"mmm_field": "x"
}`

// TestParseStdinReaderLogsBrokenSectionsAndUnknownKeys pins SPEC §5.7 /
// ANALYSIS §5 D8: corrupted sections are reported to stderr in
// stdinSectionTable order (cost before rate_limits, matching the table, even
// though rate_limits appears first in the payload text), and unknown
// top-level keys are reported as one sorted line regardless of the order
// they appeared in the payload. This is the one test in this file (per
// implement.md task-002) that uses captureStderr — repository convention
// keeps os.Stderr replacement to a single, non-parallel test.
func TestParseStdinReaderLogsBrokenSectionsAndUnknownKeys(t *testing.T) {
	t.Setenv("DEBUG", "cc-usage")

	var in StdinInput
	stderr := captureStderr(func() {
		in = parseStdinReader(strings.NewReader(stderrBrokenSectionsPayload))
	})

	costIdx := strings.Index(stderr, `section "cost" corrupted`)
	rateLimitsIdx := strings.Index(stderr, `section "rate_limits" corrupted`)
	unknownIdx := strings.Index(stderr, "unknown top-level keys ignored: aaa_field, mmm_field, zzz_field")

	if costIdx == -1 {
		t.Fatalf("stderr missing cost corruption line: %q", stderr)
	}
	if rateLimitsIdx == -1 {
		t.Fatalf("stderr missing rate_limits corruption line: %q", stderr)
	}
	if unknownIdx == -1 {
		t.Fatalf("stderr missing sorted unknown-keys line: %q", stderr)
	}
	if !(costIdx < rateLimitsIdx && rateLimitsIdx < unknownIdx) {
		t.Fatalf("stderr lines out of stdinSectionTable order: %q", stderr)
	}

	// Model/workspace/context_window sections were untouched, so identity +
	// context survive alongside the two broken sections being zeroed.
	if in.Model.ID != "claude-opus-4-6" {
		t.Fatalf("Model.ID = %q, want survived value", in.Model.ID)
	}
	if in.RateLimits != nil {
		t.Fatalf("RateLimits = %+v, want nil (zero value) after corruption", in.RateLimits)
	}
	if in.Cost.TotalCostUsd != 0 {
		t.Fatalf("Cost = %+v, want zero value after corruption", in.Cost)
	}
	if in.ContextWindow.ContextWindowSize != 200000 {
		t.Fatalf("ContextWindow.ContextWindowSize = %d, want 200000 (untouched)", in.ContextWindow.ContextWindowSize)
	}
}

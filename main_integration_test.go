package main

import (
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStatusLineIdentitylessInputWithoutCwdCacheOutputsNothing(t *testing.T) {
	home := t.TempDir()
	projectDir := t.TempDir()
	configPath := writeStatusLineTestConfig(t, home)

	output := runStatusLineProcess(t, home, projectDir, configPath, `{}`)
	if output != "" {
		t.Fatalf("identity-less stdin without cwd cache output = %q, want empty", output)
	}
}

func TestStatusLineIdentitylessInputReusesCwdCachedOutput(t *testing.T) {
	home := t.TempDir()
	projectDir := t.TempDir()
	configPath := writeStatusLineTestConfig(t, home)

	validInput := `{
		"session_id": "session-cwd-cache",
		"model": {"id": "claude-opus-4-7", "display_name": "Opus"},
		"workspace": {"current_dir": "` + jsonPath(projectDir) + `"},
		"context_window": {
			"total_input_tokens": 1000,
			"total_output_tokens": 250,
			"context_window_size": 1000000,
			"current_usage": {
				"input_tokens": 1000,
				"output_tokens": 250,
				"cache_creation_input_tokens": 0,
				"cache_read_input_tokens": 0
			}
		},
		"cost": {"total_cost_usd": 1.23}
	}`

	first := runStatusLineProcess(t, home, projectDir, configPath, validInput)
	if first == "" {
		t.Fatal("valid stdin produced empty output")
	}

	second := runStatusLineProcess(t, home, projectDir, configPath, `{}`)
	if second == "" {
		t.Fatal("identity-less stdin produced empty output despite cwd cache")
	}
	if second != first {
		t.Fatalf("identity-less stdin output did not reuse last render\nfirst:  %q\nsecond: %q", first, second)
	}
}

func TestStatusLineCwdCacheSurvivesIdleAge(t *testing.T) {
	home := t.TempDir()
	projectDir := t.TempDir()
	configPath := writeStatusLineTestConfig(t, home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	cwdKey := "cwd-" + hashCacheKey(filepath.Clean(projectDir))

	input := StdinInput{}
	input.Workspace.CurrentDir = projectDir
	input.Model.ID = "claude-opus-4-7"
	input.ContextWindow.ContextWindowSize = 1000000
	saveSessionState(cwdKey, &SessionState{
		CachedStdin: &input,
		WidgetCount: 3,
		SavedAt:     1,
		LastOutput:  "cached idle render",
	})

	path := sessionStatePath(cwdKey)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read session state failed: %v", err)
	}
	state, err := decodeSessionState(data)
	if err != nil {
		t.Fatalf("decode session state failed: %v", err)
	}
	state.SavedAt = testUnixNowMinusHours(2)
	data = mustMarshalSessionState(t, state)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write session state failed: %v", err)
	}

	output := runStatusLineProcess(t, home, projectDir, configPath, `{}`)
	if output == "" {
		t.Fatal("idle cwd cache produced empty output")
	}
	if !strings.Contains(output, "claude-opus-4-7") {
		t.Fatalf("idle cwd cache output = %q, want restored model", output)
	}
}

func TestStatusLineHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_STATUSLINE_HELPER") != "1" {
		return
	}
	args := os.Args
	for i, arg := range args {
		if arg == "--" {
			os.Args = append([]string{args[0]}, args[i+1:]...)
			break
		}
	}
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	main()
	os.Exit(0)
}

func writeStatusLineTestConfig(t *testing.T, home string) string {
	t.Helper()
	configPath := filepath.Join(home, "cc-usage.json")
	config := `{
		"displayMode": "custom",
		"separator": "space",
		"lines": [["projectInfo", "model", "cost", "context"]]
	}`
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}
	return configPath
}

func runStatusLineProcess(t *testing.T, home, projectDir, configPath, stdin string) string {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestStatusLineHelperProcess", "--", "--config", configPath)
	cmd.Dir = projectDir
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Env = append(os.Environ(),
		"GO_WANT_STATUSLINE_HELPER=1",
		"USERPROFILE="+home,
		"HOME="+home,
	)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("status line process failed: %v", err)
	}
	return string(out)
}

func jsonPath(path string) string {
	return strings.ReplaceAll(filepath.ToSlash(path), `\`, `\\`)
}

func decodeSessionState(data []byte) (*SessionState, error) {
	var state SessionState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func mustMarshalSessionState(t *testing.T, state *SessionState) []byte {
	t.Helper()
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal session state failed: %v", err)
	}
	return data
}

func testUnixNowMinusHours(hours int) int64 {
	return time.Now().Add(-time.Duration(hours) * time.Hour).Unix()
}

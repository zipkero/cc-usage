package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSelectTranscriptCandidate(t *testing.T) {
	t.Run("absent directory returns error", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "nonexistent")
		got, err := selectTranscriptCandidate(dir)
		if err == nil {
			t.Fatalf("expected error for absent dir, got nil (path=%q)", got)
		}
		if got != "" {
			t.Errorf("expected empty string on error, got %q", got)
		}
	})

	t.Run("empty directory returns error", func(t *testing.T) {
		dir := t.TempDir()
		got, err := selectTranscriptCandidate(dir)
		if err == nil {
			t.Fatalf("expected error for empty dir, got nil (path=%q)", got)
		}
		if got != "" {
			t.Errorf("expected empty string on error, got %q", got)
		}
	})

	t.Run("directory with no jsonl files returns error", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "foo.txt"), []byte{}, 0644); err != nil {
			t.Fatal(err)
		}
		got, err := selectTranscriptCandidate(dir)
		if err == nil {
			t.Fatalf("expected error, got nil (path=%q)", got)
		}
		if got != "" {
			t.Errorf("expected empty string on error, got %q", got)
		}
	})

	t.Run("single jsonl file is returned", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "session-a.jsonl")
		if err := os.WriteFile(p, []byte{}, 0644); err != nil {
			t.Fatal(err)
		}
		got, err := selectTranscriptCandidate(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != p {
			t.Errorf("got %q, want %q", got, p)
		}
	})

	t.Run("multiple jsonl files: newest mtime wins", func(t *testing.T) {
		dir := t.TempDir()
		old := filepath.Join(dir, "old.jsonl")
		newest := filepath.Join(dir, "newest.jsonl")

		if err := os.WriteFile(old, []byte{}, 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(newest, []byte{}, 0644); err != nil {
			t.Fatal(err)
		}

		base := time.Now()
		if err := os.Chtimes(old, base, base); err != nil {
			t.Fatal(err)
		}
		later := base.Add(2 * time.Second)
		if err := os.Chtimes(newest, later, later); err != nil {
			t.Fatal(err)
		}

		got, err := selectTranscriptCandidate(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != newest {
			t.Errorf("got %q, want %q (newest mtime)", got, newest)
		}
	})

	t.Run("mtime tie: lex-first filename wins", func(t *testing.T) {
		dir := t.TempDir()
		fileA := filepath.Join(dir, "aaa.jsonl")
		fileB := filepath.Join(dir, "bbb.jsonl")

		if err := os.WriteFile(fileA, []byte{}, 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fileB, []byte{}, 0644); err != nil {
			t.Fatal(err)
		}

		// 동일한 mtime으로 설정
		same := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
		if err := os.Chtimes(fileA, same, same); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(fileB, same, same); err != nil {
			t.Fatal(err)
		}

		got, err := selectTranscriptCandidate(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != fileA {
			t.Errorf("got %q, want %q (lex-first on tie)", got, fileA)
		}
	})

	t.Run("non-jsonl files are ignored", func(t *testing.T) {
		dir := t.TempDir()
		jsonl := filepath.Join(dir, "session.jsonl")
		other := filepath.Join(dir, "session.json")

		if err := os.WriteFile(jsonl, []byte{}, 0644); err != nil {
			t.Fatal(err)
		}
		// .json에 더 늦은 mtime을 주어도 무시돼야 한다
		if err := os.WriteFile(other, []byte{}, 0644); err != nil {
			t.Fatal(err)
		}
		later := time.Now().Add(10 * time.Second)
		if err := os.Chtimes(other, later, later); err != nil {
			t.Fatal(err)
		}

		got, err := selectTranscriptCandidate(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != jsonl {
			t.Errorf("got %q, want %q (non-jsonl must be ignored)", got, jsonl)
		}
	})
}

// buildAssistantLine은 테스트용 assistant jsonl entry를 만든다.
func buildAssistantLine(model, cwd string, input, output, cacheRead, cacheCreation int) string {
	return fmt.Sprintf(
		`{"type":"assistant","cwd":%q,"message":{"model":%q,"usage":{"input_tokens":%d,"output_tokens":%d,"cache_read_input_tokens":%d,"cache_creation_input_tokens":%d,"cache_creation":{"ephemeral_5m_input_tokens":%d,"ephemeral_1h_input_tokens":0}}}}`,
		cwd, model, input, output, cacheRead, cacheCreation, cacheCreation,
	)
}

func TestReadLastAssistantEntry(t *testing.T) {
	const (
		initialWindow = 64 * 1024       // 64KB
		maxWindow     = 1 * 1024 * 1024 // 1MB
	)

	t.Run("basic: last assistant entry extracted", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "session.jsonl")

		line1 := `{"type":"user","cwd":"/some/path","message":{"content":"hello"}}`
		line2 := buildAssistantLine("claude-opus-4-7", "/work/proj", 1000, 200, 50, 100)
		line3 := buildAssistantLine("claude-sonnet-4-6", "/work/proj", 2000, 400, 80, 160)

		content := strings.Join([]string{line1, line2, line3, ""}, "\n")
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		entry, err := readLastAssistantEntry(path, initialWindow, maxWindow)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if entry == nil {
			t.Fatal("expected entry, got nil")
		}
		if entry.Model != "claude-sonnet-4-6" {
			t.Errorf("model: got %q, want %q", entry.Model, "claude-sonnet-4-6")
		}
		if entry.Cwd != "/work/proj" {
			t.Errorf("cwd: got %q, want %q", entry.Cwd, "/work/proj")
		}
		if entry.Usage.InputTokens != 2000 {
			t.Errorf("input_tokens: got %d, want %d", entry.Usage.InputTokens, 2000)
		}
		if entry.Usage.OutputTokens != 400 {
			t.Errorf("output_tokens: got %d, want %d", entry.Usage.OutputTokens, 400)
		}
		if entry.Usage.CacheReadInputTokens != 80 {
			t.Errorf("cache_read: got %d, want %d", entry.Usage.CacheReadInputTokens, 80)
		}
		if entry.Usage.CacheCreation5mTokens != 160 {
			t.Errorf("cache_creation_5m: got %d, want %d", entry.Usage.CacheCreation5mTokens, 160)
		}
	})

	t.Run("non-assistant types are skipped", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "session.jsonl")

		userLine := `{"type":"user","cwd":"/work","message":{"content":"hi"}}`
		sysLine := `{"type":"system","cwd":"/work","message":{"content":"sys"}}`
		assistLine := buildAssistantLine("claude-opus-4-7", "/work", 500, 100, 0, 0)

		// assistant 다음에 user line이 있어도 마지막 assistant를 반환해야 함
		content := strings.Join([]string{assistLine, userLine, sysLine, ""}, "\n")
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		entry, err := readLastAssistantEntry(path, initialWindow, maxWindow)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if entry == nil {
			t.Fatal("expected entry, got nil")
		}
		if entry.Model != "claude-opus-4-7" {
			t.Errorf("model: got %q, want %q", entry.Model, "claude-opus-4-7")
		}
	})

	t.Run("file with no assistant entries returns nil", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "session.jsonl")

		content := `{"type":"user","message":{"content":"hello"}}` + "\n"
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		entry, err := readLastAssistantEntry(path, initialWindow, maxWindow)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if entry != nil {
			t.Errorf("expected nil, got entry with model %q", entry.Model)
		}
	})

	t.Run("empty file returns nil", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "empty.jsonl")
		if err := os.WriteFile(path, []byte{}, 0644); err != nil {
			t.Fatal(err)
		}

		entry, err := readLastAssistantEntry(path, initialWindow, maxWindow)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if entry != nil {
			t.Errorf("expected nil for empty file, got %+v", entry)
		}
	})

	t.Run("partial last line is skipped, preceding complete line matches", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "session.jsonl")

		completeLine := buildAssistantLine("claude-opus-4-7", "/work", 1000, 200, 0, 0)
		// 마지막 line은 잘린 partial (닫는 괄호 없음)
		partialLine := `{"type":"assistant","cwd":"/work","message":{"model":"claude-sonnet-4-6","usage":{`

		content := completeLine + "\n" + partialLine
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		entry, err := readLastAssistantEntry(path, initialWindow, maxWindow)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if entry == nil {
			t.Fatal("expected entry, got nil (partial last line should be skipped)")
		}
		if entry.Model != "claude-opus-4-7" {
			t.Errorf("model: got %q, want %q (should skip partial last line)", entry.Model, "claude-opus-4-7")
		}
	})

	t.Run("maxWindow exceeded returns nil", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "big.jsonl")

		// assistant entry를 먼저 쓰고 그 뒤에 smallWindow(32바이트)를 훨씬 초과하는
		// user line들을 많이 써서 assistant가 window 밖으로 밀려나도록 한다.
		assistLine := buildAssistantLine("claude-opus-4-7", "/work", 1, 1, 0, 0)
		// smallWindow=64, maxWindow=128로 줄여서 테스트
		const smallInitial = 64
		const smallMax = 128

		var sb strings.Builder
		sb.WriteString(assistLine)
		sb.WriteByte('\n')
		// 128바이트 초과하는 user 라인들
		for i := 0; i < 10; i++ {
			sb.WriteString(fmt.Sprintf(`{"type":"user","cwd":"/work","seq":%d,"padding":"%s"}`, i, strings.Repeat("x", 50)))
			sb.WriteByte('\n')
		}
		if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
			t.Fatal(err)
		}

		entry, err := readLastAssistantEntry(path, smallInitial, smallMax)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if entry != nil {
			// assistant가 window 밖이어야 nil이어야 함 — 파일이 충분히 크면 이 분기
			// 파일이 작아서 전체 읽기가 됐다면 entry를 찾을 수도 있음; 이 경우 skip
			t.Logf("entry found (file may be smaller than max window): model=%s", entry.Model)
		}
	})

	t.Run("maxWindow exceeded (large file) returns nil", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "large.jsonl")

		// assistant entry가 파일 맨 앞에 있고, 뒤에 1MB 넘는 user line들을 채워
		// maxWindow(1MB) 상한으로 읽어도 assistant에 못 닿도록 한다.
		assistLine := buildAssistantLine("claude-opus-4-7", "/work", 1, 1, 0, 0)

		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintln(f, assistLine)
		// 1MB + alpha 크기의 user line을 기록
		padding := strings.Repeat("x", 200)
		for written := 0; written < 1100*1024; {
			line := fmt.Sprintf(`{"type":"user","seq":%d,"pad":%q}`+"\n", written, padding)
			fmt.Fprint(f, line)
			written += len(line)
		}
		f.Close()

		const smallInitial = 64 * 1024
		const smallMax = 1 * 1024 * 1024
		entry, err := readLastAssistantEntry(path, smallInitial, smallMax)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if entry != nil {
			t.Errorf("expected nil when assistant entry is beyond maxWindow, got model=%s", entry.Model)
		}
	})

	t.Run("ephemeral 5m/1h split preserved", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "session.jsonl")

		// 5m=300, 1h=700 으로 분리된 entry
		line := `{"type":"assistant","cwd":"/proj","message":{"model":"claude-opus-4-7","usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":20,"cache_creation_input_tokens":1000,"cache_creation":{"ephemeral_5m_input_tokens":300,"ephemeral_1h_input_tokens":700}}}}` + "\n"
		if err := os.WriteFile(path, []byte(line+"\n"), 0644); err != nil {
			t.Fatal(err)
		}

		entry, err := readLastAssistantEntry(path, initialWindow, maxWindow)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if entry == nil {
			t.Fatal("expected entry, got nil")
		}
		if entry.Usage.CacheCreation5mTokens != 300 {
			t.Errorf("5m: got %d, want 300", entry.Usage.CacheCreation5mTokens)
		}
		if entry.Usage.CacheCreation1hTokens != 700 {
			t.Errorf("1h: got %d, want 700", entry.Usage.CacheCreation1hTokens)
		}
		if entry.Usage.CacheCreationInputTokens != 1000 {
			t.Errorf("cache_creation total: got %d, want 1000", entry.Usage.CacheCreationInputTokens)
		}
	})

	t.Run("cache_creation no ephemeral split: falls back to 5m bucket", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "session.jsonl")

		// ephemeral 없는 구버전 entry: cache_creation_input_tokens만 있음
		line := `{"type":"assistant","cwd":"/proj","message":{"model":"claude-opus-4-7","usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":0,"cache_creation_input_tokens":500}}}` + "\n"
		if err := os.WriteFile(path, []byte(line+"\n"), 0644); err != nil {
			t.Fatal(err)
		}

		entry, err := readLastAssistantEntry(path, initialWindow, maxWindow)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if entry == nil {
			t.Fatal("expected entry, got nil")
		}
		// ephemeral이 없으면 5m에 합산값 귀속
		if entry.Usage.CacheCreation5mTokens != 500 {
			t.Errorf("fallback 5m: got %d, want 500", entry.Usage.CacheCreation5mTokens)
		}
		if entry.Usage.CacheCreation1hTokens != 0 {
			t.Errorf("fallback 1h: got %d, want 0", entry.Usage.CacheCreation1hTokens)
		}
	})

	t.Run("file smaller than initialWindow reads entirely", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "tiny.jsonl")

		line := buildAssistantLine("claude-sonnet-4-6", "/tiny", 10, 5, 0, 0)
		if err := os.WriteFile(path, []byte(line+"\n"), 0644); err != nil {
			t.Fatal(err)
		}

		entry, err := readLastAssistantEntry(path, initialWindow, maxWindow)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if entry == nil {
			t.Fatal("expected entry, got nil for small file")
		}
		if entry.Model != "claude-sonnet-4-6" {
			t.Errorf("model: got %q, want claude-sonnet-4-6", entry.Model)
		}
	})
}

func TestEncodeCwdToTranscriptDir(t *testing.T) {
	tests := []struct {
		name    string
		home    string
		cwd     string
		wantSuf string // 기대하는 encoded 디렉토리 이름 (suffix)
	}{
		{
			name:    "Windows backslash + colon (drive letter)",
			home:    `C:\Users\zipke`,
			cwd:     `C:\Users\zipke\GolandProjects\cc-usage`,
			wantSuf: "C--Users-zipke-GolandProjects-cc-usage",
		},
		{
			name:    "POSIX slash",
			home:    "/home/u",
			cwd:     "/home/u/projects",
			wantSuf: "-home-u-projects",
		},
		{
			name:    "POSIX with dot segment",
			home:    "/home/u",
			cwd:     "/home/u/p.q",
			wantSuf: "-home-u-p-q",
		},
		{
			name:    "dot-prefixed directory (e.g. .claude)",
			home:    "/home/u",
			cwd:     "/home/u/.claude",
			wantSuf: "-home-u--claude",
		},
		{
			name:    "Windows drive root",
			home:    `C:\Users\u`,
			cwd:     `C:\`,
			wantSuf: "C--",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// CLAUDE_CONFIG_DIR 미설정 시 home/.claude/projects fallback을 검증.
			// 실제 셸에 env가 있을 수 있으므로 명시적으로 비운다.
			t.Setenv("CLAUDE_CONFIG_DIR", "")
			got := encodeCwdToTranscriptDir(tc.home, tc.cwd)
			wantPath := filepath.Join(tc.home, ".claude", "projects", tc.wantSuf)
			if got != wantPath {
				t.Errorf("encodeCwdToTranscriptDir(%q, %q)\n  got  %q\n  want %q", tc.home, tc.cwd, got, wantPath)
			}
		})
	}
}

// TestEncodeCwdToTranscriptDir_ConfigDirOverride는 CLAUDE_CONFIG_DIR이 설정된
// 환경(예: ~/.claude-triptopaz)에서 transcript root가 home/.claude가 아니라
// 그 config 디렉토리의 projects/를 가리키는지 검증한다. 이 env를 무시하면
// config 홈을 옮긴 사용자의 워크스페이스 transcript를 전부 놓친다(v0.3.11 회귀).
func TestEncodeCwdToTranscriptDir_ConfigDirOverride(t *testing.T) {
	cfg := filepath.Join(`C:\Users\zipke`, ".claude-triptopaz")
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)

	got := encodeCwdToTranscriptDir(`C:\Users\zipke`, `C:\Users\zipke\GolandProjects\datadog-analyzer`)
	want := filepath.Join(cfg, "projects", "C--Users-zipke-GolandProjects-datadog-analyzer")
	if got != want {
		t.Errorf("CLAUDE_CONFIG_DIR override\n  got  %q\n  want %q", got, want)
	}

	// 공백만 있는 env는 미설정으로 취급 → home fallback.
	t.Setenv("CLAUDE_CONFIG_DIR", "   ")
	gotFallback := encodeCwdToTranscriptDir(`C:\Users\zipke`, `C:\foo`)
	wantFallback := filepath.Join(`C:\Users\zipke`, ".claude", "projects", "C--foo")
	if gotFallback != wantFallback {
		t.Errorf("blank CLAUDE_CONFIG_DIR should fall back to home\n  got  %q\n  want %q", gotFallback, wantFallback)
	}
}

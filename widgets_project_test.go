package main

import (
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func stripANSI(s string) string {
	var b strings.Builder
	inEscape := false
	inCSI := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inEscape {
			if !inCSI && c == '[' {
				inCSI = true
				continue
			}
			if c >= '@' && c <= '~' {
				inEscape = false
				inCSI = false
			}
			continue
		}
		if c == '\x1b' {
			inEscape = true
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// TestProjectInfoGetDataCwdFallback checks that projectInfoWidget.GetData uses
// detectCurrentCwd() as a fallback when Workspace.CurrentDir is empty, and
// returns nil when detectCurrentCwd() also returns "" (SPEC §5.4, task-007).
func TestProjectInfoGetDataCwdFallback(t *testing.T) {
	w := projectInfoWidget{}

	t.Run("빈 workspace + detectCurrentCwd 값 있음 → DisplayPath 채워짐", func(t *testing.T) {
		// swap detectCwdEnv/detectCwdGetwd so detectCurrentCwd() returns a known path
		origEnv := detectCwdEnv
		origGetwd := detectCwdGetwd
		defer func() {
			detectCwdEnv = origEnv
			detectCwdGetwd = origGetwd
		}()

		const fakeCwd = "/fake/project/dir"
		detectCwdEnv = func(key string) string { return "" }
		detectCwdGetwd = func() (string, error) { return fakeCwd, nil }

		ctx := &Context{
			Stdin:  StdinInput{},
			Config: Config{},
		}
		data, err := w.GetData(ctx)
		if err != nil {
			t.Fatalf("GetData returned error: %v", err)
		}
		if data == nil {
			t.Fatal("GetData returned nil, want non-nil projectInfoData")
		}
		d, ok := data.(*projectInfoData)
		if !ok {
			t.Fatalf("GetData returned unexpected type %T", data)
		}
		if d.DisplayPath == "" {
			t.Fatal("DisplayPath is empty, want non-empty path from fallback cwd")
		}
		// fakeCwd base should appear in DisplayPath
		if !strings.Contains(d.DisplayPath, "dir") {
			t.Errorf("DisplayPath %q does not contain expected segment 'dir'", d.DisplayPath)
		}
	})

	t.Run("빈 workspace + detectCurrentCwd 빈 문자열 → nil(기존 동작)", func(t *testing.T) {
		origEnv := detectCwdEnv
		origGetwd := detectCwdGetwd
		defer func() {
			detectCwdEnv = origEnv
			detectCwdGetwd = origGetwd
		}()

		detectCwdEnv = func(key string) string { return "" }
		detectCwdGetwd = func() (string, error) { return "", nil }

		ctx := &Context{
			Stdin:  StdinInput{},
			Config: Config{},
		}
		data, err := w.GetData(ctx)
		if err != nil {
			t.Fatalf("GetData returned error: %v", err)
		}
		if data != nil {
			t.Fatalf("GetData returned %v, want nil when cwd is unknown", data)
		}
	})

	t.Run("정상 Workspace.CurrentDir → detectCurrentCwd 호출 없이 그대로 사용", func(t *testing.T) {
		origEnv := detectCwdEnv
		origGetwd := detectCwdGetwd
		defer func() {
			detectCwdEnv = origEnv
			detectCwdGetwd = origGetwd
		}()

		// detectCurrentCwd가 호출되면 알 수 있도록 sentinal 경로를 반환하게 설정
		detectCwdEnv = func(key string) string { return "/sentinel/path" }
		detectCwdGetwd = func() (string, error) { return "/sentinel/path", nil }

		const normalCwd = "/normal/workspace"
		ctx := &Context{
			Stdin:  StdinInput{},
			Config: Config{},
		}
		ctx.Stdin.Workspace.CurrentDir = normalCwd
		data, err := w.GetData(ctx)
		if err != nil {
			t.Fatalf("GetData returned error: %v", err)
		}
		if data == nil {
			t.Fatal("GetData returned nil, want non-nil projectInfoData")
		}
		d := data.(*projectInfoData)
		// sentinel 경로가 아닌 normalCwd 기반 경로여야 함
		if strings.Contains(d.DisplayPath, "sentinel") {
			t.Errorf("DisplayPath %q contains sentinel — detectCurrentCwd was called when it should not have been", d.DisplayPath)
		}
		if !strings.Contains(d.DisplayPath, "workspace") {
			t.Errorf("DisplayPath %q does not contain expected 'workspace' segment", d.DisplayPath)
		}
	})
}

// TestWorktreeName covers the pure last-path-segment extraction shared by
// both project widgets (SPEC §5.4; ANALYSIS §5 D4). Table-driven since this
// is pure string logic per project convention.
func TestWorktreeName(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "빈 문자열(부재) → 빈 문자열", in: "", want: ""},
		{name: "슬래시 경로 → 마지막 세그먼트", in: "/repo/.git/worktrees/feature-x", want: "feature-x"},
		{name: "백슬래시 경로 → 마지막 세그먼트", in: `C:\repo\worktrees\feature-x`, want: "feature-x"},
		{name: "이름만 있는 경우 → 그대로", in: "feature-x", want: "feature-x"},
		{name: "말미 구분자 제거 후 세그먼트", in: "/repo/worktrees/feature-x/", want: "feature-x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := worktreeName(tc.in)
			if got != tc.want {
				t.Fatalf("worktreeName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestProjectInfoWorktreeToken covers projectInfo's worktree token across key
// absence and both documented value shapes (path vs bare name) — both must
// render identically since git_worktree's own shape isn't specified
// (SPEC §5.4; ANALYSIS §5 D4). git is disabled so Branch stays empty and
// doesn't interfere with the assertions.
func TestProjectInfoWorktreeToken(t *testing.T) {
	t.Setenv("PATH", "")

	cases := []struct {
		name        string
		gitWorktree string
		wantToken   bool
		wantName    string
	}{
		{name: "키 없음 → 토큰 생략", gitWorktree: "", wantToken: false},
		{name: "경로 형태 → 마지막 세그먼트 토큰", gitWorktree: "/repo/.git/worktrees/feature-x", wantToken: true, wantName: "feature-x"},
		{name: "이름 형태 → 그대로 토큰", gitWorktree: "feature-x", wantToken: true, wantName: "feature-x"},
	}

	w := projectInfoWidget{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &Context{
				Stdin:  StdinInput{},
				Config: Config{Theme: "default"},
			}
			ctx.Stdin.Workspace.CurrentDir = "/tmp/project"
			ctx.Stdin.Workspace.GitWorktree = tc.gitWorktree

			data, err := w.GetData(ctx)
			if err != nil {
				t.Fatalf("GetData returned error: %v", err)
			}
			rendered := stripANSI(w.Render(data, ctx))
			if tc.wantToken {
				want := "[" + tc.wantName + "]"
				if !strings.Contains(rendered, want) {
					t.Fatalf("projectInfo render = %q, want token %q", rendered, want)
				}
			} else if strings.Contains(rendered, "[") || strings.Contains(rendered, "]") {
				t.Fatalf("projectInfo render = %q, want no worktree token", rendered)
			}
		})
	}
}

func TestProjectInfoOmitsRemovedStatusTokens(t *testing.T) {
	w := projectInfoWidget{}
	ctx := &Context{
		Stdin:  StdinInput{},
		Config: Config{Theme: "default"},
	}
	ctx.Stdin.Workspace.CurrentDir = "/tmp/project"
	ctx.Stdin.Worktree = &struct {
		Name           string `json:"name"`
		Path           string `json:"path"`
		Branch         string `json:"branch"`
		OriginalCwd    string `json:"original_cwd"`
		OriginalBranch string `json:"original_branch"`
	}{Name: "feature-worktree"}

	data, err := w.GetData(ctx)
	if err != nil {
		t.Fatalf("GetData returned error: %v", err)
	}
	if data == nil {
		t.Fatal("GetData returned nil, want non-nil projectInfoData")
	}
	rendered := stripANSI(w.Render(data, ctx))
	for _, token := range []string{"↑", "↓", "[", "]", "feature-worktree"} {
		if strings.Contains(rendered, token) {
			t.Fatalf("projectInfo rendered removed token %q in %q", token, rendered)
		}
	}

	rendered = w.Render(&projectInfoData{
		DisplayPath: "/tmp/project",
		Branch:      "main",
	}, ctx)
	rendered = stripANSI(rendered)
	if !strings.Contains(rendered, "/tmp/project") {
		t.Fatalf("projectInfo render = %q, want display path", rendered)
	}
	if !strings.Contains(rendered, "(main)") {
		t.Fatalf("projectInfo render = %q, want branch", rendered)
	}
	for _, token := range []string{"↑", "↓", "[", "]"} {
		if strings.Contains(rendered, token) {
			t.Fatalf("projectInfo rendered removed token %q in %q", token, rendered)
		}
	}
}

func TestProjectNameWidget(t *testing.T) {
	w := projectNameWidget{}
	ctx := &Context{
		Stdin:  StdinInput{},
		Config: Config{Theme: "default"},
	}
	ctx.Stdin.Workspace.CurrentDir = "/Users/alice/projects/cc-usage"
	t.Setenv("PATH", "")

	data, err := w.GetData(ctx)
	if err != nil {
		t.Fatalf("GetData returned error: %v", err)
	}
	if data == nil {
		t.Fatal("GetData returned nil, want non-nil projectNameData")
	}
	d, ok := data.(*projectNameData)
	if !ok {
		t.Fatalf("GetData returned unexpected type %T", data)
	}
	if d.Name != "cc-usage" {
		t.Fatalf("Name = %q, want %q", d.Name, "cc-usage")
	}
	if d.Branch != "" {
		t.Fatalf("Branch = %q, want empty branch when git is unavailable", d.Branch)
	}

	rendered := stripANSI(w.Render(data, ctx))
	if rendered != "cc-usage" {
		t.Fatalf("Render = %q, want base name only", rendered)
	}
	for _, token := range []string{"~", "…", "/"} {
		if strings.Contains(rendered, token) {
			t.Fatalf("projectName rendered path marker %q in %q", token, rendered)
		}
	}

	rendered = stripANSI(w.Render(&projectNameData{Name: "cc-usage", Branch: "main"}, ctx))
	if rendered != "cc-usage (main)" {
		t.Fatalf("Render with branch = %q, want %q", rendered, "cc-usage (main)")
	}
}

// TestProjectNameWorktreeToken covers projectName's worktree token across key
// absence and both documented value shapes (path vs bare name), mirroring
// TestProjectInfoWorktreeToken so both widgets share the same verified shape
// (SPEC §5.4; ANALYSIS §5 D4).
func TestProjectNameWorktreeToken(t *testing.T) {
	t.Setenv("PATH", "")

	cases := []struct {
		name        string
		gitWorktree string
		wantToken   bool
		wantName    string
	}{
		{name: "키 없음 → 토큰 생략", gitWorktree: "", wantToken: false},
		{name: "경로 형태 → 마지막 세그먼트 토큰", gitWorktree: "/repo/.git/worktrees/feature-x", wantToken: true, wantName: "feature-x"},
		{name: "이름 형태 → 그대로 토큰", gitWorktree: "feature-x", wantToken: true, wantName: "feature-x"},
	}

	w := projectNameWidget{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &Context{
				Stdin:  StdinInput{},
				Config: Config{Theme: "default"},
			}
			ctx.Stdin.Workspace.CurrentDir = "/Users/alice/projects/cc-usage"
			ctx.Stdin.Workspace.GitWorktree = tc.gitWorktree

			data, err := w.GetData(ctx)
			if err != nil {
				t.Fatalf("GetData returned error: %v", err)
			}
			rendered := stripANSI(w.Render(data, ctx))
			if tc.wantToken {
				want := "[" + tc.wantName + "]"
				if !strings.Contains(rendered, want) {
					t.Fatalf("projectName render = %q, want token %q", rendered, want)
				}
			} else if strings.Contains(rendered, "[") || strings.Contains(rendered, "]") {
				t.Fatalf("projectName render = %q, want no worktree token", rendered)
			}
		})
	}
}

func TestProjectNameGetDataSkipsWhenCwdUnknown(t *testing.T) {
	w := projectNameWidget{}
	origEnv := detectCwdEnv
	origGetwd := detectCwdGetwd
	defer func() {
		detectCwdEnv = origEnv
		detectCwdGetwd = origGetwd
	}()

	detectCwdEnv = func(key string) string { return "" }
	detectCwdGetwd = func() (string, error) { return "", nil }

	data, err := w.GetData(&Context{
		Stdin:  StdinInput{},
		Config: Config{Theme: "default"},
	})
	if err != nil {
		t.Fatalf("GetData returned error: %v", err)
	}
	if data != nil {
		t.Fatalf("GetData returned %v, want nil when cwd is unknown", data)
	}
}

// TestProjectPathCompressHome covers the home-tilde compression branch of the
// projectInfo path display helper (SPEC §5.1, §5.2). All inputs are
// deterministic strings — no os.UserHomeDir / wall-clock dependency.
func TestProjectPathCompressHome(t *testing.T) {
	// compressHome matches prefixes against filepath.Separator, so home must
	// carry the OS-native separator. Paths under it are joined from the
	// already-normalized home so they cannot drift from it.
	home := filepath.FromSlash("/Users/alice")
	homeProjectsPath := filepath.Join(home, "projects", "cc-usage")

	cases := []struct {
		name    string
		current string
		home    string
		want    string
	}{
		{
			name:    "current equals home collapses to tilde",
			current: home,
			home:    home,
			want:    "~",
		},
		{
			name:    "current under home gets tilde prefix",
			current: homeProjectsPath,
			home:    home,
			want:    filepath.FromSlash("~/projects/cc-usage"),
		},
		{
			name:    "current outside home stays absolute",
			current: "/var/log/system",
			home:    home,
			want:    "/var/log/system",
		},
		{
			name:    "empty home (lookup failure) keeps absolute path",
			current: homeProjectsPath,
			home:    "",
			want:    homeProjectsPath,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := compressHome(tc.current, tc.home)
			if got != tc.want {
				t.Fatalf("compressHome(%q, %q) = %q, want %q", tc.current, tc.home, got, tc.want)
			}
		})
	}
}

// TestProjectPathShrink covers the segment-aware length normalization branch
// (SPEC §5.3, §5.7). Inputs are constructed with byte-counted segments so
// the 50-rune budget assertions are deterministic on ASCII.
func TestProjectPathShrink(t *testing.T) {
	const max = 50

	// Short tilde path: well under budget — must pass through untouched.
	shortTilde := "~/projects/cc-usage"
	if utf8.RuneCountInString(shortTilde) > max {
		t.Fatalf("precondition: shortTilde must be <= %d runes, got %d", max, utf8.RuneCountInString(shortTilde))
	}

	// Long tilde path: 5 middle segments + base, total runes > 50. The
	// minimal collapsed form "~/…/<base>" must fit within the budget so we
	// observe the shrink branch (not the bust-budget fallback).
	// FromSlash adapts to the OS separator. It is the identity when
	// Separator == '/', so POSIX behavior and the rune count stay unchanged.
	longTildePath := filepath.FromSlash("~/aaaaaaaa/bbbbbbbb/cccccccc/dddddddd/eeeeeeee/proj")
	if utf8.RuneCountInString(longTildePath) <= max {
		t.Fatalf("precondition: longTildePath must be > %d runes, got %d", max, utf8.RuneCountInString(longTildePath))
	}

	// Long absolute (outside home) path > 50 runes, collapses to "/…/<base>".
	longAbsPath := filepath.FromSlash("/var/aaaaaaaa/bbbbbbbb/cccccccc/dddddddd/eeeeeeee/proj")
	if utf8.RuneCountInString(longAbsPath) <= max {
		t.Fatalf("precondition: longAbsPath must be > %d runes, got %d", max, utf8.RuneCountInString(longAbsPath))
	}

	// Base name alone exceeds budget — must be returned as-is (no mid-rune cut).
	longBase := strings.Repeat("x", 60)
	soloBasePath := "/tmp/" + longBase
	if utf8.RuneCountInString(longBase) <= max {
		t.Fatalf("precondition: longBase must be > %d runes, got %d", max, utf8.RuneCountInString(longBase))
	}

	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "length within budget is unchanged",
			in:   shortTilde,
			want: shortTilde,
		},
		{
			name: "tilde path over budget collapses middle to ellipsis",
			in:   longTildePath,
			want: filepath.FromSlash("~/…/proj"),
		},
		{
			name: "absolute path over budget keeps leading separator",
			in:   longAbsPath,
			want: filepath.FromSlash("/…/proj"),
		},
		{
			name: "base name alone exceeding budget is returned as-is",
			in:   soloBasePath,
			want: longBase,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shrinkPath(tc.in, max)
			if got != tc.want {
				t.Fatalf("shrinkPath(%q, %d) = %q, want %q", tc.in, max, got, tc.want)
			}
		})
	}
}

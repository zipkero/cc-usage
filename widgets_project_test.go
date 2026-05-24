package main

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestProjectPathCompressHome covers the home-tilde compression branch of the
// projectInfo path display helper (SPEC §5.1, §5.2). All inputs are
// deterministic strings — no os.UserHomeDir / wall-clock dependency.
func TestProjectPathCompressHome(t *testing.T) {
	const home = "/Users/alice"

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
			current: home + "/projects/cc-usage",
			home:    home,
			want:    "~/projects/cc-usage",
		},
		{
			name:    "current outside home stays absolute",
			current: "/var/log/system",
			home:    home,
			want:    "/var/log/system",
		},
		{
			name:    "empty home (lookup failure) keeps absolute path",
			current: home + "/projects/cc-usage",
			home:    "",
			want:    home + "/projects/cc-usage",
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
	longTildePath := "~/aaaaaaaa/bbbbbbbb/cccccccc/dddddddd/eeeeeeee/proj"
	if utf8.RuneCountInString(longTildePath) <= max {
		t.Fatalf("precondition: longTildePath must be > %d runes, got %d", max, utf8.RuneCountInString(longTildePath))
	}

	// Long absolute (outside home) path > 50 runes, collapses to "/…/<base>".
	longAbsPath := "/var/aaaaaaaa/bbbbbbbb/cccccccc/dddddddd/eeeeeeee/proj"
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
			want: "~/…/proj",
		},
		{
			name: "absolute path over budget keeps leading separator",
			in:   longAbsPath,
			want: "/…/proj",
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

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// version is set by ldflags at build time.
var version = "dev"

// debugLog prints debug messages to stderr when DEBUG=cc-usage or DEBUG=1.
func debugLog(context string, format string, args ...any) {
	dbg := os.Getenv("DEBUG")
	if dbg == "cc-usage" || dbg == "1" {
		msg := fmt.Sprintf(format, args...)
		fmt.Fprintf(os.Stderr, "[cc-usage:%s] %s\n", context, msg)
	}
}

// defaultConfigPath returns ~/.claude/cc-usage.json.
func defaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "cc-usage.json")
}

func main() {
	configPath := flag.String("config", defaultConfigPath(), "config path")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	// Determine configDir
	var configDir string
	if *configPath != "" {
		configDir = filepath.Dir(*configPath)
	} else {
		home, _ := os.UserHomeDir()
		configDir = filepath.Join(home, ".claude")
	}

	debugLog("main", "configPath=%s configDir=%s", *configPath, configDir)

	cfg := loadConfig(*configPath)
	debugLog("main", "config loaded: language=%s displayMode=%s", cfg.Language, cfg.DisplayMode)

	input := parseStdin()
	debugLog("main", "stdin parsed: model=%s version=%s", input.Model.ID, input.Version)

	// credential + API
	token := getCredential(configDir)
	debugLog("main", "credential: len=%d", len(token))

	var rateLimits *UsageLimits
	if token != "" {
		rateLimits = fetchUsageLimits(token, cfg.Cache)
	}

	// Purge stale session-state files in the background. Independent of token
	// presence — cleanup must run even when no API call is made.
	go cleanOldSessionStates()

	translations := loadTranslations(cfg.Language)
	debugLog("main", "translations loaded: lang=%s", cfg.Language)

	// Load cached session state. When stdin arrives empty (no session id,
	// remote, agent, transcript, or cwd) sessionCacheKey() returns "" and the
	// keyed lookup misses even if a valid cache exists on disk. Fall back to
	// the most-recent on-disk session-state so an empty stdin can still
	// restore the prior render.
	cacheKey := sessionCacheKey(input)
	cached := loadSessionState(cacheKey)
	if cached == nil && cacheKey == "" {
		cached = loadMostRecentSessionState()
		if cached != nil {
			debugLog("main", "empty stdin → fallback to most-recent session cache")
		}
	}

	ctx := &Context{
		Stdin:        input,
		Config:       cfg,
		ConfigDir:    configDir,
		Translations: translations,
		RateLimits:   rateLimits,
	}

	result := orchestrate(ctx)

	// Degraded input: current stdin rendered fewer widgets than the last good
	// render, or workspace.current_dir arrived empty while a recent cache still
	// has it. Restore the minimum needed so widgets don't flicker away.
	if cached != nil && cached.CachedStdin != nil {
		workspaceStale := ctx.Stdin.Workspace.CurrentDir == "" && cached.CachedStdin.Workspace.CurrentDir != ""
		usageDegraded := result.WidgetCount < cached.WidgetCount
		// cost widget always renders, so widget count alone cannot detect a
		// cost-only regression — track it as an independent signal.
		costRegressed := shouldRestoreCost(ctx.Stdin, cached, time.Now())

		restoreWorkspace := workspaceStale && cached.SavedAt > 0 &&
			time.Since(time.Unix(cached.SavedAt, 0)) < workspaceRestoreTTL

		if restoreWorkspace {
			debugLog("main", "workspace empty, restoring from cache (age < %s)", workspaceRestoreTTL)
			ctx.Stdin.Workspace = cached.CachedStdin.Workspace
			if ctx.Stdin.Worktree == nil {
				ctx.Stdin.Worktree = cached.CachedStdin.Worktree
			}
		}

		if usageDegraded {
			debugLog("main", "degraded input (widgets=%d, cached=%d), restoring usage fields from cache", result.WidgetCount, cached.WidgetCount)
			restoreUsageFields(&ctx.Stdin, cached.CachedStdin)
		}

		if costRegressed {
			debugLog("main", "cost regressed to 0 (cached=%.4f), restoring cost from cache", cached.CachedStdin.Cost.TotalCostUsd)
			ctx.Stdin.Cost = cached.CachedStdin.Cost
		}

		if restoreWorkspace || usageDegraded || costRegressed {
			result = orchestrate(ctx)
		}
	}

	// Suppress output when stdin lacks any session identity (workspace, model,
	// context) even after cache restoration. Without this, cost/rate-limit
	// widgets — which render unconditionally — would produce partial output
	// like "$0.00 │ 5h: -- │ 7d: --" on calls with empty stdin (e.g. right
	// after /reload-plugins before Claude Code has warmed the session).
	//
	// Exception: when account-global rate-limit data is available from the
	// API cache, render anyway. Empty status lines hide useful 5h/7d signals
	// during the warmup window where Claude Code sends degraded stdin for
	// extended periods.
	if shouldSuppressOutput(ctx.Stdin, ctx.RateLimits) {
		debugLog("main", "stdin has no identity context and no rate-limit data, suppressing output")
		return
	}

	var partsOutput string
	if len(result.Lines) > 0 {
		partsOutput = strings.Join(result.Lines, "\n")
	}

	if partsOutput != "" {
		fmt.Print(partsOutput)
	}

	// Save stdin (not rendered strings) so a future degrade can re-render with
	// fresh account-global values. Strip RateLimits: those must always come
	// from the live API cache, never from stale session memory.
	if result.WidgetCount >= 2 {
		snapshot := ctx.Stdin
		snapshot.RateLimits = nil
		saveSessionState(cacheKey, &SessionState{
			CachedStdin: &snapshot,
			WidgetCount: result.WidgetCount,
		})
	}
}

// restoreUsageFields fills empty model / cost / context_window fields on
// stdin from a cached snapshot. Each field is restored independently so a
// partially-degraded stdin keeps whatever fresh data it does carry. Pure
// function; safe to test without spinning up main().
//
// Model is part of the identity bundle: an empty stdin loses model too, so
// without explicit restoration the model widget vanishes whenever cost /
// context restore fires. Skip restoration when stdin already has either half
// of the model identity — fresh stdin always wins.
func restoreUsageFields(stdin *StdinInput, cached *StdinInput) {
	if stdin == nil || cached == nil {
		return
	}
	if stdin.Model.ID == "" && stdin.Model.DisplayName == "" {
		stdin.Model = cached.Model
	}
	if stdin.Cost.TotalCostUsd <= 0 {
		stdin.Cost = cached.Cost
	}
	if stdin.ContextWindow.TotalInputTokens+stdin.ContextWindow.TotalOutputTokens == 0 {
		stdin.ContextWindow = cached.ContextWindow
	}
}

// shouldSuppressOutput reports whether main should emit nothing to stdout.
// Returns true only when stdin has no session identity AND the API-side
// rate-limit cache has no usable bucket either. When at least one rate-limit
// bucket is present, render anyway so the status line surfaces account-global
// 5h/7d signals during long degraded-stdin warmup windows.
func shouldSuppressOutput(stdin StdinInput, rl *UsageLimits) bool {
	noIdentity := stdin.Workspace.CurrentDir == "" &&
		stdin.Model.ID == "" && stdin.Model.DisplayName == "" &&
		stdin.ContextWindow.ContextWindowSize <= 0
	if !noIdentity {
		return false
	}
	hasRateLimitData := rl != nil &&
		(rl.FiveHour != nil || rl.SevenDay != nil || rl.SevenDaySonnet != nil)
	return !hasRateLimitData
}

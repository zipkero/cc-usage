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
	flag.Parse()

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
	debugLog("main", "config loaded: language=%s plan=%s displayMode=%s", cfg.Language, cfg.Plan, cfg.DisplayMode)

	input := parseStdin()
	debugLog("main", "stdin parsed: model=%s version=%s", input.Model.ID, input.Version)

	// credential + API
	token := getCredential(configDir)
	debugLog("main", "credential: len=%d", len(token))

	var rateLimits *UsageLimits
	if token != "" {
		rateLimits = fetchUsageLimits(token, cfg.Cache)
	}

	translations := loadTranslations(cfg.Language)
	debugLog("main", "translations loaded: lang=%s", cfg.Language)

	// Load cached session state
	cacheKey := sessionCacheKey(input)
	cached := loadSessionState(cacheKey)

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
		modelStale := ctx.Stdin.Model.ID == "" && ctx.Stdin.Model.DisplayName == "" &&
			(cached.CachedStdin.Model.ID != "" || cached.CachedStdin.Model.DisplayName != "")
		usageDegraded := result.WidgetCount < cached.WidgetCount

		restoreTTL := sessionStateTTLForKey(cacheKey)
		restoreWorkspace := workspaceStale && cached.SavedAt > 0 &&
			time.Since(time.Unix(cached.SavedAt, 0)) < restoreTTL
		restoreModel := modelStale && cached.SavedAt > 0 &&
			time.Since(time.Unix(cached.SavedAt, 0)) < restoreTTL

		if restoreWorkspace {
			debugLog("main", "workspace empty, restoring from cache (age < %s)", restoreTTL)
			ctx.Stdin.Workspace = cached.CachedStdin.Workspace
			if ctx.Stdin.Worktree == nil {
				ctx.Stdin.Worktree = cached.CachedStdin.Worktree
			}
		}
		if restoreModel {
			debugLog("main", "model empty, restoring from cache (age < %s)", restoreTTL)
			ctx.Stdin.Model = cached.CachedStdin.Model
		}

		if usageDegraded {
			debugLog("main", "degraded input (widgets=%d, cached=%d), restoring usage fields from cache", result.WidgetCount, cached.WidgetCount)
			if ctx.Stdin.Cost.TotalCostUsd <= 0 {
				ctx.Stdin.Cost = cached.CachedStdin.Cost
			}
			if ctx.Stdin.ContextWindow.TotalInputTokens+ctx.Stdin.ContextWindow.TotalOutputTokens == 0 {
				ctx.Stdin.ContextWindow = cached.CachedStdin.ContextWindow
			}
		}

		if restoreWorkspace || restoreModel || usageDegraded {
			result = orchestrate(ctx)
		}
	}

	// Suppress output when stdin lacks any session identity (workspace, model,
	// context) even after cache restoration. Without this, cost/rate-limit
	// widgets — which render unconditionally — would produce partial output
	// like "$0.00 │ 5h: -- │ 7d: --" on calls with empty stdin (e.g. right
	// after /reload-plugins before Claude Code has warmed the session).
	noIdentity := ctx.Stdin.Workspace.CurrentDir == "" &&
		ctx.Stdin.Model.ID == "" && ctx.Stdin.Model.DisplayName == "" &&
		ctx.Stdin.ContextWindow.ContextWindowSize <= 0
	if noIdentity {
		debugLog("main", "stdin has no identity context, suppressing output (session_id=%q remote=%t agent_id=%q transcript_path=%q current_dir=%q)",
			ctx.Stdin.SessionId, ctx.Stdin.Remote != nil, ctx.Stdin.AgentId, ctx.Stdin.TranscriptPath, ctx.Stdin.Workspace.CurrentDir)
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

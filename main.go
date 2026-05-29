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

// configHomeDir returns Claude Code's config directory. Claude Code relocates
// it via CLAUDE_CONFIG_DIR (e.g. ~/.claude-triptopaz), and .credentials.json
// lives under that same root — so the credential lookup must follow it too,
// matching transcript.go's root resolution. Returns CLAUDE_CONFIG_DIR when set
// (whitespace-only treated as unset), otherwise <home>/.claude.
func configHomeDir(home string) string {
	if cfg := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR")); cfg != "" {
		return cfg
	}
	return filepath.Join(home, ".claude")
}

// defaultConfigPath returns {CLAUDE_CONFIG_DIR or ~/.claude}/cc-usage.json.
func defaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(configHomeDir(home), "cc-usage.json")
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
		configDir = configHomeDir(home)
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

	// Load cached session state, falling back to a cwd-based scan when stdin
	// is too degraded to produce a cache key (SPEC §5.1, §5.4, ANALYSIS §3.2).
	cacheKey := sessionCacheKey(input)
	cached := resolveCachedSessionState(cacheKey, time.Now())

	ctx := &Context{
		Stdin:        input,
		Config:       cfg,
		ConfigDir:    configDir,
		Translations: translations,
		RateLimits:   rateLimits,
	}

	result := orchestrate(ctx)

	// Degraded input: single eligibility decision determines whether any field
	// from the session cache is used to backfill the current stdin. All fields
	// are restored together (atomic) or not at all, preventing half-populated
	// status lines (SPEC §5.1–§5.3, ANALYSIS §1–§2).
	var restoredMask restoredFieldMask
	if shouldRestoreFromSession(ctx.Stdin, cached, time.Now()) {
		restoredMask = fillFromSessionCache(&ctx.Stdin, cached)
		debugLog("main", "session cache backfill: mask=%+v", restoredMask)
		result = orchestrate(ctx)
	}

	// Layer 2: transcript backfill (TTL 무관, estimated cost).
	// Layer 1 이후에도 model/ContextWindow가 여전히 비어있을 때만 발동한다.
	// entry.cwd × normalizeCwd 정확 일치 가드(D4)로 cross-cwd 노출 0 보장.
	if needsTranscriptBackfill(ctx.Stdin) {
		debugLog("transcript", "Layer 2 triggered: model=%q contextWindowSize=%d",
			ctx.Stdin.Model.ID, ctx.Stdin.ContextWindow.ContextWindowSize)
		cwd := detectCurrentCwd()
		if cwd == "" {
			debugLog("transcript", "Layer 2 skipped: cwd undetectable")
		} else {
			debugLog("transcript", "Layer 2 cwd detected: %s", cwd)
		}
		transcriptPath := ctx.Stdin.TranscriptPath
		if transcriptPath != "" {
			debugLog("transcript", "transcriptPath from stdin.transcript_path: %s", transcriptPath)
		} else if cwd != "" {
			home, homeErr := os.UserHomeDir()
			if homeErr != nil {
				debugLog("transcript", "Layer 2 skipped: home dir unavailable: %v", homeErr)
			} else {
				transcriptDir := encodeCwdToTranscriptDir(home, cwd)
				if candidate, err := selectTranscriptCandidate(transcriptDir); err == nil {
					transcriptPath = candidate
					debugLog("transcript", "transcriptPath selected from encoded dir: %s", transcriptPath)
				} else {
					debugLog("transcript", "no transcript candidate for cwd=%s dir=%s: %v", cwd, transcriptDir, err)
				}
			}
		}
		if transcriptPath != "" {
			const initialWindow = 64 * 1024      // 64KB
			const maxWindow = 1 * 1024 * 1024    // 1MB
			entry, err := readLastAssistantEntry(transcriptPath, initialWindow, maxWindow)
			if err != nil {
				debugLog("transcript", "readLastAssistantEntry error path=%s: %v", transcriptPath, err)
			} else if entry != nil {
				debugLog("transcript", "readLastAssistantEntry ok: model=%q entry.cwd=%q", entry.Model, entry.Cwd)
				// D4 가드: entry.cwd × normalizeCwd 정확 일치 시에만 적용
				entryCwdNorm := normalizeCwd(entry.Cwd)
				if cwd != "" && entryCwdNorm == cwd {
					oneMSignal := loadLastKnownOneM(cwd)
					mask2 := applyTranscriptToStdin(&ctx.Stdin, entry, oneMSignal)
					// Layer 1·2 mask 통합 (D8): stripRestoredFields가 두 layer 모두 vacate
					if mask2.Model {
						restoredMask.Model = true
					}
					if mask2.Cost {
						restoredMask.Cost = true
					}
					if mask2.ContextWindow {
						restoredMask.ContextWindow = true
					}
					ctx.CostEstimated = true // D6: transcript cost는 항상 estimated
					debugLog("transcript", "Layer 2 applied: model=%s oneMSignal=%v mask=%+v",
						ctx.Stdin.Model.ID, oneMSignal, mask2)
					result = orchestrate(ctx)
				} else {
					debugLog("transcript", "Layer 2 D4 guard blocked: entry.cwd=%q (norm=%q) != current=%q",
						entry.Cwd, entryCwdNorm, cwd)
				}
			} else {
				debugLog("transcript", "no assistant entry found in transcript: %s", transcriptPath)
			}
		} else if cwd == "" {
			// cwd 미식별로 transcriptPath를 결정하지 못한 경우 — 이미 위에서 로그함
		} else {
			debugLog("transcript", "Layer 2 skipped: no transcriptPath resolved for cwd=%s", cwd)
		}
	} else {
		debugLog("transcript", "Layer 2 not needed: model=%q contextWindowSize=%d",
			ctx.Stdin.Model.ID, ctx.Stdin.ContextWindow.ContextWindowSize)
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

	// Warmup exception: identity is absent but rate-limit data is available.
	// Restrict output to rate-limit widgets only so widgets that render
	// unconditionally (like cost showing "$0.00") don't leak misleading values
	// when no session identity is established (SPEC §5.3).
	warmupOnly := isWarmupExceptionPath(ctx.Stdin, ctx.RateLimits)
	if warmupOnly {
		debugLog("main", "warmup exception: rendering rate-limit widgets only")
		result = renderRateLimitOnly(ctx)
	}

	var partsOutput string
	if len(result.Lines) > 0 {
		partsOutput = strings.Join(result.Lines, "\n")
	}

	if partsOutput != "" {
		fmt.Print(partsOutput)
	}

	// task-011: last-known [1m] 저장 트리거.
	// 원본 stdin(input, backfill 이전)이 [1m] 신호를 직접 들고 왔고 cwd가
	// 식별될 때만 저장한다. transcript backfill로 채워진 ctx.Stdin은 base ID만
	// 가지므로 참조하지 않는다(SPEC §5.3, ANALYSIS §5 D2, D10).
	if strings.Contains(input.Model.ID, "[1m]") {
		if triggerCwd := detectCurrentCwd(); triggerCwd != "" {
			saveLastKnownOneM(triggerCwd, true)
			debugLog("main", "saved last-known [1m] for cwd=%s", triggerCwd)
		}
	}

	// Save stdin (not rendered strings) so a future degrade can re-render with
	// fresh account-global values. Strip RateLimits: those must always come
	// from the live API cache, never from stale session memory. Also strip any
	// fields that were backfilled from the session cache in this call so that
	// a degraded-only call cannot perpetuate cached values via repeated saves
	// (SPEC §5.6, ANALYSIS §5.D, adopted D1).
	if !warmupOnly && result.WidgetCount >= 2 {
		snapshot := ctx.Stdin
		snapshot.RateLimits = nil
		stripRestoredFields(&snapshot, restoredMask)
		saveSessionState(cacheKey, &SessionState{
			CachedStdin: &snapshot,
			WidgetCount: result.WidgetCount,
		})
	}
}

// resolveCachedSessionState loads the session cache for the active stdin,
// transparently falling back to a cwd-based scan whenever the primary
// per-key load misses (SPEC §5.1, §5.4, ANALYSIS §3.2, §12 D2).
//
// Order of resolution:
//  1. Primary hit (cached != nil): return as-is. fallback is never consulted,
//     so an active session's own cache always wins over sibling workspaces.
//  2. Primary miss (cached == nil): try fallbackByWorkspaceCwd regardless of
//     whether cacheKey was empty. This covers two degraded shapes with one
//     branch — fully empty stdin (cacheKey == "") and stdin that carries a
//     session_id whose per-key file is missing or in a legacy format.
//     Cross-workspace exposure is not gated here; loadByWorkspaceCwd enforces
//     exact normalized-cwd equality + sessionStateTTL, so a stale or foreign
//     cwd can never bleed through.
//
// fallback never populates RateLimits — the on-disk SessionState is written
// with RateLimits stripped (see main()'s save block), and the API-cache path
// in main() supplies fresh 5h/7d values regardless of which branch ran here.
func resolveCachedSessionState(cacheKey string, now time.Time) *SessionState {
	cached := loadSessionState(cacheKey)
	if cached == nil {
		return fallbackByWorkspaceCwd(now)
	}
	return cached
}

// fallbackByWorkspaceCwd resolves the empty-stdin fallback (SPEC §5.1, §5.4).
// Returns a SessionState only when (a) the current cwd can be identified via
// detectCurrentCwd and (b) loadByWorkspaceCwd finds a non-expired match for
// exactly that cwd. Returns nil in every other case — including missing home
// dir, unknown cwd, no candidate, or all candidates expired. Indirected through
// a package-level var so tests can replace cwd / matcher dependencies without
// having to seed real cache files under HOME.
var fallbackByWorkspaceCwd = func(now time.Time) *SessionState {
	cwd, source := detectCurrentCwdWithSource()
	if cwd == "" {
		debugLog("fallback", "empty stdin -> no cwd signal (env miss, getwd miss) -> suppress/partial")
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		debugLog("fallback", "empty stdin -> no cache for cwd=%s source=%s (home dir unavailable)", cwd, source)
		return nil
	}
	cacheDir := filepath.Join(home, ".cache", "cc-usage")
	state := loadByWorkspaceCwd(cacheDir, cwd, now)
	if state == nil {
		debugLog("fallback", "empty stdin -> no cache for cwd=%s source=%s", cwd, source)
		return nil
	}
	debugLog("fallback", "empty stdin -> matched cache via cwd=%s source=%s", cwd, source)
	return state
}

// shouldRestoreWorkspace는 workspace 복원의 stale-cwd 가드(SPEC §5.11,
// ANALYSIS §5.2)다. 같은 session 안에서 cd로 다른 워크스페이스로 이동한 직후
// 빈 workspace stdin이 도착하면 cached Workspace.CurrentDir는 직전 디렉토리(A)를
// 가리키지만 사용자는 이미 B에 있다. 그대로 복원하면 화면에 A의 cwd/projectInfo가
// 노출되므로, detectCurrentCwd로 얻은 현재 신호와 정규화 기준 정확 일치할 때만
// true를 반환한다. 현재 cwd를 식별할 수 없으면(env/getwd 모두 실패) cross-workspace
// 노출 위험을 피하기 위해 복원 자체를 skip. cost/context 등 비-워크스페이스 복원은
// 이 가드의 영향을 받지 않는다.
func shouldRestoreWorkspace(cachedCwd string) bool {
	currentCwd := detectCurrentCwd()
	if currentCwd == "" {
		debugLog("fallback", "empty stdin -> workspace restore blocked: cached_cwd=%s current_cwd=<unknown>", cachedCwd)
		return false
	}
	if normalizeCwd(cachedCwd) != currentCwd {
		debugLog("fallback", "empty stdin -> workspace restore blocked: cached_cwd=%s current_cwd=%s", cachedCwd, currentCwd)
		return false
	}
	return true
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

// isWarmupExceptionPath reports whether the status line should render only
// rate-limit widgets — true precisely when identity is absent but rate-limit
// data is available. Mirrors shouldSuppressOutput's identity check but the
// rate-limit polarity is inverted (SPEC §5.3 warmup exception).
func isWarmupExceptionPath(stdin StdinInput, rl *UsageLimits) bool {
	noIdentity := stdin.Workspace.CurrentDir == "" &&
		stdin.Model.ID == "" && stdin.Model.DisplayName == "" &&
		stdin.ContextWindow.ContextWindowSize <= 0
	if !noIdentity {
		return false
	}
	return rl != nil && (rl.FiveHour != nil || rl.SevenDay != nil || rl.SevenDaySonnet != nil)
}

// renderRateLimitOnly orchestrates only the rate-limit widgets registered for
// the warmup exception path. Cost / context / identity widgets are skipped so
// the status line cannot leak a misleading "$0.00" or empty bar when the
// session has no identity yet.
func renderRateLimitOnly(ctx *Context) OrchestrateResult {
	ids := []string{"rateLimit5h", "rateLimit7d", "rateLimit7dSonnet"}
	sep := renderSeparator(ctx.Config.Separator, getTheme(ctx.Config.Theme))
	var parts []string
	for _, id := range ids {
		w, ok := registry[id]
		if !ok {
			continue
		}
		data, err := w.GetData(ctx)
		if err != nil || data == nil {
			continue
		}
		rendered := w.Render(data, ctx)
		if rendered == "" {
			continue
		}
		parts = append(parts, rendered)
	}
	result := OrchestrateResult{}
	if len(parts) > 0 {
		result.Lines = []string{strings.Join(parts, sep)}
		result.WidgetCount = len(parts)
	}
	return result
}

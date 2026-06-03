package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
// it via CLAUDE_CONFIG_DIR. Returns CLAUDE_CONFIG_DIR when set
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

	debugLog("main", "configPath=%s", *configPath)

	cfg := loadConfig(*configPath)
	debugLog("main", "config loaded: language=%s displayMode=%s", cfg.Language, cfg.DisplayMode)

	input := parseStdin()
	debugLog("main", "stdin parsed: model=%s version=%s", input.Model.ID, input.Version)

	translations := loadTranslations(cfg.Language)
	debugLog("main", "translations loaded: lang=%s", cfg.Language)

	ctx := &Context{
		Stdin:        input,
		Config:       cfg,
		Translations: translations,
	}

	if shouldSuppressOutput(ctx.Stdin) {
		debugLog("main", "stdin has no identity context, suppressing output")
		return
	}

	result := orchestrate(ctx)
	if len(result.Lines) > 0 {
		fmt.Print(strings.Join(result.Lines, "\n"))
	}
}

// shouldSuppressOutput reports whether main should emit nothing to stdout.
// Model, workspace, and context are the session identity signals. Rate-limit
// data no longer affects this decision because it only comes from stdin.
func shouldSuppressOutput(stdin StdinInput) bool {
	return stdin.Workspace.CurrentDir == "" &&
		stdin.Model.ID == "" && stdin.Model.DisplayName == "" &&
		stdin.ContextWindow.ContextWindowSize <= 0
}

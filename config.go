package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// CacheConfig holds cache-related settings.
type CacheConfig struct {
	TTLSeconds int `json:"ttlSeconds"`
}

// Config represents the user configuration from cc-usage.json.
type Config struct {
	Language        string      `json:"language"`
	DisplayMode     string      `json:"displayMode"`
	Lines           [][]string  `json:"lines,omitempty"`
	DisabledWidgets []string    `json:"disabledWidgets,omitempty"`
	Theme           string      `json:"theme,omitempty"`
	Separator       string      `json:"separator,omitempty"`
	Preset          string      `json:"preset,omitempty"`
	Cache           CacheConfig `json:"cache"`
}

// defaultConfig returns the Config with default values.
func defaultConfig() Config {
	return Config{
		Language:    "auto",
		DisplayMode: "compact",
		Cache:       CacheConfig{TTLSeconds: 300},
	}
}

// loadConfig reads and parses the config file. Falls back to defaults on any error.
func loadConfig(path string) Config {
	defaults := defaultConfig()

	if path == "" {
		debugLog("config", "no config path, using defaults")
		return defaults
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			debugLog("config", "config file not found: %s, using defaults", path)
		} else {
			debugLog("config", "config read error: %v, using defaults", err)
		}
		return defaults
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		debugLog("config", "config parse error: %v, using defaults", err)
		return defaults
	}

	// Merge: fill zero-value fields with defaults
	if cfg.Language == "" {
		cfg.Language = defaults.Language
	}
	if cfg.DisplayMode == "" {
		cfg.DisplayMode = defaults.DisplayMode
	}
	if cfg.Cache.TTLSeconds == 0 {
		cfg.Cache.TTLSeconds = defaults.Cache.TTLSeconds
	}

	validateConfigEnums(&cfg)

	return cfg
}

// validateConfigEnums checks enum-typed config fields against their allowed
// sets. On unknown non-empty value, it emits a single-line warning to
// os.Stderr (independent of DEBUG) and resets the field to its fallback
// default. Empty values are treated as "use default" and pass silently.
func validateConfigEnums(cfg *Config) {
	themeAllowed := make([]string, 0, len(themes)+1)
	for k := range themes {
		themeAllowed = append(themeAllowed, k)
	}
	themeAllowed = append(themeAllowed, "")

	checks := []struct {
		field    string
		value    *string
		allowed  []string
		fallback string
	}{
		{"displayMode", &cfg.DisplayMode, []string{"compact", "custom"}, "compact"},
		{"separator", &cfg.Separator, []string{"pipe", "dot", "arrow", "space", ""}, ""},
		{"theme", &cfg.Theme, themeAllowed, ""},
		{"language", &cfg.Language, []string{"auto", "en", "ko"}, "auto"},
	}

	for _, c := range checks {
		v := *c.value
		if v == "" {
			continue
		}
		if containsString(c.allowed, v) {
			continue
		}
		fmt.Fprintln(os.Stderr, formatEnumWarning(c.field, v, c.allowed, c.fallback))
		*c.value = c.fallback
	}
}

func containsString(set []string, v string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}

func formatEnumWarning(field, value string, allowed []string, fallback string) string {
	sorted := append([]string(nil), allowed...)
	sort.Strings(sorted)
	return fmt.Sprintf("cc-usage: invalid config: %s=%s not in {%s}, falling back to %s",
		field, value, joinAllowed(sorted), fallback)
}

func joinAllowed(items []string) string {
	out := ""
	for i, s := range items {
		if i > 0 {
			out += ", "
		}
		if s == "" {
			out += `""`
		} else {
			out += s
		}
	}
	return out
}

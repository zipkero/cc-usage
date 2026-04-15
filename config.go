package main

import (
	"encoding/json"
	"os"
)

// CacheConfig holds cache-related settings.
type CacheConfig struct {
	TTLSeconds int `json:"ttlSeconds"`
}

// Config represents the user configuration from cc-usage.json.
type Config struct {
	Language        string      `json:"language"`
	Plan            string      `json:"plan"`
	DisplayMode     string      `json:"displayMode"`
	Lines           [][]string  `json:"lines,omitempty"`
	DisabledWidgets []string    `json:"disabledWidgets,omitempty"`
	Theme           string      `json:"theme,omitempty"`
	Separator       string      `json:"separator,omitempty"`
	Preset          string      `json:"preset,omitempty"`
	DailyBudget     *float64    `json:"dailyBudget,omitempty"`
	Cache           CacheConfig `json:"cache"`
}

// defaultConfig returns the Config with default values.
func defaultConfig() Config {
	return Config{
		Language:    "auto",
		Plan:        "max",
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
	if cfg.Plan == "" {
		cfg.Plan = defaults.Plan
	}
	if cfg.DisplayMode == "" {
		cfg.DisplayMode = defaults.DisplayMode
	}
	if cfg.Cache.TTLSeconds == 0 {
		cfg.Cache.TTLSeconds = defaults.Cache.TTLSeconds
	}

	return cfg
}

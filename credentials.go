package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// credentialsFile represents the JSON structure of .credentials.json.
type credentialsFile struct {
	ClaudeAiOauth struct {
		AccessToken string `json:"accessToken"`
	} `json:"claudeAiOauth"`
}

// Global cache for file-based credential.
var (
	cachedToken     string
	cachedFileMtime time.Time
)

// Global cache for macOS Keychain credential.
var (
	keychainCachedAt     time.Time
	keychainToken        string
	keychainBackoffUntil time.Time
)

// getCredential returns an OAuth access token, trying macOS Keychain first on darwin.
func getCredential(configDir string) string {
	if runtime.GOOS == "darwin" {
		token := getCredentialFromKeychain()
		if token != "" {
			return token
		}
		debugLog("credentials", "keychain unavailable, falling back to file")
	}
	return getCredentialFromFile(configDir)
}

// getCredentialFromKeychain tries to read the credential from macOS Keychain.
func getCredentialFromKeychain() string {
	now := time.Now()

	// Backoff: if we recently failed, skip keychain
	if now.Before(keychainBackoffUntil) {
		return ""
	}

	// Return cached if within 10 seconds
	if keychainToken != "" && now.Sub(keychainCachedAt) < 10*time.Second {
		return keychainToken
	}

	cmd := exec.Command("security", "find-generic-password", "-s", "Claude Code-credentials", "-w")
	out, err := cmd.Output()
	if err != nil {
		debugLog("credentials", "keychain lookup failed: %v", err)
		keychainBackoffUntil = now.Add(60 * time.Second)
		return ""
	}

	raw := string(out)
	raw = trimSpace(raw)
	if raw == "" {
		debugLog("credentials", "keychain returned empty value")
		keychainBackoffUntil = now.Add(60 * time.Second)
		return ""
	}

	var creds credentialsFile
	if err := json.Unmarshal([]byte(raw), &creds); err != nil {
		debugLog("credentials", "keychain JSON parse failed: %v", err)
		keychainBackoffUntil = now.Add(60 * time.Second)
		return ""
	}

	token := creds.ClaudeAiOauth.AccessToken
	if token == "" {
		debugLog("credentials", "keychain: accessToken is empty")
		keychainBackoffUntil = now.Add(60 * time.Second)
		return ""
	}

	keychainToken = token
	keychainCachedAt = now
	return token
}

// getCredentialFromFile reads the credential from {configDir}/.credentials.json.
func getCredentialFromFile(configDir string) string {
	path := filepath.Join(configDir, ".credentials.json")

	info, err := os.Stat(path)
	if err != nil {
		debugLog("credentials", "stat %s failed: %v", path, err)
		return ""
	}

	mtime := info.ModTime()
	if cachedToken != "" && mtime.Equal(cachedFileMtime) {
		return cachedToken
	}

	data, err := os.ReadFile(path)
	if err != nil {
		debugLog("credentials", "read %s failed: %v", path, err)
		return ""
	}

	var creds credentialsFile
	if err := json.Unmarshal(data, &creds); err != nil {
		debugLog("credentials", "parse %s failed: %v", path, err)
		return ""
	}

	token := creds.ClaudeAiOauth.AccessToken
	if token == "" {
		debugLog("credentials", "accessToken is empty in %s", path)
		return ""
	}

	cachedToken = token
	cachedFileMtime = mtime
	return token
}

// trimSpace removes leading/trailing whitespace including newlines.
func trimSpace(s string) string {
	// Avoid importing strings just for TrimSpace; do it inline.
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

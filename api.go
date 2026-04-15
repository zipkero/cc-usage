package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

// UsageLimitEntry represents a single rate limit bucket.
type UsageLimitEntry struct {
	Utilization int
	ResetsAt    time.Time
}

// UsageLimits holds all rate limit buckets from the API.
type UsageLimits struct {
	FiveHour       *UsageLimitEntry
	SevenDay       *UsageLimitEntry
	SevenDaySonnet *UsageLimitEntry
}

// apiUsageResponse is the JSON shape returned by the usage API.
type apiUsageResponse struct {
	FiveHour struct {
		Utilization int    `json:"utilization"`
		ResetsAt    string `json:"resets_at"`
	} `json:"five_hour"`
	SevenDay struct {
		Utilization int    `json:"utilization"`
		ResetsAt    string `json:"resets_at"`
	} `json:"seven_day"`
	SevenDaySonnet struct {
		Utilization int    `json:"utilization"`
		ResetsAt    string `json:"resets_at"`
	} `json:"seven_day_sonnet"`
}

// cacheEntry is stored in both memory and file caches (success only).
type cacheEntry struct {
	Data      *apiUsageResponse `json:"data"`
	Timestamp time.Time         `json:"timestamp"`
}

// memCache holds successful API responses keyed by token hash.
var memCache = map[string]*cacheEntry{}

// negativeCache tracks recent API failures (memory only, not persisted to file).
var negativeCache = map[string]time.Time{}

// lastCleanup tracks when old cache files were last purged.
var lastCleanup time.Time

const (
	apiURL           = "https://api.anthropic.com/api/oauth/usage"
	userAgent        = "cc-usage/0.1.0"
	apiBeta          = "oauth-2025-04-20"
	negativeCacheTTL = 30 * time.Second
	staleCacheMaxAge = time.Hour
	apiTimeout       = 10 * time.Second
)

// hashToken returns a short hex prefix of the SHA-256 of the token.
func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])[:16]
}

// fetchUsageLimits retrieves usage limits using a 3-tier cache (memory → file → API).
func fetchUsageLimits(token string, cacheCfg CacheConfig) *UsageLimits {
	if token == "" {
		return nil
	}

	hash := hashToken(token)
	now := time.Now()
	ttl := time.Duration(cacheCfg.TTLSeconds) * time.Second

	// Negative cache guard: recent failure → skip API, return stale if available
	if failedAt, ok := negativeCache[hash]; ok && now.Sub(failedAt) < negativeCacheTTL {
		debugLog("api", "negative cache hit, skipping API call")
		return staleFallback(hash, now)
	}

	// 1. Memory cache
	if entry, ok := memCache[hash]; ok && now.Sub(entry.Timestamp) < ttl {
		debugLog("api", "memory cache hit (age=%v)", now.Sub(entry.Timestamp))
		return parseUsageLimits(entry.Data)
	}

	// 2. File cache
	if entry := readFileCache(hash); entry != nil && now.Sub(entry.Timestamp) < ttl {
		debugLog("api", "file cache hit (age=%v)", now.Sub(entry.Timestamp))
		memCache[hash] = entry
		return parseUsageLimits(entry.Data)
	}

	// 3. API call
	go cleanOldCaches()

	resp, err := callAPI(token)
	if err != nil {
		debugLog("api", "API call failed: %v", err)
		negativeCache[hash] = now
		return staleFallback(hash, now)
	}

	// Success: update caches
	entry := &cacheEntry{Data: resp, Timestamp: now}
	memCache[hash] = entry
	writeFileCache(hash, entry)
	debugLog("api", "API call succeeded, cached")
	return parseUsageLimits(resp)
}

// staleFallback returns stale cache data (up to 1 hour old) or nil.
func staleFallback(hash string, now time.Time) *UsageLimits {
	// Check memory
	if entry, ok := memCache[hash]; ok && now.Sub(entry.Timestamp) < staleCacheMaxAge {
		debugLog("api", "using stale memory cache (age=%v)", now.Sub(entry.Timestamp))
		return parseUsageLimits(entry.Data)
	}
	// Check file
	if entry := readFileCache(hash); entry != nil && now.Sub(entry.Timestamp) < staleCacheMaxAge {
		debugLog("api", "using stale file cache (age=%v)", now.Sub(entry.Timestamp))
		memCache[hash] = entry
		return parseUsageLimits(entry.Data)
	}
	return nil
}

// callAPI makes an HTTP request to the Anthropic usage API.
func callAPI(token string) (*apiUsageResponse, error) {
	client := &http.Client{Timeout: apiTimeout}

	resp, err := doAPIRequest(client, token)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return decodeAPIResponse(resp.Body)

	case http.StatusTooManyRequests:
		retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
		if retryAfter > 10*time.Second {
			retryAfter = 10 * time.Second
		}
		if retryAfter < 1*time.Second {
			retryAfter = 1 * time.Second
		}
		resp.Body.Close()

		debugLog("api", "429 received, retrying after %v", retryAfter)
		time.Sleep(retryAfter)

		// One retry
		resp2, err := doAPIRequest(client, token)
		if err != nil {
			return nil, fmt.Errorf("retry failed: %w", err)
		}
		defer resp2.Body.Close()
		if resp2.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("retry returned status %d", resp2.StatusCode)
		}
		return decodeAPIResponse(resp2.Body)

	case http.StatusForbidden:
		resp.Body.Close()
		debugLog("api", "403 received, falling back to curl")
		return callAPIWithCurl(token)

	default:
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
}

// doAPIRequest creates and executes a single API request.
func doAPIRequest(client *http.Client, token string) (*http.Response, error) {
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("anthropic-beta", apiBeta)

	return client.Do(req)
}

// decodeAPIResponse reads and parses the JSON response body.
func decodeAPIResponse(body io.Reader) (*apiUsageResponse, error) {
	var result apiUsageResponse
	if err := json.NewDecoder(body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}

// parseRetryAfter parses the Retry-After header value (seconds).
func parseRetryAfter(val string) time.Duration {
	if val == "" {
		return 1 * time.Second
	}
	secs, err := strconv.Atoi(val)
	if err != nil || secs < 0 {
		return 1 * time.Second
	}
	return time.Duration(secs) * time.Second
}

// callAPIWithCurl falls back to curl for the API call (e.g., on 403 from Go HTTP).
func callAPIWithCurl(token string) (*apiUsageResponse, error) {
	curlPath, err := exec.LookPath("curl")
	if err != nil {
		return nil, fmt.Errorf("curl not found: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), apiTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, curlPath, "-s",
		"-H", "Authorization: Bearer "+token,
		"-H", "User-Agent: "+userAgent,
		"-H", "anthropic-beta: "+apiBeta,
		apiURL,
	)

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("curl failed: %w", err)
	}

	var result apiUsageResponse
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("curl response parse: %w", err)
	}
	return &result, nil
}

// cacheFilePath returns the file path for a cached API response.
func cacheFilePath(hash string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "cc-usage", "cache-"+hash+".json")
}

// readFileCache reads a cache entry from disk.
func readFileCache(hash string) *cacheEntry {
	data, err := os.ReadFile(cacheFilePath(hash))
	if err != nil {
		return nil
	}
	var entry cacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		debugLog("api", "file cache parse error: %v", err)
		return nil
	}
	return &entry
}

// writeFileCache writes a cache entry to disk.
func writeFileCache(hash string, entry *cacheEntry) {
	path := cacheFilePath(hash)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		debugLog("api", "cache dir create failed: %v", err)
		return
	}
	data, err := json.Marshal(entry)
	if err != nil {
		debugLog("api", "cache marshal failed: %v", err)
		return
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		debugLog("api", "cache write failed: %v", err)
	}
}

// parseUsageLimits converts an API response to the UsageLimits domain type.
func parseUsageLimits(resp *apiUsageResponse) *UsageLimits {
	if resp == nil {
		return nil
	}
	limits := &UsageLimits{}

	if entry := parseEntry(resp.FiveHour.Utilization, resp.FiveHour.ResetsAt); entry != nil {
		limits.FiveHour = entry
	}
	if entry := parseEntry(resp.SevenDay.Utilization, resp.SevenDay.ResetsAt); entry != nil {
		limits.SevenDay = entry
	}
	if entry := parseEntry(resp.SevenDaySonnet.Utilization, resp.SevenDaySonnet.ResetsAt); entry != nil {
		limits.SevenDaySonnet = entry
	}

	return limits
}

// parseEntry parses a single utilization/resets_at pair into a UsageLimitEntry.
func parseEntry(utilization int, resetsAt string) *UsageLimitEntry {
	if resetsAt == "" {
		return &UsageLimitEntry{Utilization: utilization}
	}
	t, err := time.Parse(time.RFC3339, resetsAt)
	if err != nil {
		debugLog("api", "parse resets_at failed: %v", err)
		return &UsageLimitEntry{Utilization: utilization}
	}
	return &UsageLimitEntry{Utilization: utilization, ResetsAt: t}
}

// cleanOldCaches removes cache files older than 1 hour. Fire-and-forget.
func cleanOldCaches() {
	now := time.Now()
	if now.Sub(lastCleanup) < time.Hour {
		return
	}
	lastCleanup = now

	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	pattern := filepath.Join(home, ".cache", "cc-usage", "cache-*.json")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return
	}
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) > time.Hour {
			_ = os.Remove(f)
		}
	}
}

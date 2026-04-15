package main

import (
	"encoding/json"
	"os"
)

// StdinInput represents the JSON payload from Claude Code's status line protocol.
type StdinInput struct {
	Model struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	} `json:"model"`

	Workspace struct {
		CurrentDir string   `json:"current_dir"`
		ProjectDir string   `json:"project_dir,omitempty"`
		AddedDirs  []string `json:"added_dirs,omitempty"`
	} `json:"workspace"`

	Worktree *struct {
		Name           string `json:"name"`
		Path           string `json:"path"`
		Branch         string `json:"branch"`
		OriginalCwd    string `json:"original_cwd"`
		OriginalBranch string `json:"original_branch"`
	} `json:"worktree,omitempty"`

	ContextWindow struct {
		TotalInputTokens    int  `json:"total_input_tokens"`
		TotalOutputTokens   int  `json:"total_output_tokens"`
		ContextWindowSize   int  `json:"context_window_size"`
		UsedPercentage      *int `json:"used_percentage,omitempty"`
		RemainingPercentage *int `json:"remaining_percentage,omitempty"`
		CurrentUsage        struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		} `json:"current_usage"`
	} `json:"context_window"`

	Cost struct {
		TotalCostUsd       float64 `json:"total_cost_usd"`
		TotalDurationMs    *int64  `json:"total_duration_ms,omitempty"`
		TotalApiDurationMs *int64  `json:"total_api_duration_ms,omitempty"`
		TotalLinesAdded    *int    `json:"total_lines_added,omitempty"`
		TotalLinesRemoved  *int    `json:"total_lines_removed,omitempty"`
	} `json:"cost"`

	RateLimits *struct {
		FiveHour *struct {
			UsedPercentage int   `json:"used_percentage"`
			ResetsAt       int64 `json:"resets_at"`
		} `json:"five_hour,omitempty"`
		SevenDay *struct {
			UsedPercentage int   `json:"used_percentage"`
			ResetsAt       int64 `json:"resets_at"`
		} `json:"seven_day,omitempty"`
	} `json:"rate_limits,omitempty"`

	OutputStyle *struct {
		Name string `json:"name,omitempty"`
	} `json:"output_style,omitempty"`

	Exceeds200kTokens bool   `json:"exceeds_200k_tokens,omitempty"`
	TranscriptPath    string `json:"transcript_path,omitempty"`
	Version           string `json:"version"`
	SessionId         string `json:"session_id,omitempty"`
	SessionName       string `json:"session_name,omitempty"`
	PermissionMode    string `json:"permission_mode,omitempty"`

	Vim *struct {
		Mode string `json:"mode"`
	} `json:"vim,omitempty"`

	Agent *struct {
		Name string `json:"name"`
	} `json:"agent,omitempty"`

	Remote *struct {
		SessionId string `json:"session_id"`
	} `json:"remote,omitempty"`

	AgentId   string `json:"agent_id,omitempty"`
	AgentType string `json:"agent_type,omitempty"`
}

// parseStdin reads and decodes JSON from stdin. Returns empty StdinInput on error.
func parseStdin() StdinInput {
	var input StdinInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		debugLog("stdin", "parse error: %v", err)
		return StdinInput{}
	}
	return input
}

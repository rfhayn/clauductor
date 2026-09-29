// Package signals reads what the panel watches: Claude Code's hook and status-line
// payloads, `claude agents --json`, `claude --version`, git's worktree list and gh's
// PR list, and the meaning of each notification type. It parses; it keeps no state.
package signals

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

// DetailMax caps a one-line summary (OneLine), in runes.
const DetailMax = 140

// HookEvent is the subset of a Claude Code hook payload the panel uses. Unknown
// fields are ignored by encoding/json and never stored: in particular the transcript
// path is deliberately not declared, because the panel never reads transcripts.
type HookEvent struct {
	SessionID            string `json:"session_id"`
	Cwd                  string `json:"cwd"`
	Event                string `json:"hook_event_name"`
	AgentID              string `json:"agent_id"`
	AgentType            string `json:"agent_type"`
	NotificationType     string `json:"notification_type"`
	Message              string `json:"message"`
	Title                string `json:"title"`
	Prompt               string `json:"prompt"`
	LastAssistantMessage string `json:"last_assistant_message"`
	Reason               string `json:"reason"`
	// v2 events. tool_input is deliberately not declared: it can hold secrets, and
	// the panel shows only which tool asks.
	ErrorType         string `json:"error_type"`         // StopFailure
	ToolName          string `json:"tool_name"`          // PermissionRequest
	CompactionTrigger string `json:"compaction_trigger"` // PreCompact / PostCompact
	Trigger           string `json:"trigger"`            // older spelling of compaction_trigger
	PreviousCwd       string `json:"previous_cwd"`       // CwdChanged
}

// ParseHook decodes a hook body.
func ParseHook(body []byte) (HookEvent, error) {
	var ev HookEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		return ev, err
	}
	if ev.Event == "" {
		return ev, fmt.Errorf("hook body has no hook_event_name")
	}
	return ev, nil
}

// StatusPayload is the subset of the statusLine stdin JSON the panel uses.
type StatusPayload struct {
	SessionID string `json:"session_id"`
	Version   string `json:"version"` // the Claude Code version that sent it
	Cwd       string `json:"cwd"`
	Workspace struct {
		CurrentDir string `json:"current_dir"`
	} `json:"workspace"`
	Model struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	} `json:"model"`
	Cost struct {
		TotalCostUSD *float64 `json:"total_cost_usd"`
		// PANEL-11: the lane metrics. Every figure below is from the same post the
		// panel already receives; none costs a model token or a new poll.
		TotalDurationMs    *int64 `json:"total_duration_ms"`
		TotalAPIDurationMs *int64 `json:"total_api_duration_ms"`
		TotalLinesAdded    *int64 `json:"total_lines_added"`
		TotalLinesRemoved  *int64 `json:"total_lines_removed"`
	} `json:"cost"`
	ContextWindow struct {
		UsedPercentage    *float64 `json:"used_percentage"`
		TotalInputTokens  *int64   `json:"total_input_tokens"`
		TotalOutputTokens *int64   `json:"total_output_tokens"`
		ContextWindowSize *int64   `json:"context_window_size"`
		CurrentUsage      *struct {
			InputTokens              int64 `json:"input_tokens"`
			OutputTokens             int64 `json:"output_tokens"`
			CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
		} `json:"current_usage"`
	} `json:"context_window"`
	Exceeds200k *bool `json:"exceeds_200k_tokens"`
	PromptCache *struct {
		Warm      *bool    `json:"warm"`
		HitRatio  *float64 `json:"hit_ratio"`
		ExpiresAt *int64   `json:"expires_at"` // unix seconds
		Requests  *int64   `json:"requests"`
		Misses    *int64   `json:"misses"`
	} `json:"prompt_cache"`
	FastMode *bool `json:"fast_mode"`
	Thinking *struct {
		Enabled *bool `json:"enabled"`
	} `json:"thinking"`
	OutputStyle struct {
		Name string `json:"name"`
	} `json:"output_style"`
	RateLimits struct {
		FiveHour *RateLimit `json:"five_hour"`
		SevenDay *RateLimit `json:"seven_day"`
	} `json:"rate_limits"`
}

// RateLimit is one quota window from the status line.
type RateLimit struct {
	UsedPercentage *float64 `json:"used_percentage"`
	ResetsAt       *int64   `json:"resets_at"`
}

// ParseStatus decodes a status-line body.
func ParseStatus(body []byte) (StatusPayload, error) {
	var s StatusPayload
	err := json.Unmarshal(body, &s)
	if err == nil && s.Cwd == "" {
		s.Cwd = s.Workspace.CurrentDir
	}
	return s, err
}

// OneLine collapses whitespace and cuts s to DetailMax runes: a summary for the
// feed, a label or a notification, never a body.
func OneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > DetailMax {
		r := []rune(s)
		s = string(r[:DetailMax]) + "…"
	}
	return s
}

// HookEvents are the events the panel subscribes to. SessionStart is absent on
// purpose: HTTP hooks do not fire for it (verified on Claude Code 2.1.284), so new
// sessions are found through `claude agents --json` instead.
//
// v2 adds StopFailure (its error_type says rate_limit), PermissionRequest, the
// compaction pair and CwdChanged. The panel only OBSERVES PermissionRequest (and
// PreCompact, which could block): /hook answers 204 with an empty body, which Claude
// Code documents as "no decision", so the permission flow proceeds unchanged. It
// never answers a permission request, because /hook takes no token.
var HookEvents = []string{"UserPromptSubmit", "Stop", "SubagentStart", "SubagentStop", "Notification", "SessionEnd",
	"StopFailure", "PermissionRequest", "PreCompact", "PostCompact", "CwdChanged"}

// Clip is OneLine cut further to n runes.
func Clip(s string, n int) string {
	s = OneLine(s)
	r := []rune(s)
	if len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

package prx

import (
	"time"
)

// WriteAccess constants for the Event.WriteAccess field.
const (
	WriteAccessNo         = -2 // User confirmed to not have write access
	WriteAccessUnlikely   = -1 // User unlikely to have write access (CONTRIBUTOR, NONE, etc.)
	WriteAccessNA         = 0  // Not applicable/not set (omitted from JSON)
	WriteAccessLikely     = 1  // User likely has write access but unable to confirm (MEMBER with 403 API response)
	WriteAccessDefinitely = 2  // User definitely has write access (OWNER, COLLABORATOR, or confirmed via API)
)

// Event represents a single event that occurred on a pull request.
// Each event captures who did what and when, with additional context depending on the event type.
type Event struct {
	Timestamp   time.Time `json:"timestamp"`
	Kind        string    `json:"kind"`
	Actor       string    `json:"actor"`
	Target      string    `json:"target,omitempty"`
	Outcome     string    `json:"outcome,omitempty"`
	Body        string    `json:"body,omitempty"`
	Description string    `json:"description,omitempty"`
	WriteAccess int       `json:"write_access,omitempty"`
	Bot         bool      `json:"bot,omitempty"`
	TargetIsBot bool      `json:"target_is_bot,omitempty"`
	Question    bool      `json:"question,omitempty"`
	Required    bool      `json:"required,omitempty"`
	Outdated    bool      `json:"outdated,omitempty"` // For review comments: indicates comment is on outdated code
}

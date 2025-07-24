package prx

import (
	"time"
)

// PullRequest represents a GitHub pull request with its essential metadata.
type PullRequest struct {
	Number         int    `json:"number"`
	Title          string `json:"title"`
	Body           string `json:"body"`
	State          string `json:"state"`           // "open", "closed"
	Draft          bool   `json:"draft"`           // Whether the PR is a draft
	Merged         bool   `json:"merged"`          // Whether the PR was merged
	Mergeable      *bool  `json:"mergeable"`       // Can be true, false, or null
	MergeableState string `json:"mergeable_state"` // "clean", "dirty", "blocked", "unstable", "unknown"

	// Timestamps
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	ClosedAt  *time.Time `json:"closed_at,omitempty"`
	MergedAt  *time.Time `json:"merged_at,omitempty"`

	// Users
	Author            string `json:"author"`
	AuthorAssociation string `json:"author_association"` // OWNER, MEMBER, COLLABORATOR, CONTRIBUTOR, etc.
	AuthorBot         bool   `json:"author_bot"`         // Whether the PR author is a bot
	MergedBy          string `json:"merged_by,omitempty"`

	// PR Size
	Additions    int `json:"additions"`     // Lines added
	Deletions    int `json:"deletions"`     // Lines removed
	ChangedFiles int `json:"changed_files"` // Number of files changed

	// Current State
	Assignees          []string `json:"assignees,omitempty"`           // Current assignees
	RequestedReviewers []string `json:"requested_reviewers,omitempty"` // Users with pending review requests
	Labels             []string `json:"labels,omitempty"`              // Current labels

	// Test Summary
	TestSummary *TestSummary `json:"test_summary,omitempty"` // Summary of test results

	// Status Summary
	StatusSummary *StatusSummary `json:"status_summary,omitempty"` // Summary of all checks (status_check + check_run)
}

// TestSummary contains aggregate test result counts
type TestSummary struct {
	Passing int `json:"passing"` // Number of passing tests
	Failing int `json:"failing"` // Number of failing tests
	Pending int `json:"pending"` // Number of pending tests
}

// StatusSummary contains aggregate status check and check run counts
type StatusSummary struct {
	Success int `json:"success"` // Number of successful checks
	Failure int `json:"failure"` // Number of failed checks
	Pending int `json:"pending"` // Number of pending checks
	Neutral int `json:"neutral"` // Number of neutral checks (skipped, cancelled, etc.)
}

// PullRequestData contains a pull request and all its associated events.
type PullRequestData struct {
	PullRequest PullRequest `json:"pull_request"`
	Events      []Event     `json:"events"`
}

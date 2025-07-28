package prx

import (
	"time"
)

// PullRequest represents a GitHub pull request with its essential metadata.
type PullRequest struct {
	// Basic Information
	Number int    `json:"number"` // PR number (e.g., 1773)
	Title  string `json:"title"`  // PR title
	Body   string `json:"body"`   // PR description (truncated to 256 chars)
	Author string `json:"author"` // GitHub username of the PR author
	
	// Status Information
	State          string `json:"state"`           // Current state: "open" or "closed"
	Draft          bool   `json:"draft"`           // True if this is a draft PR
	Merged         bool   `json:"merged"`          // True if the PR was merged
	MergedBy       string `json:"merged_by,omitempty"` // Username who merged the PR
	Mergeable      *bool  `json:"mergeable"`       // GitHub's assessment: true, false, or null (still computing)
	MergeableState string `json:"mergeable_state"` // Details: "clean", "dirty", "blocked", "unstable", "unknown"
	
	// Timestamps
	CreatedAt time.Time  `json:"created_at"`          // When the PR was created
	UpdatedAt time.Time  `json:"updated_at"`          // Last activity on the PR
	ClosedAt  *time.Time `json:"closed_at,omitempty"` // When the PR was closed (nil if still open)
	MergedAt  *time.Time `json:"merged_at,omitempty"` // When the PR was merged (nil if not merged)
	
	// Code Changes
	Additions    int `json:"additions"`     // Total lines added
	Deletions    int `json:"deletions"`     // Total lines removed
	ChangedFiles int `json:"changed_files"` // Number of files modified
	
	// People & Permissions
	AuthorBot            bool     `json:"author_bot"`             // True if author is a bot account
	AuthorHasWriteAccess bool     `json:"author_has_write_access"` // True if author can merge
	Assignees            []string `json:"assignees,omitempty"`    // Current assignees
	RequestedReviewers   []string `json:"requested_reviewers,omitempty"` // Pending review requests
	
	// Organization
	Labels []string `json:"labels,omitempty"` // Applied labels
	
	// Aggregated Summaries (computed from events)
	TestSummary     *TestSummary     `json:"test_summary,omitempty"`     // Test results summary
	StatusSummary   *StatusSummary   `json:"status_summary,omitempty"`   // All checks summary
	ApprovalSummary *ApprovalSummary `json:"approval_summary,omitempty"` // Review approvals summary
}

// TestSummary aggregates test results from check runs
type TestSummary struct {
	Passing int `json:"passing"` // Tests that completed successfully
	Failing int `json:"failing"` // Tests that failed or timed out
	Pending int `json:"pending"` // Tests that are still running or queued
}

// StatusSummary aggregates all status checks and check runs
type StatusSummary struct {
	Success int `json:"success"` // Checks that completed successfully
	Failure int `json:"failure"` // Checks that failed, errored, or require action
	Pending int `json:"pending"` // Checks that are queued or in progress
	Neutral int `json:"neutral"` // Checks that were cancelled, skipped, or neutral
}

// ApprovalSummary tracks PR review approvals and change requests
type ApprovalSummary struct {
	// Approvals from users confirmed to have write access (owners, collaborators, members with confirmed access)
	ApprovalsWithWriteAccess int `json:"approvals_with_write_access"`
	
	// Approvals from users without confirmed write access (contributors, unconfirmed members, etc.)
	ApprovalsWithoutWriteAccess int `json:"approvals_without_write_access"`
	
	// Outstanding change requests from any reviewer
	ChangesRequested int `json:"changes_requested"`
}

// PullRequestData contains a pull request and all its associated events.
type PullRequestData struct {
	PullRequest PullRequest `json:"pull_request"`
	Events      []Event     `json:"events"`
}

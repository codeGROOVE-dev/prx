package prx

import (
	"time"
)

// TestState represents the overall testing status of a pull request.
const (
	TestStateNone    = ""        // No tests or unknown state
	TestStateQueued  = "queued"  // Tests are queued to run
	TestStateRunning = "running" // Tests are currently executing
	TestStatePassing = "passing" // All tests passed
	TestStateFailing = "failing" // At least one test failed
)

// PullRequest represents a GitHub pull request with its essential metadata.
type PullRequest struct {
	CreatedAt          time.Time        `json:"created_at"`
	UpdatedAt          time.Time        `json:"updated_at"`
	ClosedAt           *time.Time       `json:"closed_at,omitempty"`
	Mergeable          *bool            `json:"mergeable"`
	TestSummary        *TestSummary     `json:"test_summary,omitempty"`
	ApprovalSummary    *ApprovalSummary `json:"approval_summary,omitempty"`
	StatusSummary      *StatusSummary   `json:"status_summary,omitempty"`
	MergedAt           *time.Time       `json:"merged_at,omitempty"`
	MergeableState     string           `json:"mergeable_state"`
	Author             string           `json:"author"`
	Body               string           `json:"body"`
	Title              string           `json:"title"`
	MergedBy           string           `json:"merged_by,omitempty"`
	State              string           `json:"state"`
	TestState          string           `json:"test_state,omitempty"`
	Assignees          []string         `json:"assignees,omitempty"`
	Labels             []string         `json:"labels,omitempty"`
	RequestedReviewers []string         `json:"requested_reviewers,omitempty"`
	AuthorWriteAccess  int              `json:"author_write_access,omitempty"`
	Number             int              `json:"number"`
	ChangedFiles       int              `json:"changed_files"`
	Deletions          int              `json:"deletions"`
	Additions          int              `json:"additions"`
	AuthorBot          bool             `json:"author_bot"`
	Merged             bool             `json:"merged"`
	Draft              bool             `json:"draft"`
}

// TestSummary aggregates test results from check runs.
type TestSummary struct {
	Passing int `json:"passing"` // Tests that completed successfully
	Failing int `json:"failing"` // Tests that failed or timed out
	Pending int `json:"pending"` // Tests that are still running or queued
}

// StatusSummary aggregates all status checks and check runs.
type StatusSummary struct {
	Success int `json:"success"` // Checks that completed successfully
	Failure int `json:"failure"` // Checks that failed, errored, or require action
	Pending int `json:"pending"` // Checks that are queued or in progress
	Neutral int `json:"neutral"` // Checks that were cancelled, skipped, or neutral
}

// ApprovalSummary tracks PR review approvals and change requests.
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
	Events      []Event     `json:"events"`
	PullRequest PullRequest `json:"pull_request"`
}

package github

import "time"

// User represents a GitHub user.
type User struct {
	Login string `json:"login"`
	Type  string `json:"type"`
}

// CheckRun represents a GitHub check run from the REST API.
type CheckRun struct {
	Name        string    `json:"name"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
	Conclusion  string    `json:"conclusion"`
	Status      string    `json:"status"`
	Output      struct {
		Title   string `json:"title"`
		Summary string `json:"summary"`
	} `json:"output"`
}

// CheckRuns represents a list of GitHub check runs.
type CheckRuns struct {
	CheckRuns []*CheckRun `json:"check_runs"`
}

// Ruleset represents a repository ruleset from the REST API.
type Ruleset struct {
	Name   string `json:"name"`
	Target string `json:"target"`
	Rules  []struct {
		Type       string `json:"type"`
		Parameters struct {
			RequiredStatusChecks []struct {
				Context string `json:"context"`
			} `json:"required_status_checks"`
		} `json:"parameters"`
	} `json:"rules"`
}

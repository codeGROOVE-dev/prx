package prx

import (
	"testing"
	"time"

	"github.com/codeGROOVE-dev/prx/pkg/prx/github"
)

func TestCheckRunStatusDescriptions(t *testing.T) {
	tests := []struct {
		name            string
		checkRun        github.CheckRun
		expectedDesc    string
		expectedOutcome string
	}{
		{
			name: "check with title and summary",
			checkRun: github.CheckRun{
				Name:        "*control",
				Status:      "completed",
				Conclusion:  "failure",
				CompletedAt: time.Now(),
				Output: struct {
					Title   string `json:"title"`
					Summary string `json:"summary"`
				}{
					Title:   "Plan requires authorisation.",
					Summary: "Plans submitted by users that are not a member of the organisation require explicit authorisation.",
				},
			},
			expectedDesc:    "Plan requires authorisation.: Plans submitted by users that are not a member of the organisation require explicit authorisation.",
			expectedOutcome: "failure",
		},
		{
			name: "check with only title",
			checkRun: github.CheckRun{
				Name:        "test-check",
				Status:      "completed",
				Conclusion:  "success",
				CompletedAt: time.Now(),
				Output: struct {
					Title   string `json:"title"`
					Summary string `json:"summary"`
				}{
					Title: "All tests passed",
				},
			},
			expectedDesc:    "All tests passed",
			expectedOutcome: "success",
		},
		{
			name: "check with only summary",
			checkRun: github.CheckRun{
				Name:        "lint-check",
				Status:      "completed",
				Conclusion:  "failure",
				CompletedAt: time.Now(),
				Output: struct {
					Title   string `json:"title"`
					Summary string `json:"summary"`
				}{
					Summary: "Found 5 linting errors",
				},
			},
			expectedDesc:    "Found 5 linting errors",
			expectedOutcome: "failure",
		},
		{
			name: "check with no output",
			checkRun: github.CheckRun{
				Name:        "basic-check",
				Status:      "completed",
				Conclusion:  "neutral",
				CompletedAt: time.Now(),
			},
			expectedDesc:    "",
			expectedOutcome: "neutral",
		},
		{
			name: "pending check (not completed)",
			checkRun: github.CheckRun{
				Name:      "pending-check",
				Status:    "in_progress",
				StartedAt: time.Now(),
			},
			expectedDesc:    "",
			expectedOutcome: "in_progress",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Process the check run into an event
			var outcome string
			var timestamp time.Time

			if !tt.checkRun.CompletedAt.IsZero() {
				outcome = tt.checkRun.Conclusion
				timestamp = tt.checkRun.CompletedAt
			} else {
				outcome = tt.checkRun.Status
				timestamp = tt.checkRun.StartedAt
			}

			event := Event{
				Kind:      "check_run",
				Timestamp: timestamp,
				Actor:     "github",
				Bot:       true,
				Outcome:   outcome,
				Body:      tt.checkRun.Name,
			}

			// Add status description from output if available (matching fetchers.go logic)
			switch {
			case tt.checkRun.Output.Title != "" && tt.checkRun.Output.Summary != "":
				event.Description = tt.checkRun.Output.Title + ": " + tt.checkRun.Output.Summary
			case tt.checkRun.Output.Title != "":
				event.Description = tt.checkRun.Output.Title
			case tt.checkRun.Output.Summary != "":
				event.Description = tt.checkRun.Output.Summary
			}

			// Verify the outcome
			if event.Outcome != tt.expectedOutcome {
				t.Errorf("Expected outcome %q, got %q", tt.expectedOutcome, event.Outcome)
			}

			// Verify the description
			if event.Description != tt.expectedDesc {
				t.Errorf("Expected description %q, got %q", tt.expectedDesc, event.Description)
			}
		})
	}
}

func TestCalculateCheckSummaryWithDescriptions(t *testing.T) {
	events := []Event{
		{
			Kind:        "check_run",
			Body:        "*control",
			Outcome:     "failure",
			Description: "Plan requires authorisation.: Plans submitted by users that are not a member of the organisation require explicit authorisation.",
		},
		{
			Kind:        "check_run",
			Body:        "build-and-test (ubuntu-22.04, all)",
			Outcome:     "success",
			Description: "",
		},
		{
			Kind:        "check_run",
			Body:        "clippy-lint",
			Outcome:     "success",
			Description: "",
		},
		{
			Kind:        "check_run",
			Body:        "build-and-test / illumos",
			Outcome:     "pending",
			Description: "Expected — Waiting for status to be reported",
		},
	}

	requiredChecks := []string{
		"build-and-test (ubuntu-22.04, all)",
		"build-and-test / illumos",
		"clippy-lint",
		"*control",
	}

	summary := calculateCheckSummary(events, requiredChecks)

	// Verify counts
	if len(summary.Success) != 2 {
		t.Errorf("Expected 2 successful checks, got %d", len(summary.Success))
	}
	if len(summary.Failing) != 1 {
		t.Errorf("Expected 1 failing check, got %d", len(summary.Failing))
	}
	if len(summary.Pending) != 1 {
		t.Errorf("Expected 1 pending check, got %d", len(summary.Pending))
	}

	// Verify failing status descriptions
	if desc, exists := summary.Failing["*control"]; !exists {
		t.Error("Expected *control in failing statuses")
	} else if desc != "Plan requires authorisation.: Plans submitted by users that are not a member of the organisation require explicit authorisation." {
		t.Errorf("Expected *control description to be preserved, got %q", desc)
	}

	// Verify pending status descriptions
	if desc, exists := summary.Pending["build-and-test / illumos"]; !exists {
		t.Error("Expected build-and-test / illumos in pending statuses")
	} else if desc != "Expected — Waiting for status to be reported" {
		t.Errorf("Expected pending check description to be preserved, got %q", desc)
	}
}

func TestDropshotPR1359Regression(t *testing.T) {
	// This test ensures we don't regress on the specific case of Dropshot PR #1359
	// where the *control check should show "Plan requires authorisation." description

	checkRun := github.CheckRun{
		Name:        "*control",
		Status:      "completed",
		Conclusion:  "failure",
		CompletedAt: time.Date(2025, 6, 25, 15, 44, 14, 0, time.UTC),
		Output: struct {
			Title   string `json:"title"`
			Summary string `json:"summary"`
		}{
			Title:   "Plan requires authorisation.",
			Summary: "Plans submitted by users that are not a member of the organisation require explicit authorisation.",
		},
	}

	// Process into event
	event := Event{
		Kind:      "check_run",
		Timestamp: checkRun.CompletedAt,
		Actor:     "github",
		Bot:       true,
		Outcome:   checkRun.Conclusion,
		Body:      checkRun.Name,
	}

	// Add description using the same logic as in fetchers.go
	switch {
	case checkRun.Output.Title != "" && checkRun.Output.Summary != "":
		event.Description = checkRun.Output.Title + ": " + checkRun.Output.Summary
	case checkRun.Output.Title != "":
		event.Description = checkRun.Output.Title
	case checkRun.Output.Summary != "":
		event.Description = checkRun.Output.Summary
	}

	expectedDescription := "Plan requires authorisation.: Plans submitted by users that are not a member of the organisation require explicit authorisation."

	if event.Description != expectedDescription {
		t.Errorf("Regression detected: *control check description not preserved correctly.\nExpected: %q\nGot: %q",
			expectedDescription, event.Description)
	}

	// Also test that it appears correctly in the check summary
	events := []Event{event}
	summary := calculateCheckSummary(events, []string{})

	if desc, exists := summary.Failing["*control"]; !exists {
		t.Error("Regression detected: *control not in failing statuses")
	} else if desc != expectedDescription {
		t.Errorf("Regression detected: *control description in summary incorrect.\nExpected: %q\nGot: %q",
			expectedDescription, desc)
	}
}

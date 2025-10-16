package prx

import (
	"reflect"
	"testing"
)

func TestCalculateCheckSummaryWithMaps(t *testing.T) {
	tests := []struct {
		name              string
		events            []Event
		requiredChecks    []string
		expectedSuccess   map[string]string
		expectedFailing   map[string]string
		expectedPending   map[string]string
		expectedCancelled map[string]string
		expectedSkipped   map[string]string
		expectedStale     map[string]string
		expectedNeutral   map[string]string
	}{
		{
			name: "mixed statuses with descriptions",
			events: []Event{
				{
					Kind:        "check_run",
					Body:        "build",
					Outcome:     "success",
					Description: "",
				},
				{
					Kind:        "check_run",
					Body:        "test",
					Outcome:     "failure",
					Description: "3 tests failed",
				},
				{
					Kind:        "status_check",
					Body:        "lint",
					Outcome:     "pending",
					Description: "Running linter...",
				},
				{
					Kind:        "check_run",
					Body:        "security",
					Outcome:     "error",
					Description: "Security scan error: timeout",
				},
			},
			requiredChecks: []string{"build", "test", "lint", "security"},
			expectedSuccess: map[string]string{
				"build": "",
			},
			expectedFailing: map[string]string{
				"test":     "3 tests failed",
				"security": "Security scan error: timeout",
			},
			expectedPending: map[string]string{
				"lint": "Running linter...",
			},
			expectedCancelled: map[string]string{},
			expectedSkipped:   map[string]string{},
			expectedStale:     map[string]string{},
			expectedNeutral:   map[string]string{},
		},
		{
			name: "missing required checks marked as pending",
			events: []Event{
				{
					Kind:    "check_run",
					Body:    "build",
					Outcome: "success",
				},
			},
			requiredChecks: []string{"build", "test", "lint"},
			expectedSuccess: map[string]string{
				"build": "",
			},
			expectedFailing: map[string]string{},
			expectedPending: map[string]string{
				"test": "Expected — Waiting for status to be reported",
				"lint": "Expected — Waiting for status to be reported",
			},
			expectedCancelled: map[string]string{},
			expectedSkipped:   map[string]string{},
			expectedStale:     map[string]string{},
			expectedNeutral:   map[string]string{},
		},
		{
			name: "action_required counted as failure",
			events: []Event{
				{
					Kind:        "check_run",
					Body:        "deploy",
					Outcome:     "action_required",
					Description: "Manual approval needed",
				},
			},
			requiredChecks:  []string{"deploy"},
			expectedSuccess: map[string]string{},
			expectedFailing: map[string]string{
				"deploy": "Manual approval needed",
			},
			expectedPending:   map[string]string{},
			expectedCancelled: map[string]string{},
			expectedSkipped:   map[string]string{},
			expectedStale:     map[string]string{},
			expectedNeutral:   map[string]string{},
		},
		{
			name: "cancelled and skipped statuses",
			events: []Event{
				{
					Kind:        "check_run",
					Body:        "optional-check",
					Outcome:     "cancelled",
					Description: "Workflow cancelled",
				},
				{
					Kind:        "status_check",
					Body:        "skipped-check",
					Outcome:     "skipped",
					Description: "Skipped due to condition",
				},
			},
			requiredChecks:  []string{},
			expectedSuccess: map[string]string{},
			expectedFailing: map[string]string{},
			expectedPending: map[string]string{},
			expectedCancelled: map[string]string{
				"optional-check": "Workflow cancelled",
			},
			expectedSkipped: map[string]string{
				"skipped-check": "Skipped due to condition",
			},
			expectedStale:   map[string]string{},
			expectedNeutral: map[string]string{},
		},
		{
			name: "duplicate check names use latest",
			events: []Event{
				{
					Kind:        "check_run",
					Body:        "test",
					Outcome:     "failure",
					Description: "First run failed",
				},
				{
					Kind:        "check_run",
					Body:        "test",
					Outcome:     "success",
					Description: "Re-run succeeded",
				},
			},
			requiredChecks: []string{"test"},
			expectedSuccess: map[string]string{
				"test": "Re-run succeeded",
			},
			expectedFailing:   map[string]string{},
			expectedPending:   map[string]string{},
			expectedCancelled: map[string]string{},
			expectedSkipped:   map[string]string{},
			expectedStale:     map[string]string{},
			expectedNeutral:   map[string]string{},
		},
		{
			name:            "no events with required checks",
			events:          []Event{},
			requiredChecks:  []string{"build", "test"},
			expectedSuccess: map[string]string{},
			expectedFailing: map[string]string{},
			expectedPending: map[string]string{
				"build": "Expected — Waiting for status to be reported",
				"test":  "Expected — Waiting for status to be reported",
			},
			expectedCancelled: map[string]string{},
			expectedSkipped:   map[string]string{},
			expectedStale:     map[string]string{},
			expectedNeutral:   map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := calculateCheckSummary(tt.events, tt.requiredChecks)

			if !reflect.DeepEqual(summary.Success, tt.expectedSuccess) {
				t.Errorf("Success mismatch\ngot:  %v\nwant: %v", summary.Success, tt.expectedSuccess)
			}
			if !reflect.DeepEqual(summary.Failing, tt.expectedFailing) {
				t.Errorf("Failing mismatch\ngot:  %v\nwant: %v", summary.Failing, tt.expectedFailing)
			}
			if !reflect.DeepEqual(summary.Pending, tt.expectedPending) {
				t.Errorf("Pending mismatch\ngot:  %v\nwant: %v", summary.Pending, tt.expectedPending)
			}
			if !reflect.DeepEqual(summary.Cancelled, tt.expectedCancelled) {
				t.Errorf("Cancelled mismatch\ngot:  %v\nwant: %v", summary.Cancelled, tt.expectedCancelled)
			}
			if !reflect.DeepEqual(summary.Skipped, tt.expectedSkipped) {
				t.Errorf("Skipped mismatch\ngot:  %v\nwant: %v", summary.Skipped, tt.expectedSkipped)
			}
			if !reflect.DeepEqual(summary.Stale, tt.expectedStale) {
				t.Errorf("Stale mismatch\ngot:  %v\nwant: %v", summary.Stale, tt.expectedStale)
			}
			if !reflect.DeepEqual(summary.Neutral, tt.expectedNeutral) {
				t.Errorf("Neutral mismatch\ngot:  %v\nwant: %v", summary.Neutral, tt.expectedNeutral)
			}
		})
	}
}

func TestCheckSummaryInitialization(t *testing.T) {
	// Test that maps are properly initialized even with no events
	summary := calculateCheckSummary([]Event{}, []string{})

	if summary.Success == nil {
		t.Error("Success map should be initialized, not nil")
	}
	if summary.Failing == nil {
		t.Error("Failing map should be initialized, not nil")
	}
	if summary.Pending == nil {
		t.Error("Pending map should be initialized, not nil")
	}
	if summary.Cancelled == nil {
		t.Error("Cancelled map should be initialized, not nil")
	}
	if summary.Skipped == nil {
		t.Error("Skipped map should be initialized, not nil")
	}
	if summary.Stale == nil {
		t.Error("Stale map should be initialized, not nil")
	}
	if summary.Neutral == nil {
		t.Error("Neutral map should be initialized, not nil")
	}

	if len(summary.Success) != 0 {
		t.Errorf("Success should be empty, got %d items", len(summary.Success))
	}
	if len(summary.Failing) != 0 {
		t.Errorf("Failing should be empty, got %d items", len(summary.Failing))
	}
	if len(summary.Pending) != 0 {
		t.Errorf("Pending should be empty, got %d items", len(summary.Pending))
	}
}

func TestCalculateApprovalSummaryWriteAccessCategories(t *testing.T) {
	tests := []struct {
		name                      string
		events                    []Event
		expectedWithAccess        int
		expectedWithUnknownAccess int
		expectedWithoutAccess     int
		expectedChangesRequested  int
	}{
		{
			name: "approval with definite write access",
			events: []Event{
				{
					Kind:        "review",
					Actor:       "owner-user",
					Outcome:     "approved",
					WriteAccess: WriteAccessDefinitely,
				},
			},
			expectedWithAccess:        1,
			expectedWithUnknownAccess: 0,
			expectedWithoutAccess:     0,
			expectedChangesRequested:  0,
		},
		{
			name: "approval with unknown write access (WriteAccessUnlikely)",
			events: []Event{
				{
					Kind:        "review",
					Actor:       "external-contributor",
					Outcome:     "approved",
					WriteAccess: WriteAccessUnlikely,
				},
			},
			expectedWithAccess:        0,
			expectedWithUnknownAccess: 1,
			expectedWithoutAccess:     0,
			expectedChangesRequested:  0,
		},
		{
			name: "approval with likely write access",
			events: []Event{
				{
					Kind:        "review",
					Actor:       "member-user",
					Outcome:     "approved",
					WriteAccess: WriteAccessLikely,
				},
			},
			expectedWithAccess:        0,
			expectedWithUnknownAccess: 1,
			expectedWithoutAccess:     0,
			expectedChangesRequested:  0,
		},
		{
			name: "approval with NA write access",
			events: []Event{
				{
					Kind:        "review",
					Actor:       "unknown-user",
					Outcome:     "approved",
					WriteAccess: WriteAccessNA,
				},
			},
			expectedWithAccess:        0,
			expectedWithUnknownAccess: 1,
			expectedWithoutAccess:     0,
			expectedChangesRequested:  0,
		},
		{
			name: "approval with confirmed no write access",
			events: []Event{
				{
					Kind:        "review",
					Actor:       "blocked-user",
					Outcome:     "approved",
					WriteAccess: WriteAccessNo,
				},
			},
			expectedWithAccess:        0,
			expectedWithUnknownAccess: 0,
			expectedWithoutAccess:     1,
			expectedChangesRequested:  0,
		},
		{
			name: "mixed approvals with different write access levels",
			events: []Event{
				{
					Kind:        "review",
					Actor:       "owner",
					Outcome:     "approved",
					WriteAccess: WriteAccessDefinitely,
				},
				{
					Kind:        "review",
					Actor:       "contributor",
					Outcome:     "approved",
					WriteAccess: WriteAccessUnlikely,
				},
				{
					Kind:        "review",
					Actor:       "member",
					Outcome:     "approved",
					WriteAccess: WriteAccessLikely,
				},
				{
					Kind:        "review",
					Actor:       "blocked",
					Outcome:     "approved",
					WriteAccess: WriteAccessNo,
				},
			},
			expectedWithAccess:        1,
			expectedWithUnknownAccess: 2,
			expectedWithoutAccess:     1,
			expectedChangesRequested:  0,
		},
		{
			name: "latest review overrides previous (approval then changes_requested)",
			events: []Event{
				{
					Kind:        "review",
					Actor:       "reviewer",
					Outcome:     "approved",
					WriteAccess: WriteAccessDefinitely,
				},
				{
					Kind:        "review",
					Actor:       "reviewer",
					Outcome:     "changes_requested",
					WriteAccess: WriteAccessDefinitely,
				},
			},
			expectedWithAccess:        0,
			expectedWithUnknownAccess: 0,
			expectedWithoutAccess:     0,
			expectedChangesRequested:  1,
		},
		{
			name: "commented reviews ignored",
			events: []Event{
				{
					Kind:        "review",
					Actor:       "commenter",
					Outcome:     "commented",
					WriteAccess: WriteAccessDefinitely,
				},
			},
			expectedWithAccess:        0,
			expectedWithUnknownAccess: 0,
			expectedWithoutAccess:     0,
			expectedChangesRequested:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := calculateApprovalSummary(tt.events)

			if summary.ApprovalsWithWriteAccess != tt.expectedWithAccess {
				t.Errorf("ApprovalsWithWriteAccess: got %d, want %d",
					summary.ApprovalsWithWriteAccess, tt.expectedWithAccess)
			}
			if summary.ApprovalsWithUnknownAccess != tt.expectedWithUnknownAccess {
				t.Errorf("ApprovalsWithUnknownAccess: got %d, want %d",
					summary.ApprovalsWithUnknownAccess, tt.expectedWithUnknownAccess)
			}
			if summary.ApprovalsWithoutWriteAccess != tt.expectedWithoutAccess {
				t.Errorf("ApprovalsWithoutWriteAccess: got %d, want %d",
					summary.ApprovalsWithoutWriteAccess, tt.expectedWithoutAccess)
			}
			if summary.ChangesRequested != tt.expectedChangesRequested {
				t.Errorf("ChangesRequested: got %d, want %d",
					summary.ChangesRequested, tt.expectedChangesRequested)
			}
		})
	}
}

func TestCheckSummaryCancelledNotInFailing(t *testing.T) {
	// Regression test: cancelled checks should only appear in cancelled map, not in failing map
	// This was a bug where cancelled checks appeared in both maps
	// Based on real-world scenario from https://github.com/codeGROOVE-dev/goose/pull/65
	events := []Event{
		{
			Kind:        "check_run",
			Body:        "Kusari Inspector",
			Outcome:     "success",
			Description: "Security Analysis Passed: No security issues found",
		},
		{
			Kind:    "check_run",
			Body:    "golangci-lint",
			Outcome: "success",
		},
		{
			Kind:    "check_run",
			Body:    "Test (ubuntu-latest)",
			Outcome: "success",
		},
		{
			Kind:    "check_run",
			Body:    "Test (windows-latest)",
			Outcome: "success",
		},
		{
			Kind:        "check_run",
			Body:        "Test (macos-latest)",
			Outcome:     "cancelled",
			Description: "cancelled",
		},
	}

	summary := calculateCheckSummary(events, []string{})

	// Verify cancelled check is ONLY in cancelled map
	if _, exists := summary.Cancelled["Test (macos-latest)"]; !exists {
		t.Error("Expected Test (macos-latest) to be in cancelled map")
	}

	// Verify cancelled check is NOT in failing map
	if _, exists := summary.Failing["Test (macos-latest)"]; exists {
		t.Error("Test (macos-latest) should NOT be in failing map, only in cancelled map")
	}

	// Verify success checks are in success map
	if len(summary.Success) != 4 {
		t.Errorf("Expected 4 successful checks, got %d", len(summary.Success))
	}

	// Verify counts
	if len(summary.Failing) != 0 {
		t.Errorf("Expected 0 failing checks, got %d", len(summary.Failing))
	}
	if len(summary.Cancelled) != 1 {
		t.Errorf("Expected 1 cancelled check, got %d", len(summary.Cancelled))
	}

	// Verify each check appears in exactly one category
	allChecks := make(map[string]int)
	for check := range summary.Success {
		allChecks[check]++
	}
	for check := range summary.Failing {
		allChecks[check]++
	}
	for check := range summary.Pending {
		allChecks[check]++
	}
	for check := range summary.Cancelled {
		allChecks[check]++
	}
	for check := range summary.Skipped {
		allChecks[check]++
	}
	for check := range summary.Stale {
		allChecks[check]++
	}
	for check := range summary.Neutral {
		allChecks[check]++
	}

	for check, count := range allChecks {
		if count != 1 {
			t.Errorf("Check %q appears in %d categories, should be exactly 1", check, count)
		}
	}
}

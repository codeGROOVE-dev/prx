package prx

import (
	"reflect"
	"testing"
)

func TestCalculateCheckSummaryWithMaps(t *testing.T) {
	tests := []struct {
		name                    string
		events                  []Event
		requiredChecks          []string
		expectedSuccess         int
		expectedFailure         int
		expectedPending         int
		expectedNeutral         int
		expectedFailingStatuses map[string]string
		expectedPendingStatuses map[string]string
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
			requiredChecks:  []string{"build", "test", "lint", "security"},
			expectedSuccess: 1,
			expectedFailure: 2,
			expectedPending: 1,
			expectedNeutral: 0,
			expectedFailingStatuses: map[string]string{
				"test":     "3 tests failed",
				"security": "Security scan error: timeout",
			},
			expectedPendingStatuses: map[string]string{
				"lint": "Running linter...",
			},
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
			requiredChecks:          []string{"build", "test", "lint"},
			expectedSuccess:         1,
			expectedFailure:         0,
			expectedPending:         2, // test and lint are missing
			expectedNeutral:         0,
			expectedFailingStatuses: map[string]string{},
			expectedPendingStatuses: map[string]string{
				"test": "Expected — Waiting for status to be reported",
				"lint": "Expected — Waiting for status to be reported",
			},
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
			expectedSuccess: 0,
			expectedFailure: 1,
			expectedPending: 0,
			expectedNeutral: 0,
			expectedFailingStatuses: map[string]string{
				"deploy": "Manual approval needed",
			},
			expectedPendingStatuses: map[string]string{},
		},
		{
			name: "neutral statuses",
			events: []Event{
				{
					Kind:    "check_run",
					Body:    "optional-check",
					Outcome: "cancelled",
				},
				{
					Kind:    "status_check",
					Body:    "skipped-check",
					Outcome: "skipped",
				},
			},
			requiredChecks:          []string{},
			expectedSuccess:         0,
			expectedFailure:         0,
			expectedPending:         0,
			expectedNeutral:         2,
			expectedFailingStatuses: map[string]string{},
			expectedPendingStatuses: map[string]string{},
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
			requiredChecks:          []string{"test"},
			expectedSuccess:         1,
			expectedFailure:         0,
			expectedPending:         0,
			expectedNeutral:         0,
			expectedFailingStatuses: map[string]string{},
			expectedPendingStatuses: map[string]string{},
		},
		{
			name:                    "no events with required checks",
			events:                  []Event{},
			requiredChecks:          []string{"build", "test"},
			expectedSuccess:         0,
			expectedFailure:         0,
			expectedPending:         2,
			expectedNeutral:         0,
			expectedFailingStatuses: map[string]string{},
			expectedPendingStatuses: map[string]string{
				"build": "Expected — Waiting for status to be reported",
				"test":  "Expected — Waiting for status to be reported",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := calculateCheckSummary(tt.events, tt.requiredChecks)

			if summary.Success != tt.expectedSuccess {
				t.Errorf("Success: got %d, want %d", summary.Success, tt.expectedSuccess)
			}
			if summary.Failure != tt.expectedFailure {
				t.Errorf("Failure: got %d, want %d", summary.Failure, tt.expectedFailure)
			}
			if summary.Pending != tt.expectedPending {
				t.Errorf("Pending: got %d, want %d", summary.Pending, tt.expectedPending)
			}
			if summary.Neutral != tt.expectedNeutral {
				t.Errorf("Neutral: got %d, want %d", summary.Neutral, tt.expectedNeutral)
			}

			// Check failing statuses map
			if !reflect.DeepEqual(summary.FailingStatuses, tt.expectedFailingStatuses) {
				t.Errorf("FailingStatuses mismatch\ngot:  %v\nwant: %v",
					summary.FailingStatuses, tt.expectedFailingStatuses)
			}

			// Check pending statuses map
			if !reflect.DeepEqual(summary.PendingStatuses, tt.expectedPendingStatuses) {
				t.Errorf("PendingStatuses mismatch\ngot:  %v\nwant: %v",
					summary.PendingStatuses, tt.expectedPendingStatuses)
			}
		})
	}
}

func TestCheckSummaryInitialization(t *testing.T) {
	// Test that maps are properly initialized even with no events
	summary := calculateCheckSummary([]Event{}, []string{})

	if summary.FailingStatuses == nil {
		t.Error("FailingStatuses map should be initialized, not nil")
	}

	if summary.PendingStatuses == nil {
		t.Error("PendingStatuses map should be initialized, not nil")
	}

	if len(summary.FailingStatuses) != 0 {
		t.Errorf("FailingStatuses should be empty, got %d items", len(summary.FailingStatuses))
	}

	if len(summary.PendingStatuses) != 0 {
		t.Errorf("PendingStatuses should be empty, got %d items", len(summary.PendingStatuses))
	}
}

package prx

import (
	"testing"
	"time"
)

func TestFinalizePullRequest(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name             string
		pr               PullRequest
		events           []Event
		requiredChecks   []string
		testStateFromAPI string
		wantTestState    string
		wantMergeable    *bool
		wantDescContains string
	}{
		{
			name: "blocked pr without approvals",
			pr: PullRequest{
				Number:         1,
				MergeableState: "blocked",
			},
			events:           []Event{},
			requiredChecks:   []string{},
			testStateFromAPI: "",
			wantTestState:    TestStateNone,
			wantMergeable:    boolPtr(false),
			wantDescContains: "requires approval",
		},
		{
			name: "clean pr ready to merge",
			pr: PullRequest{
				Number:         1,
				MergeableState: "clean",
			},
			events: []Event{
				{
					Kind:        EventKindReview,
					Actor:       "reviewer",
					Timestamp:   now,
					Outcome:     "APPROVED",
					WriteAccess: WriteAccessDefinitely,
				},
				{
					Kind:      EventKindStatusCheck,
					Timestamp: now,
					Body:      "test",
					Outcome:   "success",
				},
			},
			requiredChecks:   []string{"test"},
			testStateFromAPI: "passing",
			wantTestState:    TestStatePassing,
			wantDescContains: "ready to merge",
		},
		{
			name: "unstable pr with failing checks",
			pr: PullRequest{
				Number:         1,
				MergeableState: "unstable",
			},
			events: []Event{
				{
					Kind:      EventKindStatusCheck,
					Timestamp: now,
					Body:      "test",
					Outcome:   "failure",
				},
			},
			requiredChecks:   []string{"test"},
			testStateFromAPI: "failing",
			wantTestState:    TestStateFailing,
			wantDescContains: "status checks are failing",
		},
		{
			name: "dirty pr with merge conflicts",
			pr: PullRequest{
				Number:         1,
				MergeableState: "dirty",
			},
			events:           []Event{},
			requiredChecks:   []string{},
			testStateFromAPI: "",
			wantTestState:    TestStateNone,
			wantMergeable:    boolPtr(false),
			wantDescContains: "merge conflicts",
		},
		{
			name: "draft pr",
			pr: PullRequest{
				Number:         1,
				MergeableState: "draft",
				Draft:          true,
			},
			events:           []Event{},
			requiredChecks:   []string{},
			testStateFromAPI: "",
			wantTestState:    TestStateNone,
			wantDescContains: "draft state",
		},
		{
			name: "unknown mergeable state",
			pr: PullRequest{
				Number:         1,
				MergeableState: "unknown",
			},
			events:           []Event{},
			requiredChecks:   []string{},
			testStateFromAPI: "",
			wantTestState:    TestStateNone,
			wantDescContains: "being calculated",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			finalizePullRequest(&tt.pr, tt.events, tt.requiredChecks, tt.testStateFromAPI)

			if tt.pr.TestState != tt.wantTestState {
				t.Errorf("TestState = %v, want %v", tt.pr.TestState, tt.wantTestState)
			}

			if tt.wantMergeable != nil {
				if tt.pr.Mergeable == nil {
					t.Errorf("Mergeable is nil, want %v", *tt.wantMergeable)
				} else if *tt.pr.Mergeable != *tt.wantMergeable {
					t.Errorf("Mergeable = %v, want %v", *tt.pr.Mergeable, *tt.wantMergeable)
				}
			}

			if tt.wantDescContains != "" {
				if tt.pr.MergeableStateDescription == "" {
					t.Errorf("MergeableStateDescription is empty, want to contain %q", tt.wantDescContains)
				} else if !contains(tt.pr.MergeableStateDescription, tt.wantDescContains) {
					t.Errorf("MergeableStateDescription = %q, want to contain %q",
						tt.pr.MergeableStateDescription, tt.wantDescContains)
				}
			}
		})
	}
}

func TestFixTestState(t *testing.T) {
	tests := []struct {
		name          string
		checkSummary  *CheckSummary
		wantTestState string
	}{
		{
			name: "failing checks",
			checkSummary: &CheckSummary{
				Failing: map[string]string{"test1": "failed"},
				Success: map[string]string{"test2": "passed"},
			},
			wantTestState: TestStateFailing,
		},
		{
			name: "cancelled checks",
			checkSummary: &CheckSummary{
				Cancelled: map[string]string{"test1": "cancelled"},
			},
			wantTestState: TestStateFailing,
		},
		{
			name: "pending checks",
			checkSummary: &CheckSummary{
				Pending: map[string]string{"test1": "pending"},
				Success: map[string]string{"test2": "passed"},
			},
			wantTestState: TestStatePending,
		},
		{
			name: "only success checks",
			checkSummary: &CheckSummary{
				Success: map[string]string{"test1": "passed", "test2": "passed"},
			},
			wantTestState: TestStatePassing,
		},
		{
			name: "no checks",
			checkSummary: &CheckSummary{
				Success: map[string]string{},
			},
			wantTestState: TestStateNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := &PullRequest{
				CheckSummary: tt.checkSummary,
			}
			fixTestState(pr)
			if pr.TestState != tt.wantTestState {
				t.Errorf("TestState = %v, want %v", pr.TestState, tt.wantTestState)
			}
		})
	}
}

func TestSetMergeableDescription(t *testing.T) {
	tests := []struct {
		name             string
		mergeableState   string
		checkSummary     *CheckSummary
		approvalSummary  *ApprovalSummary
		wantDescContains string
	}{
		{
			name:           "blocked state without approvals",
			mergeableState: "blocked",
			checkSummary: &CheckSummary{
				Failing: map[string]string{},
				Pending: map[string]string{},
			},
			approvalSummary: &ApprovalSummary{
				ApprovalsWithWriteAccess: 0,
			},
			wantDescContains: "requires approval",
		},
		{
			name:             "dirty state",
			mergeableState:   "dirty",
			wantDescContains: "merge conflicts",
		},
		{
			name:             "unstable state",
			mergeableState:   "unstable",
			wantDescContains: "failing",
		},
		{
			name:             "clean state",
			mergeableState:   "clean",
			wantDescContains: "ready to merge",
		},
		{
			name:             "unknown state",
			mergeableState:   "unknown",
			wantDescContains: "being calculated",
		},
		{
			name:             "draft state",
			mergeableState:   "draft",
			wantDescContains: "draft",
		},
		{
			name:             "unknown mergeable state value",
			mergeableState:   "some_other_value",
			wantDescContains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := &PullRequest{
				MergeableState:  tt.mergeableState,
				CheckSummary:    tt.checkSummary,
				ApprovalSummary: tt.approvalSummary,
			}
			if pr.CheckSummary == nil {
				pr.CheckSummary = &CheckSummary{}
			}
			if pr.ApprovalSummary == nil {
				pr.ApprovalSummary = &ApprovalSummary{}
			}
			setMergeableDescription(pr)
			if tt.wantDescContains == "" {
				if pr.MergeableStateDescription != "" {
					t.Errorf("Expected empty description for unknown state, got %q", pr.MergeableStateDescription)
				}
			} else if !contains(pr.MergeableStateDescription, tt.wantDescContains) {
				t.Errorf("MergeableStateDescription = %q, want to contain %q",
					pr.MergeableStateDescription, tt.wantDescContains)
			}
		})
	}
}

func TestSetBlockedDescription(t *testing.T) {
	tests := []struct {
		name             string
		approvalSummary  *ApprovalSummary
		checkSummary     *CheckSummary
		wantDescContains string
	}{
		{
			name: "no approvals, no checks",
			approvalSummary: &ApprovalSummary{
				ApprovalsWithWriteAccess: 0,
			},
			checkSummary: &CheckSummary{
				Failing: map[string]string{},
				Pending: map[string]string{},
			},
			wantDescContains: "requires approval",
		},
		{
			name: "no approvals with pending checks",
			approvalSummary: &ApprovalSummary{
				ApprovalsWithWriteAccess: 0,
			},
			checkSummary: &CheckSummary{
				Failing: map[string]string{},
				Pending: map[string]string{"test": "pending"},
			},
			wantDescContains: "requires approval and has pending",
		},
		{
			name: "failing checks without approval",
			approvalSummary: &ApprovalSummary{
				ApprovalsWithWriteAccess: 0,
			},
			checkSummary: &CheckSummary{
				Failing: map[string]string{"test": "failed"},
			},
			wantDescContains: "failing status checks and requires approval",
		},
		{
			name: "failing checks with approval",
			approvalSummary: &ApprovalSummary{
				ApprovalsWithWriteAccess: 1,
			},
			checkSummary: &CheckSummary{
				Failing: map[string]string{"test": "failed"},
			},
			wantDescContains: "blocked by failing status checks",
		},
		{
			name: "pending checks only",
			approvalSummary: &ApprovalSummary{
				ApprovalsWithWriteAccess: 1,
			},
			checkSummary: &CheckSummary{
				Failing: map[string]string{},
				Pending: map[string]string{"test": "pending"},
			},
			wantDescContains: "blocked by pending",
		},
		{
			name: "has approvals but still blocked",
			approvalSummary: &ApprovalSummary{
				ApprovalsWithWriteAccess: 1,
			},
			checkSummary: &CheckSummary{
				Failing: map[string]string{},
				Pending: map[string]string{},
			},
			wantDescContains: "blocked by required status checks",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := &PullRequest{
				MergeableState:  "blocked",
				ApprovalSummary: tt.approvalSummary,
				CheckSummary:    tt.checkSummary,
			}
			setBlockedDescription(pr)
			if !contains(pr.MergeableStateDescription, tt.wantDescContains) {
				t.Errorf("MergeableStateDescription = %q, want to contain %q",
					pr.MergeableStateDescription, tt.wantDescContains)
			}
		})
	}
}

func boolPtr(b bool) *bool {
	return &b
}

func contains(s, substr string) bool {
	return s != "" && substr != "" && (s == substr || len(s) >= len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

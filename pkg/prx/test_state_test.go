package prx

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

func TestCheckRunsTestStateCalculation(t *testing.T) { //nolint:maintidx // Complex test with many scenarios, acceptable for comprehensive testing
	client := &Client{
		github: &mockGithubClient{},
		logger: slog.Default(),
	}

	tests := []struct {
		name      string
		checkRuns []githubCheckRun
		expected  string
	}{
		{
			name:      "no check runs",
			checkRuns: []githubCheckRun{},
			expected:  TestStateNone,
		},
		{
			name: "all passing",
			checkRuns: []githubCheckRun{
				{
					Name:        "test-1",
					Status:      "completed",
					Conclusion:  "success",
					StartedAt:   time.Now().Add(-5 * time.Minute),
					CompletedAt: time.Now().Add(-3 * time.Minute),
				},
				{
					Name:        "test-2",
					Status:      "completed",
					Conclusion:  "success",
					StartedAt:   time.Now().Add(-4 * time.Minute),
					CompletedAt: time.Now().Add(-2 * time.Minute),
				},
			},
			expected: TestStatePassing,
		},
		{
			name: "one failing overrides passing",
			checkRuns: []githubCheckRun{
				{
					Name:        "test-1",
					Status:      "completed",
					Conclusion:  "success",
					StartedAt:   time.Now().Add(-5 * time.Minute),
					CompletedAt: time.Now().Add(-3 * time.Minute),
				},
				{
					Name:        "test-2",
					Status:      "completed",
					Conclusion:  "failure",
					StartedAt:   time.Now().Add(-4 * time.Minute),
					CompletedAt: time.Now().Add(-2 * time.Minute),
				},
			},
			expected: TestStateFailing,
		},
		{
			name: "running tests override queued",
			checkRuns: []githubCheckRun{
				{
					Name:      "test-1",
					Status:    "queued",
					StartedAt: time.Now().Add(-5 * time.Minute),
				},
				{
					Name:      "test-2",
					Status:    "in_progress",
					StartedAt: time.Now().Add(-3 * time.Minute),
				},
			},
			expected: TestStateRunning,
		},
		{
			name: "queued tests",
			checkRuns: []githubCheckRun{
				{
					Name:      "test-1",
					Status:    "queued",
					StartedAt: time.Now().Add(-2 * time.Minute),
				},
				{
					Name:      "test-2",
					Status:    "queued",
					StartedAt: time.Now().Add(-1 * time.Minute),
				},
			},
			expected: TestStateQueued,
		},
		{
			name: "mixed states - failure has priority",
			checkRuns: []githubCheckRun{
				{
					Name:        "test-1",
					Status:      "completed",
					Conclusion:  "success",
					StartedAt:   time.Now().Add(-10 * time.Minute),
					CompletedAt: time.Now().Add(-8 * time.Minute),
				},
				{
					Name:      "test-2",
					Status:    "in_progress",
					StartedAt: time.Now().Add(-5 * time.Minute),
				},
				{
					Name:        "test-3",
					Status:      "completed",
					Conclusion:  "failure",
					StartedAt:   time.Now().Add(-4 * time.Minute),
					CompletedAt: time.Now().Add(-2 * time.Minute),
				},
			},
			expected: TestStateFailing,
		},
		{
			name: "neutral and cancelled don't affect state",
			checkRuns: []githubCheckRun{
				{
					Name:        "test-1",
					Status:      "completed",
					Conclusion:  "success",
					StartedAt:   time.Now().Add(-5 * time.Minute),
					CompletedAt: time.Now().Add(-3 * time.Minute),
				},
				{
					Name:        "test-2",
					Status:      "completed",
					Conclusion:  "neutral",
					StartedAt:   time.Now().Add(-4 * time.Minute),
					CompletedAt: time.Now().Add(-3 * time.Minute),
				},
				{
					Name:        "test-3",
					Status:      "completed",
					Conclusion:  "cancelled",
					StartedAt:   time.Now().Add(-3 * time.Minute),
					CompletedAt: time.Now().Add(-2 * time.Minute),
				},
			},
			expected: TestStatePassing,
		},
		{
			name: "only neutral/cancelled/skipped results in none",
			checkRuns: []githubCheckRun{
				{
					Name:        "test-1",
					Status:      "completed",
					Conclusion:  "neutral",
					StartedAt:   time.Now().Add(-5 * time.Minute),
					CompletedAt: time.Now().Add(-3 * time.Minute),
				},
				{
					Name:        "test-2",
					Status:      "completed",
					Conclusion:  "skipped",
					StartedAt:   time.Now().Add(-4 * time.Minute),
					CompletedAt: time.Now().Add(-2 * time.Minute),
				},
			},
			expected: TestStateNone,
		},
		{
			name: "action required treated as failure",
			checkRuns: []githubCheckRun{
				{
					Name:        "test-1",
					Status:      "completed",
					Conclusion:  "success",
					StartedAt:   time.Now().Add(-5 * time.Minute),
					CompletedAt: time.Now().Add(-3 * time.Minute),
				},
				{
					Name:        "test-2",
					Status:      "completed",
					Conclusion:  "action_required",
					StartedAt:   time.Now().Add(-4 * time.Minute),
					CompletedAt: time.Now().Add(-2 * time.Minute),
				},
			},
			expected: TestStateFailing,
		},
		{
			name: "timed_out treated as failure",
			checkRuns: []githubCheckRun{
				{
					Name:        "test-1",
					Status:      "completed",
					Conclusion:  "timed_out",
					StartedAt:   time.Now().Add(-10 * time.Minute),
					CompletedAt: time.Now().Add(-5 * time.Minute),
				},
			},
			expected: TestStateFailing,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock PR for the checkRuns method
			pr := &githubPullRequest{
				Head: struct {
					SHA string `json:"sha"`
					Ref string `json:"ref"`
				}{SHA: "abc123", Ref: "feature-branch"},
			}

			// Mock the GitHub API response
			mockClient, ok := client.github.(*mockGithubClient)
			if !ok {
				t.Fatal("failed to cast to mockGithubClient")
			}

			// Convert to pointers as expected by githubCheckRuns
			var checkRunPtrs []*githubCheckRun
			for i := range tt.checkRuns {
				checkRunPtrs = append(checkRunPtrs, &tt.checkRuns[i])
			}

			mockClient.responses = map[string]any{
				"/repos/test/repo/commits/abc123/check-runs?per_page=100": githubCheckRuns{
					CheckRuns: checkRunPtrs,
				},
			}

			// Call checkRuns method
			events, testState, err := client.checkRuns(context.Background(), "test", "repo", pr, []string{})
			if err != nil {
				t.Fatalf("checkRuns() failed: %v", err)
			}

			if testState != tt.expected {
				t.Errorf("checkRuns() testState = %q, want %q", testState, tt.expected)
			}

			// Verify events were created for valid check runs (excludes unknown status)
			expectedEventCount := 0
			for _, cr := range tt.checkRuns {
				if !cr.CompletedAt.IsZero() || cr.Status == "queued" || cr.Status == "in_progress" {
					expectedEventCount++
				}
			}

			if len(events) != expectedEventCount {
				t.Errorf("checkRuns() created %d events, want %d", len(events), expectedEventCount)
			}
		})
	}
}

func TestPullRequestTestStateIntegration(t *testing.T) {
	// Test that PullRequest method correctly captures test state
	client := &Client{
		github: &mockGithubClient{},
		logger: slog.Default(),
	}

	mockClient, ok := client.github.(*mockGithubClient)
	if !ok {
		t.Fatal("failed to cast to mockGithubClient")
	}
	mockClient.responses = map[string]any{
		"/repos/test/repo/pulls/1": githubPullRequest{
			Number: 1,
			Title:  "Test PR",
			State:  "open",
			Head: struct {
				SHA string `json:"sha"`
				Ref string `json:"ref"`
			}{SHA: "abc123", Ref: "feature-branch"},
			User: &githubUser{Login: "testuser"},
		},
		"/repos/test/repo/commits/abc123/check-runs?per_page=100": githubCheckRuns{
			CheckRuns: []*githubCheckRun{
				{
					Name:        "ci-test",
					Status:      "completed",
					Conclusion:  "failure",
					StartedAt:   time.Now().Add(-5 * time.Minute),
					CompletedAt: time.Now().Add(-3 * time.Minute),
				},
			},
		},
		// Mock empty responses for other endpoints
		"/repos/test/repo/pulls/1/commits":       []githubPullRequestCommit{},
		"/repos/test/repo/issues/1/comments":     []githubComment{},
		"/repos/test/repo/pulls/1/reviews":       []githubReview{},
		"/repos/test/repo/pulls/1/comments":      []githubReviewComment{},
		"/repos/test/repo/issues/1/timeline":     []githubTimelineEvent{},
		"/repos/test/repo/commits/abc123/status": map[string]any{"statuses": []any{}},
	}

	result, err := client.PullRequest(context.Background(), "test", "repo", 1)
	if err != nil {
		t.Fatalf("PullRequest() failed: %v", err)
	}

	// Check that test state was correctly set
	if result.PullRequest.TestState != TestStateFailing {
		t.Errorf("PullRequest.TestState = %q, want %q", result.PullRequest.TestState, TestStateFailing)
	}

	// Verify we have a check run event
	var hasCheckRunEvent bool
	for _, event := range result.Events {
		if event.Kind == "check_run" {
			hasCheckRunEvent = true
			break
		}
	}
	if !hasCheckRunEvent {
		t.Error("Expected at least one check_run event")
	}
}

//nolint:errcheck,gocritic // Test handlers don't need to check w.Write errors; if-else chains are fine for URL routing
package prx

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestCheckRunHistory_MultipleCommits tests that we capture check run failures
// from earlier commits even when later commits have successful runs.
// This reproduces the issue from slacker PR #66 where test failures were not visible
// after a successful run on the final commit.
func TestCheckRunHistory_MultipleCommits(t *testing.T) {
	// Simulate multiple commits with check runs that fail initially then succeed
	commit1SHA := "2aec85aceb79be10be46bde3521e0cf9c4b0a0ff"
	commit2SHA := "3f892156c97c97e2029da78a1e70661da4a3231f"
	commit3SHA := "fccd1f2d7ff05afbd8bdb61bec617a509a9757a3"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/graphql" {
			w.WriteHeader(http.StatusOK)
			response := fmt.Sprintf(`{
				"data": {
					"repository": {
						"pullRequest": {
							"number": 66,
							"title": "Fix tests to work with Go stable",
							"body": "Test PR with multiple commits",
							"state": "OPEN",
							"createdAt": "2025-10-29T23:00:00Z",
							"updatedAt": "2025-10-30T01:30:00Z",
							"isDraft": false,
							"additions": 150,
							"deletions": 50,
							"changedFiles": 8,
							"mergeable": "MERGEABLE",
							"mergeStateStatus": "CLEAN",
							"authorAssociation": "OWNER",
							"author": {"login": "developer", "__typename": "User"},
							"assignees": {"nodes": []},
							"labels": {"nodes": []},
							"participants": {"nodes": []},
							"reviewRequests": {"nodes": []},
							"baseRef": {"name": "main", "target": {"oid": "basesha"}},
							"headRef": {"name": "fix-tests", "target": {"oid": "%s"}},
							"commits": {
								"pageInfo": {"hasNextPage": false},
								"nodes": [
									{
										"commit": {
											"oid": "%s",
											"message": "Initial attempt to fix tests",
											"committedDate": "2025-10-29T23:30:00Z",
											"author": {"name": "Developer", "email": "dev@example.com", "user": {"login": "developer"}}
										}
									},
									{
										"commit": {
											"oid": "%s",
											"message": "Second attempt",
											"committedDate": "2025-10-29T23:41:00Z",
											"author": {"name": "Developer", "email": "dev@example.com", "user": {"login": "developer"}}
										}
									},
									{
										"commit": {
											"oid": "%s",
											"message": "Use Go stable",
											"committedDate": "2025-10-30T01:25:00Z",
											"author": {"name": "Developer", "email": "dev@example.com", "user": {"login": "developer"}}
										}
									}
								]
							},
							"reviews": {"pageInfo": {"hasNextPage": false}, "nodes": []},
							"reviewThreads": {"nodes": []},
							"comments": {"pageInfo": {"hasNextPage": false}, "nodes": []},
							"timelineItems": {"pageInfo": {"hasNextPage": false}, "nodes": []}
						}
					}
				}
			}`, commit3SHA, commit1SHA, commit2SHA, commit3SHA)
			_, _ = w.Write([]byte(response))
		} else if strings.Contains(r.URL.Path, "/rulesets") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		} else if strings.Contains(r.URL.Path, fmt.Sprintf("/commits/%s/check-runs", commit1SHA)) {
			// First commit - tests FAILED
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"check_runs": [
					{
						"name": "Kusari Inspector",
						"status": "completed",
						"conclusion": "success",
						"started_at": "2025-10-29T23:30:54Z",
						"completed_at": "2025-10-29T23:31:52Z",
						"output": {"title": "Security Analysis Passed", "summary": "No security issues found"}
					},
					{
						"name": "Unit and Integration Tests",
						"status": "completed",
						"conclusion": "failure",
						"started_at": "2025-10-29T23:30:51Z",
						"completed_at": "2025-10-29T23:33:06Z",
						"output": {"title": "Tests Failed", "summary": "3 tests failed with Go unstable"}
					}
				]
			}`))
		} else if strings.Contains(r.URL.Path, fmt.Sprintf("/commits/%s/check-runs", commit2SHA)) {
			// Second commit - tests still FAILED
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"check_runs": [
					{
						"name": "Kusari Inspector",
						"status": "completed",
						"conclusion": "success",
						"started_at": "2025-10-29T23:41:53Z",
						"completed_at": "2025-10-29T23:42:49Z",
						"output": {"title": "Security Analysis Passed", "summary": "No security issues found"}
					},
					{
						"name": "Unit and Integration Tests",
						"status": "completed",
						"conclusion": "failure",
						"started_at": "2025-10-29T23:41:49Z",
						"completed_at": "2025-10-29T23:44:29Z",
						"output": {"title": "Tests Failed", "summary": "2 tests still failing"}
					}
				]
			}`))
		} else if strings.Contains(r.URL.Path, fmt.Sprintf("/commits/%s/check-runs", commit3SHA)) {
			// Third commit - tests finally PASSED
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"check_runs": [
					{
						"name": "Kusari Inspector",
						"status": "completed",
						"conclusion": "success",
						"started_at": "2025-10-30T01:25:27Z",
						"completed_at": "2025-10-30T01:26:25Z",
						"output": {"title": "Security Analysis Passed", "summary": "No security issues found"}
					},
					{
						"name": "Unit and Integration Tests",
						"status": "completed",
						"conclusion": "success",
						"started_at": "2025-10-30T01:25:24Z",
						"completed_at": "2025-10-30T01:28:21Z",
						"output": {"title": "All Tests Passed", "summary": "Tests now pass with Go stable"}
					}
				]
			}`))
		}
	}))
	defer server.Close()

	httpClient := &http.Client{Transport: http.DefaultTransport}
	client := NewClient("test-token", WithHTTPClient(httpClient))
	client.github = &githubClient{
		client: httpClient,
		token:  "test-token",
		api:    server.URL,
	}

	ctx := context.Background()
	prData, err := client.PullRequest(ctx, "codeGROOVE-dev", "slacker", 66)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if prData == nil {
		t.Fatal("Expected PR data, got nil")
	}

	// Count check_run events
	checkRunCount := 0
	failureCount := 0
	successCount := 0
	var checkRunEvents []Event

	for _, event := range prData.Events {
		if event.Kind == "check_run" {
			checkRunCount++
			checkRunEvents = append(checkRunEvents, event)
			switch event.Outcome {
			case "failure":
				failureCount++
			case "success":
				successCount++
			}
		}
	}

	// We should have 6 total check runs (2 checks × 3 commits)
	if checkRunCount != 6 {
		t.Errorf("Expected 6 check_run events (2 checks × 3 commits), got %d", checkRunCount)
		for i, event := range checkRunEvents {
			t.Logf("  Event %d: %s - %s (outcome: %s, target: %s)", i+1, event.Body, event.Timestamp, event.Outcome, event.Target)
		}
	}

	// We should have 2 failures (Unit and Integration Tests on commits 1 and 2)
	if failureCount != 2 {
		t.Errorf("Expected 2 check_run failure events, got %d", failureCount)
	}

	// We should have 4 successes (Kusari on all 3 commits + Unit tests on commit 3)
	if successCount != 4 {
		t.Errorf("Expected 4 check_run success events, got %d", successCount)
	}

	// Verify that each check run has a commit SHA in the Target field
	for _, event := range checkRunEvents {
		if event.Target == "" {
			t.Errorf("check_run event for %s is missing commit SHA in Target field", event.Body)
		}
	}

	// Verify the CheckSummary shows the LATEST state (both checks passing)
	if prData.PullRequest.CheckSummary == nil {
		t.Fatal("Expected CheckSummary to be set")
	}

	if len(prData.PullRequest.CheckSummary.Success) != 2 {
		t.Errorf("Expected 2 successful checks in summary, got %d", len(prData.PullRequest.CheckSummary.Success))
	}

	if len(prData.PullRequest.CheckSummary.Failing) != 0 {
		t.Errorf("Expected 0 failing checks in summary (latest run passed), got %d", len(prData.PullRequest.CheckSummary.Failing))
	}

	// Verify TestState is passing (since latest runs passed)
	if prData.PullRequest.TestState != "passing" {
		t.Errorf("Expected TestState 'passing', got '%s'", prData.PullRequest.TestState)
	}
}

// TestCheckRunHistory_CommitSHAPreservation tests that commit SHAs are properly
// preserved in both commit events and check_run events.
func TestCheckRunHistory_CommitSHAPreservation(t *testing.T) {
	commitSHA := "abc123def456"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/graphql" {
			w.WriteHeader(http.StatusOK)
			response := fmt.Sprintf(`{
				"data": {
					"repository": {
						"pullRequest": {
							"number": 100,
							"title": "Test PR",
							"body": "Testing commit SHA preservation",
							"state": "OPEN",
							"createdAt": "2025-01-01T00:00:00Z",
							"updatedAt": "2025-01-01T01:00:00Z",
							"isDraft": false,
							"additions": 10,
							"deletions": 5,
							"changedFiles": 2,
							"mergeable": "MERGEABLE",
							"mergeStateStatus": "CLEAN",
							"authorAssociation": "CONTRIBUTOR",
							"author": {"login": "contributor", "__typename": "User"},
							"assignees": {"nodes": []},
							"labels": {"nodes": []},
							"participants": {"nodes": []},
							"reviewRequests": {"nodes": []},
							"baseRef": {"name": "main", "target": {"oid": "basesha"}},
							"headRef": {"name": "feature", "target": {"oid": "%s"}},
							"commits": {
								"pageInfo": {"hasNextPage": false},
								"nodes": [
									{
										"commit": {
											"oid": "%s",
											"message": "Add new feature",
											"committedDate": "2025-01-01T00:30:00Z",
											"author": {"name": "Contributor", "email": "contrib@example.com", "user": {"login": "contributor"}}
										}
									}
								]
							},
							"reviews": {"pageInfo": {"hasNextPage": false}, "nodes": []},
							"reviewThreads": {"nodes": []},
							"comments": {"pageInfo": {"hasNextPage": false}, "nodes": []},
							"timelineItems": {"pageInfo": {"hasNextPage": false}, "nodes": []}
						}
					}
				}
			}`, commitSHA, commitSHA)
			_, _ = w.Write([]byte(response))
		} else if strings.Contains(r.URL.Path, "/rulesets") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		} else if strings.Contains(r.URL.Path, fmt.Sprintf("/commits/%s/check-runs", commitSHA)) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"check_runs": [
					{
						"name": "ci/test",
						"status": "completed",
						"conclusion": "success",
						"started_at": "2025-01-01T00:35:00Z",
						"completed_at": "2025-01-01T00:40:00Z",
						"output": {"title": "Tests passed", "summary": "All good"}
					}
				]
			}`))
		}
	}))
	defer server.Close()

	httpClient := &http.Client{Transport: http.DefaultTransport}
	client := NewClient("test-token", WithHTTPClient(httpClient))
	client.github = &githubClient{
		client: httpClient,
		token:  "test-token",
		api:    server.URL,
	}

	ctx := context.Background()
	prData, err := client.PullRequest(ctx, "testowner", "testrepo", 100)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Find the commit event
	var commitEvent *Event
	for i := range prData.Events {
		if prData.Events[i].Kind == "commit" {
			commitEvent = &prData.Events[i]
			break
		}
	}

	if commitEvent == nil {
		t.Fatal("Expected to find a commit event")
	}

	// Verify commit event has SHA in Body field
	if commitEvent.Body != commitSHA {
		t.Errorf("Expected commit event Body to be '%s', got '%s'", commitSHA, commitEvent.Body)
	}

	// Verify commit event has message in Description field
	if !strings.Contains(commitEvent.Description, "Add new feature") {
		t.Errorf("Expected commit event Description to contain commit message, got '%s'", commitEvent.Description)
	}

	// Find the check_run event
	var checkRunEvent *Event
	for i := range prData.Events {
		if prData.Events[i].Kind == "check_run" {
			checkRunEvent = &prData.Events[i]
			break
		}
	}

	if checkRunEvent == nil {
		t.Fatal("Expected to find a check_run event")
	}

	// Verify check_run event has commit SHA in Target field
	if checkRunEvent.Target != commitSHA {
		t.Errorf("Expected check_run event Target to be '%s', got '%s'", commitSHA, checkRunEvent.Target)
	}

	// Verify check_run event has check name in Body field
	if checkRunEvent.Body != "ci/test" {
		t.Errorf("Expected check_run event Body to be 'ci/test', got '%s'", checkRunEvent.Body)
	}
}

// TestCheckRunHistory_LatestStateCalculation tests that calculateCheckSummary
// correctly identifies the latest state when multiple runs exist for the same check.
func TestCheckRunHistory_LatestStateCalculation(t *testing.T) {
	// Create events with multiple runs of the same check at different times
	events := []Event{
		{
			Kind:      "check_run",
			Timestamp: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
			Body:      "ci/test",
			Outcome:   "failure",
			Target:    "commit1",
		},
		{
			Kind:      "check_run",
			Timestamp: time.Date(2025, 1, 1, 11, 0, 0, 0, time.UTC),
			Body:      "ci/test",
			Outcome:   "failure",
			Target:    "commit2",
		},
		{
			Kind:      "check_run",
			Timestamp: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
			Body:      "ci/test",
			Outcome:   "success",
			Target:    "commit3",
		},
	}

	summary := calculateCheckSummary(events, nil)

	// The latest run (12:00) was successful, so the check should be in Success
	if len(summary.Success) != 1 {
		t.Errorf("Expected 1 successful check, got %d", len(summary.Success))
	}

	if _, ok := summary.Success["ci/test"]; !ok {
		t.Error("Expected ci/test to be in Success category")
	}

	if len(summary.Failing) != 0 {
		t.Errorf("Expected 0 failing checks (latest was success), got %d", len(summary.Failing))
	}
}

// TestCheckRunHistory_OutOfOrderEvents tests that the timestamp-based logic
// correctly handles events that arrive out of chronological order.
func TestCheckRunHistory_OutOfOrderEvents(t *testing.T) {
	// Events intentionally out of order - older success should not override newer failure
	events := []Event{
		{
			Kind:      "check_run",
			Timestamp: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), // Newest (failure)
			Body:      "ci/lint",
			Outcome:   "failure",
			Target:    "commit3",
		},
		{
			Kind:      "check_run",
			Timestamp: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC), // Oldest (success)
			Body:      "ci/lint",
			Outcome:   "success",
			Target:    "commit1",
		},
		{
			Kind:      "check_run",
			Timestamp: time.Date(2025, 1, 1, 11, 0, 0, 0, time.UTC), // Middle (success)
			Body:      "ci/lint",
			Outcome:   "success",
			Target:    "commit2",
		},
	}

	summary := calculateCheckSummary(events, nil)

	// The latest run (12:00) failed, so the check should be in Failing
	if len(summary.Failing) != 1 {
		t.Errorf("Expected 1 failing check, got %d", len(summary.Failing))
	}

	if _, ok := summary.Failing["ci/lint"]; !ok {
		t.Error("Expected ci/lint to be in Failing category (latest run was failure)")
	}

	if len(summary.Success) != 0 {
		t.Errorf("Expected 0 successful checks (latest was failure), got %d", len(summary.Success))
	}
}

// TestCalculateTestStateFromCheckSummary tests the calculateTestStateFromCheckSummary function.
func TestCalculateTestStateFromCheckSummary(t *testing.T) {
	client := &Client{}

	tests := []struct {
		name      string
		summary   *CheckSummary
		wantState string
	}{
		{
			name:      "nil summary returns none",
			summary:   nil,
			wantState: TestStateNone,
		},
		{
			name: "failing checks returns failing",
			summary: &CheckSummary{
				Success: map[string]string{"test1": "passed"},
				Failing: map[string]string{"test2": "failed"},
				Pending: map[string]string{},
			},
			wantState: TestStateFailing,
		},
		{
			name: "only pending checks returns pending",
			summary: &CheckSummary{
				Success: map[string]string{},
				Failing: map[string]string{},
				Pending: map[string]string{"test1": "waiting"},
			},
			wantState: TestStatePending,
		},
		{
			name: "only successful checks returns passing",
			summary: &CheckSummary{
				Success: map[string]string{"test1": "passed", "test2": "passed"},
				Failing: map[string]string{},
				Pending: map[string]string{},
			},
			wantState: TestStatePassing,
		},
		{
			name: "no checks returns none",
			summary: &CheckSummary{
				Success: map[string]string{},
				Failing: map[string]string{},
				Pending: map[string]string{},
			},
			wantState: TestStateNone,
		},
		{
			name: "failing takes precedence over pending",
			summary: &CheckSummary{
				Success: map[string]string{},
				Failing: map[string]string{"test1": "failed"},
				Pending: map[string]string{"test2": "waiting"},
			},
			wantState: TestStateFailing,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := client.calculateTestStateFromCheckSummary(tt.summary)
			if got != tt.wantState {
				t.Errorf("calculateTestStateFromCheckSummary() = %v, want %v", got, tt.wantState)
			}
		})
	}
}

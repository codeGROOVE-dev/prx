//nolint:errcheck,gocritic // Test handlers don't need to check w.Write errors; if-else chains are fine for URL routing
package prx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClient_PullRequestWithCheckRuns(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/graphql" {
			w.WriteHeader(http.StatusOK)
			response := `{
				"data": {
					"repository": {
						"pullRequest": {
							"number": 555,
							"title": "PR with Check Runs",
							"body": "Test PR with CI checks",
							"state": "OPEN",
							"createdAt": "2023-01-01T00:00:00Z",
							"updatedAt": "2023-01-02T00:00:00Z",
							"isDraft": false,
							"additions": 75,
							"deletions": 30,
							"changedFiles": 4,
							"mergeable": "MERGEABLE",
							"mergeStateStatus": "CLEAN",
							"authorAssociation": "OWNER",
							"author": {"login": "developer", "__typename": "User"},
							"assignees": {"nodes": []},
							"labels": {"nodes": []},
							"participants": {"nodes": []},
							"reviewRequests": {"nodes": []},
							"baseRef": {"name": "main"},
							"headRef": {"name": "feature", "target": {"oid": "commitsha123"}},
							"reviews": {"pageInfo": {"hasNextPage": false}, "nodes": []},
							"reviewThreads": {"nodes": []},
							"comments": {"pageInfo": {"hasNextPage": false}, "nodes": []},
							"timelineItems": {"pageInfo": {"hasNextPage": false}, "nodes": []}
						}
					}
				}
			}`
			_, _ = w.Write([]byte(response))
		} else if strings.Contains(r.URL.Path, "/rulesets") {
			w.WriteHeader(http.StatusOK)
			// Return rulesets with required checks
			_, _ = w.Write([]byte(`[
				{
					"id": 123,
					"name": "Branch protection",
					"target": "branch",
					"rules": [
						{
							"type": "required_status_checks",
							"parameters": {
								"required_status_checks": [
									{"context": "ci/test"},
									{"context": "ci/lint"}
								]
							}
						}
					]
				}
			]`))
		} else if strings.Contains(r.URL.Path, "/check-runs") {
			w.WriteHeader(http.StatusOK)
			// Return check runs
			_, _ = w.Write([]byte(`{
				"check_runs": [
					{
						"name": "ci/test",
						"status": "completed",
						"conclusion": "success",
						"started_at": "2023-01-02T08:00:00Z",
						"completed_at": "2023-01-02T08:10:00Z",
						"html_url": "https://github.com/test/repo/runs/1",
						"app": {"owner": {"login": "github-actions[bot]"}},
						"output": {
							"title": "Tests passed",
							"summary": "All tests passed successfully"
						}
					},
					{
						"name": "ci/lint",
						"status": "completed",
						"conclusion": "failure",
						"started_at": "2023-01-02T08:00:00Z",
						"completed_at": "2023-01-02T08:05:00Z",
						"html_url": "https://github.com/test/repo/runs/2",
						"app": {"owner": {"login": "github-actions[bot]"}},
						"output": {
							"title": "Linting failed",
							"summary": "Found style issues"
						}
					},
					{
						"name": "ci/build",
						"status": "in_progress",
						"conclusion": "",
						"started_at": "2023-01-02T08:00:00Z",
						"html_url": "https://github.com/test/repo/runs/3",
						"app": {"owner": {"login": "github-actions[bot]"}},
						"output": {"title": "", "summary": ""}
					}
				]
			}`))
		} else if strings.Contains(r.URL.Path, "/branches/main/protection") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"required_status_checks": {
					"strict": true,
					"contexts": ["ci/test", "ci/lint"],
					"checks": [
						{"context": "ci/test", "app_id": 123},
						{"context": "ci/lint", "app_id": 123}
					]
				}
			}`))
		}
	}))
	defer server.Close()

	httpClient := &http.Client{Transport: http.DefaultTransport}
	client := NewClient("test-token", WithHTTPClient(httpClient))
	client.github = newTestGitHubClient(httpClient, "test-token", server.URL)

	ctx := context.Background()
	prData, err := client.PullRequest(ctx, "testowner", "testrepo", 555)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if prData == nil {
		t.Fatal("Expected PR data, got nil")
	}
	if prData.PullRequest.Number != 555 {
		t.Errorf("Expected PR number 555, got %d", prData.PullRequest.Number)
	}

	// Verify check summaries were calculated
	if prData.PullRequest.TestState == "" {
		t.Error("Expected TestState to be set")
	}
}

func TestClient_PullRequestWithBranchProtection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/graphql" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"data": {
					"repository": {
						"pullRequest": {
							"number": 666,
							"title": "PR with Branch Protection",
							"body": "Test PR",
							"state": "OPEN",
							"createdAt": "2023-01-01T00:00:00Z",
							"updatedAt": "2023-01-02T00:00:00Z",
							"isDraft": false,
							"additions": 25,
							"deletions": 10,
							"changedFiles": 2,
							"mergeable": "MERGEABLE",
							"mergeStateStatus": "BLOCKED",
							"authorAssociation": "CONTRIBUTOR",
							"author": {"login": "contributor", "__typename": "User"},
							"assignees": {"nodes": []},
							"labels": {"nodes": []},
							"participants": {"nodes": []},
							"reviewRequests": {"nodes": []},
							"baseRef": {"name": "main"},
							"headRef": {"name": "fix", "target": {"oid": "sha456def"}},
							"reviews": {"pageInfo": {"hasNextPage": false}, "nodes": []},
							"reviewThreads": {"nodes": []},
							"comments": {"pageInfo": {"hasNextPage": false}, "nodes": []},
							"timelineItems": {"pageInfo": {"hasNextPage": false}, "nodes": []}
						}
					}
				}
			}`))
		} else if strings.Contains(r.URL.Path, "/rulesets") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[
				{
					"id": 456,
					"name": "Require reviews",
					"target": "branch",
					"rules": [
						{
							"type": "pull_request",
							"parameters": {"required_approving_review_count": 2}
						},
						{
							"type": "required_status_checks",
							"parameters": {
								"required_status_checks": [
									{"context": "security/scan"},
									{"context": "quality/coverage"}
								]
							}
						}
					]
				}
			]`))
		} else if strings.Contains(r.URL.Path, "/check-runs") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"check_runs": [
					{
						"name": "security/scan",
						"status": "completed",
						"conclusion": "success",
						"started_at": "2023-01-02T09:00:00Z",
						"completed_at": "2023-01-02T09:15:00Z",
						"html_url": "https://github.com/test/repo/runs/10",
						"app": {"owner": {"login": "security-bot[bot]"}},
						"output": {"title": "No vulnerabilities", "summary": "Scan complete"}
					},
					{
						"name": "quality/coverage",
						"status": "completed",
						"conclusion": "failure",
						"started_at": "2023-01-02T09:00:00Z",
						"completed_at": "2023-01-02T09:10:00Z",
						"html_url": "https://github.com/test/repo/runs/11",
						"app": {"owner": {"login": "coverage-bot[bot]"}},
						"output": {"title": "Coverage too low", "summary": "Need 80%, got 75%"}
					}
				]
			}`))
		}
	}))
	defer server.Close()

	httpClient := &http.Client{Transport: http.DefaultTransport}
	client := NewClient("test-token", WithHTTPClient(httpClient))
	client.github = newTestGitHubClient(httpClient, "test-token", server.URL)

	ctx := context.Background()
	prData, err := client.PullRequest(ctx, "testowner", "testrepo", 666)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if prData == nil {
		t.Fatal("Expected PR data, got nil")
	}

	// Verify blocked state and description
	if prData.PullRequest.MergeableState != "blocked" {
		t.Errorf("Expected blocked state, got %s", prData.PullRequest.MergeableState)
	}

	if prData.PullRequest.MergeableStateDescription == "" {
		t.Error("Expected MergeableStateDescription to be set for blocked PR")
	}
}

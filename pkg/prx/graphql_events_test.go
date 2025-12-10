//nolint:errcheck,gocritic // Test handlers don't need to check w.Write errors; if-else chains are fine for URL routing
package prx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClient_PullRequestWithReviews(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/graphql" {
			w.WriteHeader(http.StatusOK)
			response := `{
				"data": {
					"repository": {
						"pullRequest": {
							"number": 789,
							"title": "PR with Reviews",
							"body": "Test PR with reviews and comments",
							"state": "OPEN",
							"createdAt": "2023-01-01T00:00:00Z",
							"updatedAt": "2023-01-03T00:00:00Z",
							"isDraft": false,
							"additions": 200,
							"deletions": 100,
							"changedFiles": 10,
							"mergeable": "MERGEABLE",
							"mergeStateStatus": "CLEAN",
							"authorAssociation": "MEMBER",
							"author": {"login": "reviewer1", "__typename": "User"},
							"assignees": {"nodes": [
								{"login": "assignee1", "__typename": "User"}
							]},
							"labels": {"nodes": [
								{"name": "bug"},
								{"name": "critical"}
							]},
							"participants": {"nodes": [
								{"login": "participant1", "__typename": "User", "id": "U1"},
								{"login": "participant2", "__typename": "User", "id": "U2"}
							]},
							"reviewRequests": {"nodes": [
								{"requestedReviewer": {"login": "requestedreviewer", "__typename": "User"}}
							]},
							"baseRef": {"name": "main"},
							"headRef": {"name": "feature-branch", "target": {"oid": "def456abc789"}},
							"reviews": {
								"pageInfo": {"hasNextPage": false},
								"nodes": [
									{
										"author": {"login": "reviewer1", "__typename": "User", "id": "R1"},
										"state": "APPROVED",
										"submittedAt": "2023-01-02T10:00:00Z",
										"body": "LGTM!",
										"authorAssociation": "MEMBER"
									},
									{
										"author": {"login": "reviewer2", "__typename": "User", "id": "R2"},
										"state": "CHANGES_REQUESTED",
										"submittedAt": "2023-01-02T11:00:00Z",
										"body": "Please fix the tests",
										"authorAssociation": "CONTRIBUTOR"
									},
									{
										"author": {"login": "reviewer3", "__typename": "User", "id": "R3"},
										"state": "COMMENTED",
										"submittedAt": "2023-01-02T12:00:00Z",
										"body": "Just a comment",
										"authorAssociation": "NONE"
									}
								]
							},
							"reviewThreads": {"nodes": [
								{
									"isResolved": true,
									"comments": {
										"nodes": [
											{
												"author": {"login": "reviewer1", "__typename": "User", "id": "R1"},
												"body": "Please update this",
												"createdAt": "2023-01-02T09:00:00Z",
												"authorAssociation": "MEMBER"
											}
										]
									}
								}
							]},
							"comments": {
								"pageInfo": {"hasNextPage": false},
								"nodes": [
									{
										"author": {"login": "commenter1", "__typename": "User", "id": "C1"},
										"body": "This is a comment",
										"createdAt": "2023-01-02T08:00:00Z",
										"authorAssociation": "COLLABORATOR"
									}
								]
							},
							"timelineItems": {
								"pageInfo": {"hasNextPage": false},
								"nodes": [
									{
										"__typename": "LabeledEvent",
										"createdAt": "2023-01-02T07:00:00Z",
										"actor": {"login": "labeler", "__typename": "User", "id": "L1"},
										"label": {"name": "bug"}
									},
									{
										"__typename": "AssignedEvent",
										"createdAt": "2023-01-02T07:30:00Z",
										"actor": {"login": "assigner", "__typename": "User", "id": "A1"},
										"assignee": {"login": "assignee1", "__typename": "User"}
									},
									{
										"__typename": "ReviewRequestedEvent",
										"createdAt": "2023-01-02T08:00:00Z",
										"actor": {"login": "requester", "__typename": "User", "id": "RQ1"},
										"requestedReviewer": {"login": "requestedreviewer", "__typename": "User"}
									},
									{
										"__typename": "MergedEvent",
										"createdAt": "2023-01-03T00:00:00Z",
										"actor": {"login": "merger", "__typename": "User", "id": "M1"},
										"mergeRefName": "main",
										"commit": {"oid": "mergecommit123"}
									}
								]
							}
						}
					}
				}
			}`
			_, _ = w.Write([]byte(response))
		} else if strings.Contains(r.URL.Path, "/rulesets") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		} else if strings.Contains(r.URL.Path, "/check-runs") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"check_runs": []}`))
		}
	}))
	defer server.Close()

	httpClient := &http.Client{Transport: http.DefaultTransport}
	client := NewClient("test-token", WithHTTPClient(httpClient))
	client.github = newTestGitHubClient(httpClient, "test-token", server.URL)

	ctx := context.Background()
	prData, err := client.PullRequest(ctx, "testowner", "testrepo", 789)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if prData == nil {
		t.Fatal("Expected PR data, got nil")
	}
	if prData.PullRequest.Number != 789 {
		t.Errorf("Expected PR number 789, got %d", prData.PullRequest.Number)
	}

	// Verify events were parsed
	if len(prData.Events) == 0 {
		t.Error("Expected events, got none")
	}

	// Count different event types
	eventTypes := make(map[string]int)
	for i := range prData.Events {
		eventTypes[prData.Events[i].Kind]++
	}

	// Should have reviews
	if eventTypes["review"] < 2 {
		t.Errorf("Expected at least 2 review events, got %d", eventTypes["review"])
	}

	// Should have comments
	if eventTypes["comment"] == 0 {
		t.Error("Expected comment events")
	}
}

func TestClient_PullRequestWithBots(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/graphql" {
			w.WriteHeader(http.StatusOK)
			response := `{
				"data": {
					"repository": {
						"pullRequest": {
							"number": 999,
							"title": "PR with Bot Activity",
							"body": "Test PR with bot comments",
							"state": "OPEN",
							"createdAt": "2023-01-01T00:00:00Z",
							"updatedAt": "2023-01-02T00:00:00Z",
							"isDraft": false,
							"additions": 50,
							"deletions": 25,
							"changedFiles": 3,
							"mergeable": "MERGEABLE",
							"mergeStateStatus": "CLEAN",
							"authorAssociation": "OWNER",
							"author": {"login": "humanuser", "__typename": "User"},
							"assignees": {"nodes": []},
							"labels": {"nodes": []},
							"participants": {"nodes": []},
							"reviewRequests": {"nodes": []},
							"baseRef": {"name": "main"},
							"headRef": {"name": "fix-bug", "target": {"oid": "bottest123"}},
							"reviews": {
								"pageInfo": {"hasNextPage": false},
								"nodes": [
									{
										"author": {"login": "dependabot[bot]", "__typename": "Bot", "id": "BOT_123"},
										"state": "APPROVED",
										"submittedAt": "2023-01-02T10:00:00Z",
										"body": "Approved by bot",
										"authorAssociation": "NONE"
									}
								]
							},
							"reviewThreads": {"nodes": []},
							"comments": {
								"pageInfo": {"hasNextPage": false},
								"nodes": [
									{
										"author": {"login": "renovate-bot", "__typename": "Bot", "id": "BOT_456"},
										"body": "Bot comment",
										"createdAt": "2023-01-02T09:00:00Z",
										"authorAssociation": "NONE"
									}
								]
							},
							"timelineItems": {
								"pageInfo": {"hasNextPage": false},
								"nodes": [
									{
										"__typename": "IssueComment",
										"createdAt": "2023-01-02T08:00:00Z",
										"author": {"login": "github-actions[bot]", "__typename": "Bot", "id": "BOT_789"},
										"body": "CI completed"
									}
								]
							}
						}
					}
				}
			}`
			_, _ = w.Write([]byte(response))
		} else if strings.Contains(r.URL.Path, "/rulesets") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		} else if strings.Contains(r.URL.Path, "/check-runs") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"check_runs": []}`))
		}
	}))
	defer server.Close()

	httpClient := &http.Client{Transport: http.DefaultTransport}
	client := NewClient("test-token", WithHTTPClient(httpClient))
	client.github = newTestGitHubClient(httpClient, "test-token", server.URL)

	ctx := context.Background()
	prData, err := client.PullRequest(ctx, "testowner", "testrepo", 999)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if prData == nil {
		t.Fatal("Expected PR data, got nil")
	}

	// Count bot vs human events
	botEvents := 0
	for i := range prData.Events {
		if prData.Events[i].Bot {
			botEvents++
		}
	}

	if botEvents == 0 {
		t.Error("Expected bot events, got none")
	}
}

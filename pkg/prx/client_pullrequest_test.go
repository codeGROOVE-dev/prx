//nolint:errcheck,gocritic // Test handlers don't need to check w.Write errors; if-else chains are fine for URL routing
package prx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClient_PullRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/graphql" {
			w.WriteHeader(http.StatusOK)
			response := `{
				"data": {
					"repository": {
						"pullRequest": {
							"number": 123,
							"title": "Test PR",
							"body": "Test description",
							"state": "OPEN",
							"createdAt": "2023-01-01T00:00:00Z",
							"updatedAt": "2023-01-02T00:00:00Z",
							"isDraft": false,
							"additions": 100,
							"deletions": 50,
							"changedFiles": 5,
							"mergeable": "MERGEABLE",
							"mergeStateStatus": "CLEAN",
							"authorAssociation": "OWNER",
							"author": {"login": "testauthor", "__typename": "User"},
							"assignees": {"nodes": []},
							"labels": {"nodes": []},
							"participants": {"nodes": []},
							"reviewRequests": {"nodes": []},
							"baseRef": {"name": "main"},
							"headRef": {"name": "feature", "target": {"oid": "abc123def456"}},
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
			_, _ = w.Write([]byte(`[]`))
		} else if strings.Contains(r.URL.Path, "/check-runs") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"check_runs": []}`))
		}
	}))
	defer server.Close()

	httpClient := &http.Client{Transport: http.DefaultTransport}
	client := NewClient("test-token", WithHTTPClient(httpClient))

	// Override the API URL to point to our test server
	client.github = &githubClient{
		client: httpClient,
		token:  "test-token",
		api:    server.URL,
	}

	ctx := context.Background()
	prData, err := client.PullRequest(ctx, "testowner", "testrepo", 123)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if prData == nil {
		t.Fatal("Expected PR data, got nil")
	}
	if prData.PullRequest.Number != 123 {
		t.Errorf("Expected PR number 123, got %d", prData.PullRequest.Number)
	}
	if prData.PullRequest.Title != "Test PR" {
		t.Errorf("Expected title 'Test PR', got '%s'", prData.PullRequest.Title)
	}
	if prData.PullRequest.Author != "testauthor" {
		t.Errorf("Expected author 'testauthor', got '%s'", prData.PullRequest.Author)
	}
}

func TestClient_PullRequestWithCache(t *testing.T) {
	tmpDir := t.TempDir()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/graphql" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"data": {
					"repository": {
						"pullRequest": {
							"number": 456,
							"title": "Cached PR",
							"body": "Test cached PR",
							"state": "OPEN",
							"createdAt": "2023-01-01T00:00:00Z",
							"updatedAt": "2023-01-02T00:00:00Z",
							"isDraft": false,
							"additions": 10,
							"deletions": 5,
							"changedFiles": 2,
							"mergeable": "MERGEABLE",
							"mergeStateStatus": "CLEAN",
							"authorAssociation": "OWNER",
							"author": {"login": "cachetest", "__typename": "User"},
							"assignees": {"nodes": []},
							"labels": {"nodes": []},
							"participants": {"nodes": []},
							"reviewRequests": {"nodes": []},
							"baseRef": {"name": "main"},
							"headRef": {"name": "feature", "target": {"oid": "cachehash"}},
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
			_, _ = w.Write([]byte(`[]`))
		} else if strings.Contains(r.URL.Path, "/check-runs") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"check_runs": []}`))
		}
	}))
	defer server.Close()

	httpClient := &http.Client{Transport: http.DefaultTransport}
	client, err := NewCacheClient("test-token", tmpDir, WithHTTPClient(httpClient))
	if err != nil {
		t.Fatalf("Failed to create cache client: %v", err)
	}

	// Override the API URL
	client.github = &githubClient{
		client: httpClient,
		token:  "test-token",
		api:    server.URL,
	}

	ctx := context.Background()
	refTime := time.Now()
	prData, err := client.PullRequestWithReferenceTime(ctx, "testowner", "testrepo", 456, refTime)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if prData == nil {
		t.Fatal("Expected PR data, got nil")
	}
	if prData.PullRequest.Number != 456 {
		t.Errorf("Expected PR number 456, got %d", prData.PullRequest.Number)
	}

	// Call again with same reference time to test cache hit path
	prData2, err := client.PullRequestWithReferenceTime(ctx, "testowner", "testrepo", 456, refTime)
	if err != nil {
		t.Fatalf("Expected no error on cached request, got: %v", err)
	}
	if prData2 == nil {
		t.Fatal("Expected PR data from cache, got nil")
	}
}

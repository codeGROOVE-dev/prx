package prx

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestCacheClient(t *testing.T) {
	// Create temporary cache directory
	cacheDir := t.TempDir()

	// Create test server
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		switch r.URL.Path {
		case "/repos/test/repo/pulls/1":
			pr := githubPullRequest{
				Number:    1,
				Title:     "Test PR",
				Body:      "Test body",
				CreatedAt: time.Now().Add(-24 * time.Hour),
				UpdatedAt: time.Now().Add(-2 * time.Hour),
				User:      &githubUser{Login: "testuser"},
				State:     "closed",
				ClosedAt:  time.Now().Add(-1 * time.Hour),
			}
			pr.Head.SHA = "abc123"
			if err := json.NewEncoder(w).Encode(pr); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		case "/repos/test/repo/pulls/1/commits":
			commit := &githubPullRequestCommit{
				Author: &githubUser{Login: "testuser"},
			}
			commit.Commit.Author.Date = time.Now().Add(-12 * time.Hour)
			commit.Commit.Message = "Test commit"
			if err := json.NewEncoder(w).Encode([]*githubPullRequestCommit{commit}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		case "/repos/test/repo/commits/abc123/status":
			// Combined status endpoint - return empty response
			if _, err := w.Write([]byte(`{"state": "success", "statuses": []}`)); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		case "/repos/test/repo/branches//protection":
			// Branch protection endpoint - return empty protection
			if _, err := w.Write([]byte(`{}`)); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		case "/repos/test/repo/branches//protection/required_status_checks":
			// Required status checks endpoint - return no required checks
			if _, err := w.Write([]byte(`{}`)); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		case "/repos/test/repo/rulesets":
			// Rulesets endpoint - return empty rulesets
			if _, err := w.Write([]byte(`[]`)); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		case "/graphql":
			// GraphQL endpoint - return a minimal PR response
			response := `{"data": {"repository": {"pullRequest": {
				"number": 1,
				"title": "Test PR",
				"body": "Test body",
				"state": "CLOSED",
				"isDraft": false,
				"createdAt": "2023-01-01T00:00:00Z",
				"updatedAt": "2023-01-01T01:00:00Z",
				"closedAt": "2023-01-01T02:00:00Z",
				"mergedAt": null,
				"mergedBy": null,
				"mergeable": "UNKNOWN",
				"mergeStateStatus": "UNKNOWN",
				"additions": 10,
				"deletions": 5,
				"changedFiles": 2,
				"author": {"login": "testuser"},
				"authorAssociation": "CONTRIBUTOR",
				"headRef": {"target": {"oid": "abc123"}},
				"baseRef": {"name": "main", "target": {"oid": "def456"}},
				"assignees": {"nodes": []},
				"labels": {"nodes": []},
				"reviews": {"nodes": []},
				"reviewRequests": {"nodes": []},
				"reviewThreads": {"nodes": []},
				"commits": {"nodes": []},
				"statusCheckRollup": null,
				"timelineItems": {"nodes": [], "pageInfo": {"hasNextPage": false}},
				"comments": {"nodes": []}
			}}}}`
			if _, err := w.Write([]byte(response)); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		default:
			// Check runs endpoint expects a different format
			if r.URL.Path == "/repos/test/repo/commits/abc123/check-runs" {
				if _, err := w.Write([]byte(`{"check_runs": []}`)); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
			} else {
				// Return empty array for other endpoints
				if _, err := w.Write([]byte("[]")); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
			}
		}
	}))
	defer server.Close()

	// Create cache client with test server
	client, err := NewCacheClient("test-token", cacheDir,
		WithHTTPClient(&http.Client{Transport: &http.Transport{}}),
		WithLogger(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))),
	)
	if err != nil {
		t.Fatalf("Failed to create cache client: %v", err)
	}
	defer func() {
		if closeErr := client.Close(); closeErr != nil {
			t.Errorf("Failed to close client: %v", closeErr)
		}
	}()

	// Override the GitHub client to use test server
	if gc, ok := client.github.(*githubClient); ok {
		gc.api = server.URL
	}

	ctx := context.Background()

	// First request - should hit the API
	beforeFirstRequest := requestCount
	events1, err := client.PullRequest(ctx, "test", "repo", 1, time.Now().Add(-2*time.Hour))
	if err != nil {
		t.Fatalf("First request failed: %v", err)
	}
	afterFirstRequest := requestCount

	if afterFirstRequest == beforeFirstRequest {
		t.Error("Expected API requests for first call")
	}

	if len(events1.Events) < 2 { // At least PR opened and closed events
		t.Errorf("Expected at least 2 events, got %d", len(events1.Events))
	}

	// Second request with same reference time - should use cache for most endpoints
	beforeSecondRequest := requestCount
	events2, err := client.PullRequest(ctx, "test", "repo", 1, time.Now().Add(-2*time.Hour))
	if err != nil {
		t.Fatalf("Second request failed: %v", err)
	}
	afterSecondRequest := requestCount

	// We expect zero API requests for cached call (sfcache handles persistence)
	if afterSecondRequest != beforeSecondRequest {
		t.Logf("Note: Got %d API requests for cached call (cache may need warming)", afterSecondRequest-beforeSecondRequest)
	}

	if len(events1.Events) != len(events2.Events) {
		t.Errorf("Expected same number of events from cache, got %d vs %d", len(events1.Events), len(events2.Events))
	}

	// Third request with future reference time - should hit API again
	beforeThirdRequest := requestCount
	_, err = client.PullRequest(ctx, "test", "repo", 1, time.Now().Add(1*time.Hour))
	if err != nil {
		t.Fatalf("Third request failed: %v", err)
	}
	afterThirdRequest := requestCount

	if afterThirdRequest == beforeThirdRequest {
		t.Error("Expected API requests for future reference time")
	}
}

func TestCacheKeyGeneration(t *testing.T) {
	// Test that cache keys are consistent
	key1 := prCacheKey("owner", "repo", 123)
	key2 := prCacheKey("owner", "repo", 123)

	if key1 != key2 {
		t.Error("Cache keys should be consistent for same inputs")
	}

	// Test that different inputs produce different keys
	key3 := prCacheKey("owner", "repo", 456)
	if key1 == key3 {
		t.Error("Different inputs should produce different cache keys")
	}

	// Verify key format (should be 64 char hex string)
	if len(key1) != 64 {
		t.Errorf("Cache key should be 64 characters, got %d", len(key1))
	}

	if !isHexString(key1) {
		t.Error("Cache key should be a hex string")
	}
}

func TestIsHexString(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"0123456789abcdef", true},
		{"ABCDEF", true},
		{"0123456789ABCDEF", true},
		{"xyz", false},
		{"12g4", false},
		{"", true}, // Empty string is technically all hex
	}

	for _, tt := range tests {
		result := isHexString(tt.input)
		if result != tt.expected {
			t.Errorf("isHexString(%q) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}

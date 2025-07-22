package prevents

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestClient_FetchPullRequestEvents(t *testing.T) {
	// Create a test server
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	// Mock PR endpoint
	mux.HandleFunc("/repos/owner/repo/pulls/1", func(w http.ResponseWriter, r *http.Request) {
		pr := map[string]interface{}{
			"number": 1,
			"state":  "open",
			"created_at": "2024-01-01T12:00:00Z",
			"user": map[string]interface{}{
				"login":    "testuser",
				"html_url": "https://github.com/testuser",
			},
			"head": map[string]interface{}{
				"sha": "abc123",
			},
		}
		json.NewEncoder(w).Encode(pr)
	})

	// Mock commits endpoint
	mux.HandleFunc("/repos/owner/repo/pulls/1/commits", func(w http.ResponseWriter, r *http.Request) {
		commits := []map[string]interface{}{
			{
				"sha":      "abc123",
				"html_url": "https://github.com/owner/repo/commit/abc123",
				"commit": map[string]interface{}{
					"message": "Initial commit",
					"author": map[string]interface{}{
						"name": "Test Author",
						"date": "2024-01-01T12:30:00Z",
					},
				},
				"author": map[string]interface{}{
					"login":    "testuser",
					"html_url": "https://github.com/testuser",
				},
			},
		}
		json.NewEncoder(w).Encode(commits)
	})

	// Mock other endpoints with empty responses
	mux.HandleFunc("/repos/owner/repo/issues/1/comments", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]interface{}{})
	})
	mux.HandleFunc("/repos/owner/repo/pulls/1/reviews", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]interface{}{})
	})
	mux.HandleFunc("/repos/owner/repo/pulls/1/comments", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]interface{}{})
	})
	mux.HandleFunc("/repos/owner/repo/issues/1/timeline", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]interface{}{})
	})
	mux.HandleFunc("/repos/owner/repo/statuses/abc123", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]interface{}{})
	})
	mux.HandleFunc("/repos/owner/repo/commits/abc123/check-runs", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"total_count": 0,
			"check_runs":  []interface{}{},
		})
	})

	// Create client with test server
	httpClient := &http.Client{
		Transport: &http.Transport{
			Proxy: func(req *http.Request) (*url.URL, error) {
				return url.Parse(server.URL)
			},
		},
	}
	
	// Override the base URL in the client
	client := NewClientWithHTTP(httpClient)
	client.github.BaseURL, _ = url.Parse(server.URL + "/")

	// Test fetching events
	ctx := context.Background()
	events, err := client.FetchPullRequestEvents(ctx, "owner", "repo", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify we got at least the PR opened event and commit
	if len(events) < 2 {
		t.Errorf("expected at least 2 events, got %d", len(events))
	}

	// Verify first event is PR opened
	if events[0].Type != EventTypePROpened {
		t.Errorf("expected first event to be PR opened, got %s", events[0].Type)
	}

	// Verify events are sorted by timestamp
	for i := 1; i < len(events); i++ {
		if events[i].Timestamp.Before(events[i-1].Timestamp) {
			t.Error("events not sorted by timestamp")
		}
	}
}


func TestEventTypes(t *testing.T) {
	// Verify event type constants are properly defined
	eventTypes := []EventType{
		EventTypeCommit,
		EventTypeComment,
		EventTypeReview,
		EventTypeReviewComment,
		EventTypeStatusCheck,
		EventTypeCheckRun,
		EventTypeCheckSuite,
		EventTypePROpened,
		EventTypePRClosed,
		EventTypePRMerged,
		EventTypePRReopened,
		EventTypeAssigned,
		EventTypeUnassigned,
		EventTypeLabeled,
		EventTypeUnlabeled,
		EventTypeMilestoned,
		EventTypeDemilestoned,
		EventTypeReviewRequested,
		EventTypeReviewRequestRemoved,
	}

	// Ensure all event types are unique
	seen := make(map[EventType]bool)
	for _, et := range eventTypes {
		if seen[et] {
			t.Errorf("duplicate event type: %s", et)
		}
		seen[et] = true
	}
}
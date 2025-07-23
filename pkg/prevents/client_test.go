package prevents

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"
)

// mockGithubClient implements githubAPIClient for testing
type mockGithubClient struct {
	responses map[string]any
	calls     []string
}

func (m *mockGithubClient) get(ctx context.Context, path string, v any) (*githubResponse, error) {
	m.calls = append(m.calls, path)

	if response, ok := m.responses[path]; ok {
		data, _ := json.Marshal(response)
		return &githubResponse{NextPage: 0}, json.Unmarshal(data, v)
	}

	// Return empty response
	return &githubResponse{NextPage: 0}, nil
}

func TestClientWithMock(t *testing.T) {
	mock := &mockGithubClient{
		responses: map[string]any{
			"/repos/owner/repo/pulls/1": githubPullRequest{
				CreatedAt: time.Now().Add(-24 * time.Hour),
				UpdatedAt: time.Now().Add(-1 * time.Hour),
				User:      &githubUser{Login: "testuser"},
				State:     "open",
				Head: struct {
					SHA string `json:"sha"`
				}{SHA: "abc123"},
			},
			"/repos/owner/repo/pulls/1/commits":           []*githubPullRequestCommit{},
			"/repos/owner/repo/issues/1/comments":         []*githubComment{},
			"/repos/owner/repo/pulls/1/reviews":           []*githubReview{},
			"/repos/owner/repo/pulls/1/comments":          []*githubReviewComment{},
			"/repos/owner/repo/issues/1/timeline":         []*githubTimelineEvent{},
			"/repos/owner/repo/statuses/abc123":           []*githubStatus{},
			"/repos/owner/repo/commits/abc123/check-runs": githubCheckRuns{CheckRuns: []*githubCheckRun{}},
		},
	}

	client := &Client{
		github: mock,
		logger: slog.Default(),
	}

	ctx := context.Background()
	events, err := client.PullRequestEvents(ctx, "owner", "repo", 1)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Should have at least the PR opened event
	if len(events) < 1 {
		t.Errorf("Expected at least 1 event, got %d", len(events))
	}

	// Verify API calls were made
	expectedCalls := []string{
		"/repos/owner/repo/pulls/1",
		"/repos/owner/repo/pulls/1/commits",
		"/repos/owner/repo/issues/1/comments",
		"/repos/owner/repo/pulls/1/reviews",
		"/repos/owner/repo/pulls/1/comments",
		"/repos/owner/repo/issues/1/timeline",
		"/repos/owner/repo/statuses/abc123",
		"/repos/owner/repo/commits/abc123/check-runs",
	}

	if len(mock.calls) != len(expectedCalls) {
		t.Errorf("Expected %d API calls, got %d", len(expectedCalls), len(mock.calls))
		t.Logf("Actual calls: %v", mock.calls)
	}
}

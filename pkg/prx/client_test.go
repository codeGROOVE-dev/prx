package prx

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"testing"
	"time"
)

// mockGithubClient is a mock implementation for testing
type mockGithubClient struct {
	mu        sync.Mutex
	responses map[string]any
	calls     []string
}

func (m *mockGithubClient) get(ctx context.Context, path string, v any) (*githubResponse, error) {
	m.mu.Lock()
	m.calls = append(m.calls, path)
	m.mu.Unlock()

	if response, ok := m.responses[path]; ok {
		data, _ := json.Marshal(response)
		return &githubResponse{NextPage: 0}, json.Unmarshal(data, v)
	}

	// Return empty response
	return &githubResponse{NextPage: 0}, nil
}

func (m *mockGithubClient) raw(ctx context.Context, path string) (json.RawMessage, *githubResponse, error) {
	m.mu.Lock()
	m.calls = append(m.calls, path)
	m.mu.Unlock()

	if response, ok := m.responses[path]; ok {
		data, _ := json.Marshal(response)
		return json.RawMessage(data), &githubResponse{NextPage: 0}, nil
	}

	// Return empty array for paginated endpoints
	return json.RawMessage("[]"), &githubResponse{NextPage: 0}, nil
}

func (m *mockGithubClient) userPermission(ctx context.Context, owner, repo, username string) (string, error) {
	path := "/repos/" + owner + "/" + repo + "/collaborators/" + username + "/permission"
	m.calls = append(m.calls, path)

	if response, ok := m.responses[path]; ok {
		if perm, ok := response.(string); ok {
			return perm, nil
		}
	}

	// Default to read access
	return "read", nil
}

func TestClientWithMock(t *testing.T) {
	mock := &mockGithubClient{
		responses: map[string]any{
			"/repos/owner/repo/pulls/1": githubPullRequest{
				Number:            1,
				Title:             "Test PR",
				Body:              "Test description",
				CreatedAt:         time.Now().Add(-24 * time.Hour),
				UpdatedAt:         time.Now().Add(-1 * time.Hour),
				User:              &githubUser{Login: "testuser"},
				AuthorAssociation: "CONTRIBUTOR",
				State:             "open",
				Head: struct {
					SHA string `json:"sha"`
					Ref string `json:"ref"`
				}{SHA: "abc123", Ref: "feature-branch"},
				Base: struct {
					Ref string `json:"ref"`
				}{Ref: "main"},
			},
			"/repos/owner/repo/pulls/1/commits?page=1&per_page=100":    []*githubPullRequestCommit{},
			"/repos/owner/repo/issues/1/comments?page=1&per_page=100":  []*githubComment{},
			"/repos/owner/repo/pulls/1/reviews?page=1&per_page=100":    []*githubReview{},
			"/repos/owner/repo/pulls/1/comments?page=1&per_page=100":   []*githubReviewComment{},
			"/repos/owner/repo/issues/1/timeline?page=1&per_page=100":  []*githubTimelineEvent{},
			"/repos/owner/repo/statuses/abc123?per_page=100":           []*githubStatus{},
			"/repos/owner/repo/commits/abc123/check-runs?per_page=100": githubCheckRuns{CheckRuns: []*githubCheckRun{}},
		},
	}

	client := &Client{
		github: mock,
		logger: slog.Default(),
		permissionCache: &permissionCache{
			memory: make(map[string]permissionEntry),
		},
	}

	ctx := context.Background()
	data, err := client.PullRequest(ctx, "owner", "repo", 1)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Should have PR metadata
	if data.PullRequest.Number != 1 {
		t.Errorf("Expected PR number 1, got %d", data.PullRequest.Number)
	}

	// Should have at least the PR opened event
	if len(data.Events) < 1 {
		t.Errorf("Expected at least 1 event, got %d", len(data.Events))
	}

	// Verify API calls were made
	expectedCalls := []string{
		"/repos/owner/repo/pulls/1",
		"/repos/owner/repo/pulls/1/commits?page=1&per_page=100",
		"/repos/owner/repo/issues/1/comments?page=1&per_page=100",
		"/repos/owner/repo/pulls/1/reviews?page=1&per_page=100",
		"/repos/owner/repo/pulls/1/comments?page=1&per_page=100",
		"/repos/owner/repo/issues/1/timeline?page=1&per_page=100",
		"/repos/owner/repo/statuses/abc123?per_page=100",
		"/repos/owner/repo/commits/abc123/check-runs?per_page=100",
	}

	mock.mu.Lock()
	actualCalls := len(mock.calls)
	callsCopy := make([]string, len(mock.calls))
	copy(callsCopy, mock.calls)
	mock.mu.Unlock()

	if actualCalls != len(expectedCalls) {
		t.Errorf("Expected %d API calls, got %d", len(expectedCalls), actualCalls)
		t.Logf("Actual calls: %v", callsCopy)
	}
}

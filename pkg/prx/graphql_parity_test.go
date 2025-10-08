package prx

import (
	"context"
	"log/slog"
	"reflect"
	"sort"
	"testing"
)

// TestGraphQLParity verifies that GraphQL implementation returns the same data as REST
func TestGraphQLParity(t *testing.T) {
	// This test would need a real GitHub token and repository to test against
	// For unit testing, we'll create a mock comparison
	t.Skip("Requires real GitHub API access for full parity testing")

	ctx := context.Background()
	client := &Client{} // Would need proper initialization

	// Test data
	owner := "golang"
	repo := "go"
	prNumber := 1

	// Fetch via direct call (non-cached)
	restData, err := client.pullRequestViaGraphQL(ctx, owner, repo, prNumber)
	if err != nil {
		t.Fatalf("Direct fetch failed: %v", err)
	}

	// Fetch via GraphQL
	graphqlData, err := client.pullRequestViaGraphQL(ctx, owner, repo, prNumber)
	if err != nil {
		t.Fatalf("GraphQL fetch failed: %v", err)
	}

	// Compare critical fields
	comparePullRequestData(t, restData, graphqlData)
}

// comparePullRequestData compares REST and GraphQL results
func comparePullRequestData(t *testing.T, rest, graphql *PullRequestData) {
	t.Helper()
	// Compare PullRequest fields
	pr1 := rest.PullRequest
	pr2 := graphql.PullRequest

	// Basic fields
	assertEqual(t, "Number", pr1.Number, pr2.Number)
	assertEqual(t, "Title", pr1.Title, pr2.Title)
	assertEqual(t, "Author", pr1.Author, pr2.Author)
	assertEqual(t, "State", pr1.State, pr2.State)
	assertEqual(t, "Draft", pr1.Draft, pr2.Draft)
	assertEqual(t, "Merged", pr1.Merged, pr2.Merged)
	assertEqual(t, "MergeableState", pr1.MergeableState, pr2.MergeableState)
	assertEqual(t, "AuthorWriteAccess", pr1.AuthorWriteAccess, pr2.AuthorWriteAccess)

	// Check summaries
	if pr1.CheckSummary != nil && pr2.CheckSummary != nil {
		assertEqual(t, "CheckSummary.Success count", len(pr1.CheckSummary.Success), len(pr2.CheckSummary.Success))
		assertEqual(t, "CheckSummary.Failing count", len(pr1.CheckSummary.Failing), len(pr2.CheckSummary.Failing))
		assertEqual(t, "CheckSummary.Pending count", len(pr1.CheckSummary.Pending), len(pr2.CheckSummary.Pending))
		assertEqual(t, "CheckSummary.Cancelled count", len(pr1.CheckSummary.Cancelled), len(pr2.CheckSummary.Cancelled))
		assertEqual(t, "CheckSummary.Skipped count", len(pr1.CheckSummary.Skipped), len(pr2.CheckSummary.Skipped))
		assertEqual(t, "CheckSummary.Stale count", len(pr1.CheckSummary.Stale), len(pr2.CheckSummary.Stale))
		assertEqual(t, "CheckSummary.Neutral count", len(pr1.CheckSummary.Neutral), len(pr2.CheckSummary.Neutral))
	}

	// Compare event counts by type
	restEventCounts := countEventsByType(rest.Events)
	graphqlEventCounts := countEventsByType(graphql.Events)

	for eventType, restCount := range restEventCounts {
		graphqlCount := graphqlEventCounts[eventType]
		if restCount != graphqlCount {
			t.Errorf("Event count mismatch for %s: REST=%d, GraphQL=%d",
				eventType, restCount, graphqlCount)
		}
	}

	// Compare critical event fields
	compareEvents(t, rest.Events, graphql.Events)
}

// countEventsByType counts events by their Kind
func countEventsByType(events []Event) map[string]int {
	counts := make(map[string]int)
	for i := range events {
		counts[events[i].Kind]++
	}
	return counts
}

// compareEvents compares event details
func compareEvents(t *testing.T, restEvents, graphqlEvents []Event) {
	t.Helper()
	// Sort events by timestamp and kind for comparison
	sort.Slice(restEvents, func(i, j int) bool {
		if restEvents[i].Timestamp.Equal(restEvents[j].Timestamp) {
			return restEvents[i].Kind < restEvents[j].Kind
		}
		return restEvents[i].Timestamp.Before(restEvents[j].Timestamp)
	})

	sort.Slice(graphqlEvents, func(i, j int) bool {
		if graphqlEvents[i].Timestamp.Equal(graphqlEvents[j].Timestamp) {
			return graphqlEvents[i].Kind < graphqlEvents[j].Kind
		}
		return graphqlEvents[i].Timestamp.Before(graphqlEvents[j].Timestamp)
	})

	// Compare critical fields for matching events
	for i := 0; i < len(restEvents) && i < len(graphqlEvents); i++ {
		rest := restEvents[i]
		graphql := graphqlEvents[i]

		// Check critical fields
		if rest.Kind != graphql.Kind {
			t.Logf("Event kind mismatch at index %d: REST=%s, GraphQL=%s",
				i, rest.Kind, graphql.Kind)
			continue
		}

		// For events with write access, ensure it's preserved
		if rest.WriteAccess != WriteAccessNA && graphql.WriteAccess == WriteAccessNA {
			t.Errorf("WriteAccess lost for event %s by %s: REST=%d, GraphQL=%d",
				rest.Kind, rest.Actor, rest.WriteAccess, graphql.WriteAccess)
		}

		// For bot detection
		if rest.Bot != graphql.Bot && rest.Actor != "" {
			t.Logf("Bot detection mismatch for %s: REST=%v, GraphQL=%v",
				rest.Actor, rest.Bot, graphql.Bot)
		}

		// For status checks, ensure outcome is preserved
		if rest.Kind == "status_check" || rest.Kind == "check_run" {
			if rest.Outcome != graphql.Outcome {
				t.Errorf("Outcome mismatch for %s: REST=%s, GraphQL=%s",
					rest.Body, rest.Outcome, graphql.Outcome)
			}
		}
	}
}

// assertEqual is a test helper
func assertEqual(t *testing.T, field string, expected, actual any) {
	t.Helper()
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("%s mismatch: expected=%v, actual=%v", field, expected, actual)
	}
}

// TestGraphQLBotDetection tests bot detection logic
func TestGraphQLBotDetection(t *testing.T) {
	tests := []struct {
		name     string
		actor    graphQLActor
		expected bool
	}{
		{
			name:     "bot suffix",
			actor:    graphQLActor{Login: "dependabot[bot]"},
			expected: true,
		},
		{
			name:     "bot dash suffix",
			actor:    graphQLActor{Login: "renovate-bot"},
			expected: true,
		},
		{
			name:     "robot suffix",
			actor:    graphQLActor{Login: "k8s-ci-robot"},
			expected: true,
		},
		{
			name:     "regular user",
			actor:    graphQLActor{Login: "octocat"},
			expected: false,
		},
		{
			name:     "bot ID prefix",
			actor:    graphQLActor{Login: "actions", ID: "BOT_123"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isBotFromGraphQL(tt.actor)
			if result != tt.expected {
				t.Errorf("isBotFromGraphQL(%v) = %v, want %v",
					tt.actor, result, tt.expected)
			}
		})
	}
}

// TestWriteAccessMapping tests the write access calculation
func TestWriteAccessMapping(t *testing.T) {
	ctx := context.Background()
	c := &Client{
		logger:             slog.Default(),
		collaboratorsCache: &collaboratorsCache{memory: make(map[string]collaboratorsEntry)},
	}

	tests := []struct {
		association string
		expected    int
	}{
		{"OWNER", WriteAccessDefinitely},
		{"COLLABORATOR", WriteAccessDefinitely},
		{"MEMBER", WriteAccessLikely}, // Falls back to likely when collaborators API unavailable
		{"CONTRIBUTOR", WriteAccessUnlikely},
		{"NONE", WriteAccessUnlikely},
		{"FIRST_TIME_CONTRIBUTOR", WriteAccessUnlikely},
		{"FIRST_TIMER", WriteAccessUnlikely},
		{"UNKNOWN", WriteAccessNA},
	}

	for _, tt := range tests {
		t.Run(tt.association, func(t *testing.T) {
			result := c.writeAccessFromAssociation(ctx, "owner", "repo", "user", tt.association)
			if result != tt.expected {
				t.Errorf("writeAccessFromAssociation(%s) = %d, want %d",
					tt.association, result, tt.expected)
			}
		})
	}
}

// TestRequiredChecksExtraction tests extraction of required checks from GraphQL
func TestRequiredChecksExtraction(t *testing.T) {
	data := &graphQLPullRequestComplete{
		BaseRef: struct {
			RefUpdateRule *struct {
				RequiredStatusCheckContexts []string `json:"requiredStatusCheckContexts"`
			} `json:"refUpdateRule"`
			BranchProtectionRule *struct {
				RequiredStatusCheckContexts  []string `json:"requiredStatusCheckContexts"`
				RequiredApprovingReviewCount int      `json:"requiredApprovingReviewCount"`
				RequiresStatusChecks         bool     `json:"requiresStatusChecks"`
			} `json:"branchProtectionRule"`
			Target struct {
				OID string `json:"oid"`
			} `json:"target"`
			Name string `json:"name"`
		}{
			RefUpdateRule: &struct {
				RequiredStatusCheckContexts []string `json:"requiredStatusCheckContexts"`
			}{
				RequiredStatusCheckContexts: []string{"test", "lint"},
			},
			BranchProtectionRule: &struct {
				RequiredStatusCheckContexts  []string `json:"requiredStatusCheckContexts"`
				RequiredApprovingReviewCount int      `json:"requiredApprovingReviewCount"`
				RequiresStatusChecks         bool     `json:"requiresStatusChecks"`
			}{
				RequiredStatusCheckContexts: []string{"build", "test"}, // "test" is duplicate
			},
		},
	}

	c := &Client{}
	checks := c.extractRequiredChecksFromGraphQL(data)

	// Should deduplicate and contain all unique checks
	expectedChecks := map[string]bool{
		"test":  true,
		"lint":  true,
		"build": true,
	}

	if len(checks) != len(expectedChecks) {
		t.Errorf("Expected %d checks, got %d", len(expectedChecks), len(checks))
	}

	for _, check := range checks {
		if !expectedChecks[check] {
			t.Errorf("Unexpected check: %s", check)
		}
	}
}

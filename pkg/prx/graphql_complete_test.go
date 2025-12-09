package prx

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/codeGROOVE-dev/sfcache"
)

func TestIsBot(t *testing.T) {
	tests := []struct {
		name   string
		actor  graphQLActor
		wantIs bool
	}{
		{
			name:   "empty actor",
			actor:  graphQLActor{},
			wantIs: false,
		},
		{
			name:   "type is bot",
			actor:  graphQLActor{Login: "someuser", Type: "Bot"},
			wantIs: true,
		},
		{
			name:   "github app bot with [bot] suffix",
			actor:  graphQLActor{Login: "dependabot[bot]"},
			wantIs: true,
		},
		{
			name:   "bot with -bot suffix lowercase",
			actor:  graphQLActor{Login: "my-bot"},
			wantIs: true,
		},
		{
			name:   "bot with _bot suffix lowercase",
			actor:  graphQLActor{Login: "my_bot"},
			wantIs: true,
		},
		{
			name:   "bot with -robot suffix",
			actor:  graphQLActor{Login: "my-robot"},
			wantIs: true,
		},
		{
			name:   "bot with bot- prefix",
			actor:  graphQLActor{Login: "bot-mybot"},
			wantIs: true,
		},
		{
			name:   "dependabot (ends with bot)",
			actor:  graphQLActor{Login: "dependabot"},
			wantIs: true,
		},
		{
			name:   "renovatebot (ends with bot)",
			actor:  graphQLActor{Login: "renovatebot"},
			wantIs: true,
		},
		{
			name:   "bot ID prefix",
			actor:  graphQLActor{Login: "someuser", ID: "BOT_12345"},
			wantIs: true,
		},
		{
			name:   "bot ID contains Bot",
			actor:  graphQLActor{Login: "someuser", ID: "someBot12345"},
			wantIs: true,
		},
		{
			name:   "regular user",
			actor:  graphQLActor{Login: "regularuser"},
			wantIs: false,
		},
		{
			name:   "user with bot in name but not suffix",
			actor:  graphQLActor{Login: "mybotuser"},
			wantIs: false, // "user" comes after "bot"
		},
		{
			name:   "short bot (3 chars)",
			actor:  graphQLActor{Login: "bot"},
			wantIs: false, // len check prevents matching "bot" exactly
		},
		{
			name:   "mixed case bot suffix",
			actor:  graphQLActor{Login: "MyBot"},
			wantIs: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isBot(tt.actor)
			if got != tt.wantIs {
				t.Errorf("isBot() = %v, want %v for actor %+v", got, tt.wantIs, tt.actor)
			}
		})
	}
}

func TestGraphQLActor(t *testing.T) {
	actor := graphQLActor{
		Login: "testuser",
		ID:    "user123",
		Type:  "User",
	}

	if actor.Login != "testuser" {
		t.Errorf("Expected login 'testuser', got '%s'", actor.Login)
	}
	if actor.ID != "user123" {
		t.Errorf("Expected ID 'user123', got '%s'", actor.ID)
	}
	if actor.Type != "User" {
		t.Errorf("Expected type 'User', got '%s'", actor.Type)
	}
}

func TestGraphQLPageInfo(t *testing.T) {
	pageInfo := graphQLPageInfo{
		EndCursor:   "cursor123",
		HasNextPage: true,
	}

	if pageInfo.EndCursor != "cursor123" {
		t.Errorf("Expected EndCursor 'cursor123', got '%s'", pageInfo.EndCursor)
	}
	if !pageInfo.HasNextPage {
		t.Errorf("Expected HasNextPage to be true")
	}

	emptyPageInfo := graphQLPageInfo{}
	if emptyPageInfo.HasNextPage {
		t.Errorf("Expected empty HasNextPage to be false")
	}
}

func TestConvertGraphQLReviewCommentsWithOutdated(t *testing.T) {
	client := &Client{
		logger:             slog.Default(),
		collaboratorsCache: sfcache.New[string, map[string]string](sfcache.TTL(collaboratorsCacheTTL)),
	}
	ctx := context.Background()

	// Create test data with review threads containing outdated comments
	data := &graphQLPullRequestComplete{
		ReviewThreads: struct {
			Nodes []struct {
				Comments struct {
					Nodes []struct {
						CreatedAt         time.Time    `json:"createdAt"`
						Author            graphQLActor `json:"author"`
						ID                string       `json:"id"`
						Body              string       `json:"body"`
						Outdated          bool         `json:"outdated"`
						AuthorAssociation string       `json:"authorAssociation"`
					} `json:"nodes"`
				} `json:"comments"`
				IsResolved bool `json:"isResolved"`
				IsOutdated bool `json:"isOutdated"`
			} `json:"nodes"`
		}{
			Nodes: []struct {
				Comments struct {
					Nodes []struct {
						CreatedAt         time.Time    `json:"createdAt"`
						Author            graphQLActor `json:"author"`
						ID                string       `json:"id"`
						Body              string       `json:"body"`
						Outdated          bool         `json:"outdated"`
						AuthorAssociation string       `json:"authorAssociation"`
					} `json:"nodes"`
				} `json:"comments"`
				IsResolved bool `json:"isResolved"`
				IsOutdated bool `json:"isOutdated"`
			}{
				{
					IsOutdated: true,
					IsResolved: true,
					Comments: struct {
						Nodes []struct {
							CreatedAt         time.Time    `json:"createdAt"`
							Author            graphQLActor `json:"author"`
							ID                string       `json:"id"`
							Body              string       `json:"body"`
							Outdated          bool         `json:"outdated"`
							AuthorAssociation string       `json:"authorAssociation"`
						} `json:"nodes"`
					}{
						Nodes: []struct {
							CreatedAt         time.Time    `json:"createdAt"`
							Author            graphQLActor `json:"author"`
							ID                string       `json:"id"`
							Body              string       `json:"body"`
							Outdated          bool         `json:"outdated"`
							AuthorAssociation string       `json:"authorAssociation"`
						}{
							{
								ID:                "comment1",
								Body:              "Should be Unlock() I think?",
								CreatedAt:         time.Date(2025, 7, 18, 16, 46, 27, 0, time.UTC),
								Outdated:          true,
								Author:            graphQLActor{Login: "reviewer1"},
								AuthorAssociation: "CONTRIBUTOR",
							},
							{
								ID:                "comment2",
								Body:              "eh yeah, absolutely! Good catch!",
								CreatedAt:         time.Date(2025, 7, 18, 16, 50, 21, 0, time.UTC),
								Outdated:          true,
								Author:            graphQLActor{Login: "author1"},
								AuthorAssociation: "OWNER",
							},
						},
					},
				},
				{
					IsOutdated: false,
					IsResolved: false,
					Comments: struct {
						Nodes []struct {
							CreatedAt         time.Time    `json:"createdAt"`
							Author            graphQLActor `json:"author"`
							ID                string       `json:"id"`
							Body              string       `json:"body"`
							Outdated          bool         `json:"outdated"`
							AuthorAssociation string       `json:"authorAssociation"`
						} `json:"nodes"`
					}{
						Nodes: []struct {
							CreatedAt         time.Time    `json:"createdAt"`
							Author            graphQLActor `json:"author"`
							ID                string       `json:"id"`
							Body              string       `json:"body"`
							Outdated          bool         `json:"outdated"`
							AuthorAssociation string       `json:"authorAssociation"`
						}{
							{
								ID:                "comment3",
								Body:              "This looks good to me",
								CreatedAt:         time.Date(2025, 7, 19, 10, 0, 0, 0, time.UTC),
								Outdated:          false,
								Author:            graphQLActor{Login: "reviewer2"},
								AuthorAssociation: "MEMBER",
							},
						},
					},
				},
			},
		},
	}

	// Convert GraphQL data to events
	events := client.convertGraphQLToEventsComplete(ctx, data, "testowner", "testrepo")

	// Filter to only review_comment events
	var reviewComments []Event
	for _, event := range events {
		if event.Kind == "review_comment" {
			reviewComments = append(reviewComments, event)
		}
	}

	// Verify we got 3 review comments
	if len(reviewComments) != 3 {
		t.Fatalf("Expected 3 review comments, got %d", len(reviewComments))
	}

	// Verify first comment is outdated
	if !reviewComments[0].Outdated {
		t.Errorf("Expected first comment to be outdated")
	}
	if reviewComments[0].Body != "Should be Unlock() I think?" {
		t.Errorf("Expected first comment body 'Should be Unlock() I think?', got '%s'", reviewComments[0].Body)
	}

	// Verify second comment is outdated
	if !reviewComments[1].Outdated {
		t.Errorf("Expected second comment to be outdated")
	}
	if reviewComments[1].Body != "eh yeah, absolutely! Good catch!" {
		t.Errorf("Expected second comment body 'eh yeah, absolutely! Good catch!', got '%s'", reviewComments[1].Body)
	}

	// Verify third comment is NOT outdated
	if reviewComments[2].Outdated {
		t.Errorf("Expected third comment to NOT be outdated")
	}
	if reviewComments[2].Body != "This looks good to me" {
		t.Errorf("Expected third comment body 'This looks good to me', got '%s'", reviewComments[2].Body)
	}
}

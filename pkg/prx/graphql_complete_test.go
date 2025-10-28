package prx

import (
	"testing"
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

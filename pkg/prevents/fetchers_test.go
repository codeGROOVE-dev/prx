package prevents

import (
	"log/slog"
	"testing"

	"github.com/google/go-github/v57/github"
)

func TestIsBot(t *testing.T) {
	tests := []struct {
		name     string
		user     *github.User
		expected bool
	}{
		{
			name:     "nil user",
			user:     nil,
			expected: false,
		},
		{
			name: "GitHub App bot",
			user: &github.User{
				Login: github.String("github-actions"),
				Type:  github.String("Bot"),
			},
			expected: true,
		},
		{
			name: "user with -bot suffix",
			user: &github.User{
				Login: github.String("dependabot-bot"),
				Type:  github.String("User"),
			},
			expected: true,
		},
		{
			name: "user with [bot] suffix",
			user: &github.User{
				Login: github.String("renovate[bot]"),
				Type:  github.String("User"),
			},
			expected: true,
		},
		{
			name: "regular user",
			user: &github.User{
				Login: github.String("octocat"),
				Type:  github.String("User"),
			},
			expected: false,
		},
		{
			name: "user with -robot suffix",
			user: &github.User{
				Login: github.String("k8s-ci-robot"),
				Type:  github.String("User"),
			},
			expected: true,
		},
		{
			name: "user with bot in middle of name",
			user: &github.User{
				Login: github.String("robot-user"),
				Type:  github.String("User"),
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isBot(tt.user)
			if result != tt.expected {
				t.Errorf("isBot(%v) = %v, want %v", tt.user, result, tt.expected)
			}
		})
	}
}

func TestParseTimelineEvent_Targets(t *testing.T) {
	c := &Client{logger: slog.Default()}
	
	tests := []struct {
		name            string
		event           *github.Timeline
		expectedTargets []string
	}{
		{
			name: "assigned event with target",
			event: &github.Timeline{
				Event:     github.String("assigned"),
				CreatedAt: &github.Timestamp{},
				Actor:     &github.User{Login: github.String("manager")},
				Assignee:  &github.User{Login: github.String("developer1")},
			},
			expectedTargets: []string{"developer1"},
		},
		{
			name: "review_requested event with user target",
			event: &github.Timeline{
				Event:     github.String("review_requested"),
				CreatedAt: &github.Timestamp{},
				Actor:     &github.User{Login: github.String("author")},
				Reviewer:  &github.User{Login: github.String("reviewer1")},
			},
			expectedTargets: []string{"reviewer1"},
		},
		{
			name: "review_requested event with team target",
			event: &github.Timeline{
				Event:         github.String("review_requested"),
				CreatedAt:     &github.Timestamp{},
				Actor:         &github.User{Login: github.String("author")},
				RequestedTeam: &github.Team{Name: github.String("backend-team")},
			},
			expectedTargets: []string{"backend-team"},
		},
		{
			name: "labeled event with target",
			event: &github.Timeline{
				Event:     github.String("labeled"),
				CreatedAt: &github.Timestamp{},
				Actor:     &github.User{Login: github.String("triager")},
				Label:     &github.Label{Name: github.String("bug")},
			},
			expectedTargets: []string{"bug"},
		},
		{
			name: "milestoned event with target",
			event: &github.Timeline{
				Event:     github.String("milestoned"),
				CreatedAt: &github.Timestamp{},
				Actor:     &github.User{Login: github.String("pm")},
				Milestone: &github.Milestone{Title: github.String("v2.0 Release")},
			},
			expectedTargets: []string{"v2.0 Release"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := c.parseTimelineEvent(tt.event)
			if event == nil {
				t.Fatal("expected non-nil event")
			}
			
			if len(event.Targets) != len(tt.expectedTargets) {
				t.Errorf("expected %d targets, got %d", len(tt.expectedTargets), len(event.Targets))
				return
			}
			
			for i, target := range event.Targets {
				if target != tt.expectedTargets[i] {
					t.Errorf("expected target[%d] = %q, got %q", i, tt.expectedTargets[i], target)
				}
			}
		})
	}
}
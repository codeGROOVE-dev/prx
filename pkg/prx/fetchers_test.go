package prx

import (
	"log/slog"
	"testing"
	"time"
)

func TestIsBot(t *testing.T) {
	tests := []struct {
		name     string
		user     *githubUser
		expected bool
	}{
		{
			name:     "nil user",
			user:     nil,
			expected: false,
		},
		{
			name: "GitHub App bot",
			user: &githubUser{
				Login: "github-actions",
				Type:  "Bot",
			},
			expected: true,
		},
		{
			name: "user with -bot suffix",
			user: &githubUser{
				Login: "dependabot-bot",
				Type:  "User",
			},
			expected: true,
		},
		{
			name: "user with [bot] suffix",
			user: &githubUser{
				Login: "renovate[bot]",
				Type:  "User",
			},
			expected: true,
		},
		{
			name: "regular user",
			user: &githubUser{
				Login: "octocat",
				Type:  "User",
			},
			expected: false,
		},
		{
			name: "user with -robot suffix",
			user: &githubUser{
				Login: "k8s-ci-robot",
				Type:  "User",
			},
			expected: true,
		},
		{
			name: "user with bot in middle of name",
			user: &githubUser{
				Login: "robot-user",
				Type:  "User",
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
		event           *githubTimelineEvent
		expectedTargets []string
	}{
		{
			name: "assigned event with target",
			event: &githubTimelineEvent{
				Event:     "assigned",
				CreatedAt: time.Now(),
				Actor:     &githubUser{Login: "manager"},
				Assignee:  &githubUser{Login: "developer1"},
			},
			expectedTargets: []string{"developer1"},
		},
		{
			name: "review_requested event with user target",
			event: &githubTimelineEvent{
				Event:             "review_requested",
				CreatedAt:         time.Now(),
				Actor:             &githubUser{Login: "author"},
				RequestedReviewer: &githubUser{Login: "reviewer1"},
			},
			expectedTargets: []string{"reviewer1"},
		},
		{
			name: "review_requested event with team target",
			event: &githubTimelineEvent{
				Event:     "review_requested",
				CreatedAt: time.Now(),
				Actor:     &githubUser{Login: "author"},
				RequestedTeam: struct {
					Name string `json:"name"`
				}{Name: "backend-team"},
			},
			expectedTargets: []string{"backend-team"},
		},
		{
			name: "labeled event with target",
			event: &githubTimelineEvent{
				Event:     "labeled",
				CreatedAt: time.Now(),
				Actor:     &githubUser{Login: "triager"},
				Label: struct {
					Name string `json:"name"`
				}{Name: "bug"},
			},
			expectedTargets: []string{"bug"},
		},
		{
			name: "milestoned event with target",
			event: &githubTimelineEvent{
				Event:     "milestoned",
				CreatedAt: time.Now(),
				Actor:     &githubUser{Login: "pm"},
				Milestone: struct {
					Title string `json:"title"`
				}{Title: "v2.0 Release"},
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

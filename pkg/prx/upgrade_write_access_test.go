package prx

import (
	"testing"
	"time"
)

func TestUpgradeWriteAccess(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		events   []Event
		expected map[string]int // actor -> expected write access
	}{
		{
			name: "upgrade write access for user who merged PR",
			events: []Event{
				{
					Kind:        "comment",
					Timestamp:   now.Add(-2 * time.Hour),
					Actor:       "reviewer1",
					WriteAccess: WriteAccessLikely, // 1
				},
				{
					Kind:      "pr_merged",
					Timestamp: now.Add(-1 * time.Hour),
					Actor:     "reviewer1",
				},
			},
			expected: map[string]int{
				"reviewer1": WriteAccessDefinitely, // Should be upgraded to 2
			},
		},
		{
			name: "upgrade write access for user who labeled issue",
			events: []Event{
				{
					Kind:        "review",
					Timestamp:   now.Add(-3 * time.Hour),
					Actor:       "maintainer",
					WriteAccess: WriteAccessLikely, // 1
					Outcome:     "approved",
				},
				{
					Kind:      "labeled",
					Timestamp: now.Add(-2 * time.Hour),
					Actor:     "maintainer",
					Target:    "bug",
				},
			},
			expected: map[string]int{
				"maintainer": WriteAccessDefinitely, // Should be upgraded to 2
			},
		},
		{
			name: "don't upgrade if already definitely has write access",
			events: []Event{
				{
					Kind:        "comment",
					Timestamp:   now.Add(-2 * time.Hour),
					Actor:       "owner",
					WriteAccess: WriteAccessDefinitely, // Already 2
				},
				{
					Kind:      "pr_merged",
					Timestamp: now.Add(-1 * time.Hour),
					Actor:     "owner",
				},
			},
			expected: map[string]int{
				"owner": WriteAccessDefinitely, // Should remain 2
			},
		},
		{
			name: "don't upgrade if user has no write access",
			events: []Event{
				{
					Kind:        "comment",
					Timestamp:   now.Add(-2 * time.Hour),
					Actor:       "contributor",
					WriteAccess: WriteAccessUnlikely, // -1
				},
				{
					Kind:      "comment",
					Timestamp: now.Add(-1 * time.Hour),
					Actor:     "contributor",
					Body:      "Please merge this",
				},
			},
			expected: map[string]int{
				"contributor": WriteAccessUnlikely, // Should remain 0
			},
		},
		{
			name: "upgrade multiple users based on different actions",
			events: []Event{
				{
					Kind:        "comment",
					Timestamp:   now.Add(-4 * time.Hour),
					Actor:       "user1",
					WriteAccess: WriteAccessLikely, // 1
				},
				{
					Kind:        "review",
					Timestamp:   now.Add(-3 * time.Hour),
					Actor:       "user2",
					WriteAccess: WriteAccessLikely, // 1
					Outcome:     "approved",
				},
				{
					Kind:      "assigned",
					Timestamp: now.Add(-2 * time.Hour),
					Actor:     "user1",
					Target:    "assignee1",
				},
				{
					Kind:      "milestoned",
					Timestamp: now.Add(-1 * time.Hour),
					Actor:     "user2",
				},
			},
			expected: map[string]int{
				"user1": WriteAccessDefinitely, // Should be upgraded to 2
				"user2": WriteAccessDefinitely, // Should be upgraded to 2
			},
		},
		{
			name: "handle events with nil write access",
			events: []Event{
				{
					Kind:        "comment",
					Timestamp:   now.Add(-2 * time.Hour),
					Actor:       "user1",
					WriteAccess: WriteAccessNA, // no write access info
				},
				{
					Kind:      "labeled",
					Timestamp: now.Add(-1 * time.Hour),
					Actor:     "user1",
				},
			},
			expected: map[string]int{
				// user1 should not have write access modified since it was nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy of events to avoid modifying test data
			events := make([]Event, len(tt.events))
			copy(events, tt.events)

			// Apply the upgrade function
			upgradeWriteAccess(events)

			// Check results - look for events with WriteAccess field
			for _, event := range events {
				if event.WriteAccess != WriteAccessNA {
					if expectedAccess, ok := tt.expected[event.Actor]; ok {
						if event.WriteAccess != expectedAccess {
							t.Errorf("%s: Actor %s has write access %d, expected %d",
								tt.name, event.Actor, event.WriteAccess, expectedAccess)
						}
					}
				}
			}
		})
	}
}

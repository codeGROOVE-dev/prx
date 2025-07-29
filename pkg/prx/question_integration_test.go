package prx

import (
	"testing"
)

func TestQuestionFieldIntegration(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		outcome  string
		expected bool
	}{
		{
			name:     "approval with question",
			body:     "Looks good. Could you test and confirm that this driver works for both `docker-machine` and `minikube`?",
			outcome:  "APPROVED",
			expected: true,
		},
		{
			name:     "changes requested with question",
			body:     "How should we handle the error case here?",
			outcome:  "changes_requested",
			expected: true,
		},
		{
			name:     "approval without question",
			body:     "LGTM! Great work on this PR.",
			outcome:  "APPROVED",
			expected: false,
		},
		{
			name:     "comment with implicit question",
			body:     "I need help understanding this logic. Can you explain the rationale",
			outcome:  "",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate creating an event like in reviews
			event := Event{
				Kind:     "review",
				Body:     tt.body,
				Outcome:  tt.outcome,
				Question: containsQuestion(tt.body),
			}

			if event.Question != tt.expected {
				t.Errorf("expected Question=%v for body %q, got %v", tt.expected, tt.body, event.Question)
			}

			// Verify outcome is preserved
			if event.Outcome != tt.outcome {
				t.Errorf("expected Outcome=%q, got %q", tt.outcome, event.Outcome)
			}
		})
	}
}

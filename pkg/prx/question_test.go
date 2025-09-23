package prx

import (
	"testing"
)

func TestContainsQuestion(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Basic question mark detection
		{
			name:     "simple question",
			input:    "What do you think?",
			expected: true,
		},
		{
			name:     "single word question",
			input:    "help?",
			expected: true,
		},
		{
			name:     "username mention question",
			input:    "@tstromberg?",
			expected: true,
		},
		{
			name:     "question in middle",
			input:    "I wonder if this is correct? Let me know.",
			expected: true,
		},
		{
			name:     "multiple questions",
			input:    "Is this okay? What about that?",
			expected: true,
		},
		{
			name:     "no question",
			input:    "This is a statement.",
			expected: false,
		},

		// Edge cases
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "very short text",
			input:    "Hi",
			expected: false,
		},
		{
			name:     "just question mark",
			input:    "?",
			expected: true,
		},
		{
			name:     "whitespace only",
			input:    "   \n\t  ",
			expected: false,
		},

		// Pattern-based questions (positive cases)
		{
			name:     "how can pattern",
			input:    "How can I fix this issue?",
			expected: true,
		},
		{
			name:     "how do pattern",
			input:    "How do we implement this feature",
			expected: true,
		},
		{
			name:     "should I pattern",
			input:    "Should I refactor this code",
			expected: true,
		},
		{
			name:     "can you pattern",
			input:    "Can you help me understand this",
			expected: true,
		},
		{
			name:     "any suggestions",
			input:    "Any suggestions for improving performance",
			expected: true,
		},
		{
			name:     "thoughts on",
			input:    "Thoughts on this approach",
			expected: true,
		},
		{
			name:     "wondering if",
			input:    "I'm wondering if we should use a different library",
			expected: true,
		},
		{
			name:     "what do you think",
			input:    "This seems to work well. What do you think about the performance",
			expected: true,
		},
		{
			name:     "need help",
			input:    "I need help with the authentication logic",
			expected: true,
		},
		{
			name:     "is it possible",
			input:    "Is it possible to add caching here",
			expected: true,
		},
		{
			name:     "could you review",
			input:    "Could you review this implementation",
			expected: true,
		},
		{
			name:     "can someone help",
			input:    "Can someone help with this issue",
			expected: true,
		},
		{
			name:     "can anyone review",
			input:    "Can anyone review my changes",
			expected: true,
		},
		{
			name:     "anyone know why",
			input:    "anyone know why the tests are broken",
			expected: true,
		},
		{
			name:     "does someone know",
			input:    "Does someone know how to fix this",
			expected: true,
		},
		{
			name:     "could someone explain",
			input:    "Could someone explain this behavior",
			expected: true,
		},
		{
			name:     "would anyone mind",
			input:    "Would anyone mind reviewing this PR",
			expected: true,
		},
		{
			name:     "has anyone seen",
			input:    "Has anyone seen this error before",
			expected: true,
		},
		{
			name:     "who can help",
			input:    "Who can help me understand this",
			expected: true,
		},
		{
			name:     "do you know",
			input:    "Do you know if this is expected behavior",
			expected: true,
		},
		{
			name:     "will this work",
			input:    "Will this work with the new API",
			expected: true,
		},
		{
			name:     "should this be",
			input:    "Should this be using async/await",
			expected: true,
		},
		{
			name:     "anyone else notice",
			input:    "Anyone else notice the performance regression",
			expected: true,
		},
		{
			name:     "why is pattern",
			input:    "Why is this failing",
			expected: true,
		},
		{
			name:     "when should pattern",
			input:    "When should we release this",
			expected: true,
		},
		{
			name:     "where should pattern",
			input:    "Where should I put this configuration",
			expected: true,
		},
		{
			name:     "which one pattern",
			input:    "Which one is better for our use case",
			expected: true,
		},

		// Case insensitivity
		{
			name:     "case insensitive upper",
			input:    "HOW CAN we make this better",
			expected: true,
		},
		{
			name:     "case insensitive mixed",
			input:    "CaN yOu help with this",
			expected: true,
		},

		// Word boundary tests (should NOT match)
		{
			name:     "can i in middle of word",
			input:    "I can iterate through the list",
			expected: false,
		},
		{
			name:     "can i in American Institute",
			input:    "The American Institute of Technology",
			expected: false,
		},
		{
			name:     "need help in compound word",
			input:    "I need helpers for the project",
			expected: false,
		},
		{
			name:     "advice on in compound",
			input:    "The advice online was helpful",
			expected: false,
		},
		{
			name:     "how do in compound",
			input:    "Show documentation for this",
			expected: false,
		},
		{
			name:     "is it in visit",
			input:    "This iteration is complete",
			expected: false,
		},
		{
			name:     "any thoughts in company",
			input:    "Company thoughtsphere is innovative",
			expected: false,
		},
		{
			name:     "scan someone not can someone",
			input:    "We scan someone's credentials",
			expected: false,
		},
		{
			name:     "everyone knows not anyone know",
			input:    "Everyone knows that this is the standard",
			expected: false,
		},
		{
			name:     "has anyonething compound",
			input:    "This class has anyonething property",
			expected: false,
		},
		{
			name:     "mexican someone not can someone",
			input:    "Mexican someones are delicious",
			expected: false,
		},
		{
			name:     "who in relative clause",
			input:    "The developer who wrote this did a great job",
			expected: false,
		},
		{
			name:     "what in compound whatsoever",
			input:    "There are no issues whatsoever",
			expected: false,
		},
		{
			name:     "scan anyone's not can anyone",
			input:    "We'll scan anyone's PR for issues",
			expected: false,
		},

		// Multiple spaces between words
		{
			name:     "multiple spaces between words",
			input:    "Can  you    help   me",
			expected: true,
		},
		{
			name:     "tabs between words",
			input:    "How\tcan\tI\tfix\tthis",
			expected: true,
		},
		{
			name:     "newlines between words",
			input:    "What do\nyou\nthink about this",
			expected: true,
		},

		// Real-world examples
		{
			name:     "PR review request",
			input:    "@johndoe could you take a look at this PR when you get a chance",
			expected: true,
		},
		{
			name:     "code review comment",
			input:    "LGTM, but wondering if we should add more tests",
			expected: true,
		},
		{
			name:     "bug report",
			input:    "Is this a known issue or should I file a bug report",
			expected: true,
		},
		{
			name:     "implementation question",
			input:    "How should we handle the edge case where the user has no permissions",
			expected: true,
		},
		{
			name:     "not a question statement",
			input:    "Fixed the bug and added tests. The CI should pass now.",
			expected: false,
		},
		{
			name:     "confirmation message",
			input:    "I've updated the code according to your feedback",
			expected: false,
		},

		// Punctuation variations
		{
			name:     "question with exclamation",
			input:    "Can you believe this works!?",
			expected: true,
		},
		{
			name:     "pattern at end of sentence",
			input:    "Let me know if you need help.",
			expected: true,
		},
		{
			name:     "pattern with comma",
			input:    "Can you, if possible, review this today",
			expected: true,
		},

		// Special characters that shouldn't break regex
		{
			name:     "regex special chars in text",
			input:    "The regex (.*) pattern. Can you help fix it",
			expected: true,
		},
		{
			name:     "brackets and parentheses",
			input:    "The function foo() returns [1,2,3]. Is it possible to change this",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsQuestion(tt.input)
			if result != tt.expected {
				t.Errorf("containsQuestion(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

// Benchmark to ensure performance is acceptable
func BenchmarkContainsQuestion(b *testing.B) {
	testCases := []string{
		"Can you review this PR?",
		"This is a simple statement without any questions.",
		"I need help understanding the authentication flow",
		"The American Institute of Technology is prestigious",
		"What do you think about using GraphQL instead of REST for this API endpoint implementation?",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, tc := range testCases {
			_ = containsQuestion(tc)
		}
	}
}

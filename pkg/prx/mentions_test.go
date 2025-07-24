package prx

import (
	"reflect"
	"testing"
)

func TestExtractMentions(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "single mention",
			input:    "Hey @tstromberg, can you review this?",
			expected: []string{"tstromberg"},
		},
		{
			name:     "multiple mentions",
			input:    "@alice and @bob should look at this. cc @charlie",
			expected: []string{"alice", "bob", "charlie"},
		},
		{
			name:     "duplicate mentions",
			input:    "@user1 please check. @user1 what do you think?",
			expected: []string{"user1"},
		},
		{
			name:     "no mentions",
			input:    "This is a comment without any mentions",
			expected: []string{},
		},
		{
			name:     "mention with hyphen",
			input:    "@user-name and @another-user-123",
			expected: []string{"user-name", "another-user-123"},
		},
		{
			name:     "single character username",
			input:    "@a @b @c",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "email should not match",
			input:    "Contact us at support@example.com",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractMentions(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("extractMentions(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestContainsQuestion(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "simple question",
			input:    "What do you think?",
			expected: true,
		},
		{
			name:     "question in middle",
			input:    "I wonder if this is correct? Let me know.",
			expected: true,
		},
		{
			name:     "no question",
			input:    "This is a statement.",
			expected: false,
		},
		{
			name:     "multiple questions",
			input:    "Is this okay? What about that?",
			expected: true,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
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
			name:     "case insensitive",
			input:    "HOW CAN we make this better",
			expected: true,
		},
		{
			name:     "no question patterns",
			input:    "The implementation looks good to me",
			expected: false,
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

package prx

import (
	"regexp"
	"sort"
	"strings"
)

// mentionRegex matches GitHub usernames in the format @username
// GitHub usernames can contain alphanumeric characters and hyphens, but not consecutive hyphens
var mentionRegex = regexp.MustCompile(`(?:^|[^a-zA-Z0-9])@([a-zA-Z0-9][a-zA-Z0-9\-]{0,38}[a-zA-Z0-9]|[a-zA-Z0-9])`)

// questionPatterns contains common patterns that indicate a question or request for advice
var questionPatterns = []string{
	"how can",
	"how do",
	"how would",
	"how should",
	"should i",
	"should we",
	"can i",
	"can we",
	"can you",
	"could you",
	"would you",
	"what do you think",
	"what's the best",
	"what is the best",
	"any suggestions",
	"any ideas",
	"any thoughts",
	"anyone know",
	"does anyone",
	"is it possible",
	"is there a way",
	"wondering if",
	"thoughts on",
	"advice on",
	"help with",
	"need help",
}

// extractMentions extracts all @username mentions from a text string
func extractMentions(text string) []string {
	matches := mentionRegex.FindAllStringSubmatch(text, -1)
	mentions := make([]string, 0, len(matches))
	seen := make(map[string]bool)

	for _, match := range matches {
		if len(match) > 1 {
			username := match[1]
			if !seen[username] {
				mentions = append(mentions, username)
				seen[username] = true
			}
		}
	}

	return mentions
}

// containsQuestion checks if the text contains patterns that indicate a question or request for advice
func containsQuestion(text string) bool {
	// Check for question mark first (fast path)
	if strings.Contains(text, "?") {
		return true
	}

	// Convert to lowercase only if needed for pattern matching
	lowerText := strings.ToLower(text)

	// Check for common question patterns
	for _, pattern := range questionPatterns {
		if strings.Contains(lowerText, pattern) {
			return true
		}
	}

	return false
}

// sortEventsByTimestamp sorts events by timestamp in ascending order.
func sortEventsByTimestamp(events []Event) {
	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp.Before(events[j].Timestamp)
	})
}

// isHexString checks if a string contains only hexadecimal characters.
func isHexString(s string) bool {
	// Performance optimization: work with bytes instead of runes for ASCII-only hex strings
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

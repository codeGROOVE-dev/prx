package prx

import (
	"regexp"
	"sort"
	"strings"
)

var mentionRegex = regexp.MustCompile(`(?:^|[^a-zA-Z0-9])@([a-zA-Z0-9][a-zA-Z0-9\-]{0,38}[a-zA-Z0-9]|[a-zA-Z0-9])`)

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

func containsQuestion(text string) bool {
	if strings.Contains(text, "?") {
		return true
	}
	lowerText := strings.ToLower(text)
	for _, pattern := range questionPatterns {
		if strings.Contains(lowerText, pattern) {
			return true
		}
	}
	return false
}

func sortEventsByTimestamp(events []Event) {
	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp.Before(events[j].Timestamp)
	})
}

func isHexString(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

func calculateTestSummary(events []Event) *TestSummary {
	summary := &TestSummary{}
	checkStates := make(map[string]string)
	
	for _, event := range events {
		if event.Kind == CheckRun && event.Body != "" {
			checkStates[event.Body] = event.Outcome
		}
	}
	
	for _, outcome := range checkStates {
		switch outcome {
		case "success":
			summary.Passing++
		case "failure", "timed_out", "action_required":
			summary.Failing++
		case "", "neutral", "cancelled", "skipped", "stale", "queued", "in_progress", "pending":
			summary.Pending++
		}
	}
	
	return summary
}

func calculateStatusSummary(events []Event) *StatusSummary {
	summary := &StatusSummary{}
	checkStates := make(map[string]string)
	
	for _, event := range events {
		if (event.Kind == StatusCheck || event.Kind == CheckRun) && event.Body != "" {
			key := string(event.Kind) + ":" + event.Body
			checkStates[key] = event.Outcome
		}
	}
	
	for _, outcome := range checkStates {
		switch outcome {
		case "success":
			summary.Success++
		case "failure", "error", "timed_out", "action_required":
			summary.Failure++
		case "pending", "queued", "in_progress", "waiting":
			summary.Pending++
		case "neutral", "cancelled", "skipped", "stale":
			summary.Neutral++
		}
	}
	
	return summary
}

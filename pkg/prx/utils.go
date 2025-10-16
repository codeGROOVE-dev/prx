package prx

import (
	"regexp"
	"strings"
	"sync"
)

const (
	// maxTruncateLength is the default truncation length for text fields.
	maxTruncateLength = 256
)

// questionPatterns defines phrases that typically indicate a question is being asked.
// Each pattern will be compiled into a regex with word boundaries to avoid false positives.
var questionPatterns = []string{
	// Direct questions
	"how can",
	"how do",
	"how would",
	"how should",
	"how about",
	"how to",
	// Modal questions
	"should i",
	"should we",
	"should this",
	"can i",
	"can we",
	"can you",
	"can someone",
	"can anyone",
	"could i",
	"could we",
	"could you",
	"could someone",
	"could anyone",
	"would you",
	"would someone",
	"would anyone",
	"will you",
	"will this",
	"may i",
	// What questions
	"what do you think",
	"what's the",
	"what is the",
	"what are",
	"what about",
	// Request patterns
	"any suggestions",
	"any ideas",
	"any thoughts",
	"any feedback",
	"anyone know",
	"anyone else",
	"someone know",
	"does anyone",
	"does someone",
	"does this",
	"do we",
	"do you",
	// Possibility questions
	"is it possible",
	"is there a way",
	"is this",
	"is that",
	"are there",
	"are we",
	// Indirect questions
	"wondering if",
	"thoughts on",
	"advice on",
	"help with",
	"need help",
	// Why/when/where/who questions
	"why is",
	"why does",
	"when should",
	"when can",
	"when will",
	"where should",
	"where can",
	"where is",
	"which one",
	"which is",
	"who can",
	"who is",
	"who knows",
	// Have/has questions
	"have you",
	"has anyone",
	"has someone",
}

// questionRegexCache caches compiled regexes for performance.
var (
	questionRegexCache map[string]*regexp.Regexp
	questionRegexOnce  sync.Once
)

// initQuestionRegexes compiles all question patterns into regexes with word boundaries.
// This is done once on first use to avoid repeated compilation.
func initQuestionRegexes() {
	questionRegexCache = make(map[string]*regexp.Regexp, len(questionPatterns))
	for _, p := range questionPatterns {
		// Split pattern into words and add word boundaries
		w := strings.Fields(p)
		if len(w) == 0 {
			continue // Skip empty patterns (defensive programming)
		}
		for i, word := range w {
			w[i] = regexp.QuoteMeta(word)
		}
		// Add word boundaries: \b at start of first word, \b at end of last word
		w[0] = "\\b" + w[0]
		w[len(w)-1] = w[len(w)-1] + "\\b"
		// Join with flexible whitespace and compile as case-insensitive
		questionRegexCache[p] = regexp.MustCompile("(?i)" + strings.Join(w, "\\s+"))
	}
}

// containsQuestion determines if text contains a question based on:
// 1. Presence of a question mark
// 2. Common question patterns with proper word boundaries.
func containsQuestion(text string) bool {
	// Quick check for question mark
	if strings.Contains(text, "?") {
		return true
	}

	// Return false for empty or very short text
	if len(text) < 3 {
		return false
	}

	// Initialize regex cache once
	questionRegexOnce.Do(initQuestionRegexes)

	// Check against compiled patterns
	for _, re := range questionRegexCache {
		if re.MatchString(text) {
			return true
		}
	}

	return false
}

func isHexString(s string) bool {
	for i := range s {
		c := s[i]
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			continue
		}
		return false
	}
	return true
}

func truncate(s string) string {
	if len(s) <= maxTruncateLength {
		return s
	}
	return s[:maxTruncateLength]
}

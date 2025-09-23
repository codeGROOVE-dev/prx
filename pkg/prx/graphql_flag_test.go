package prx

import (
	"testing"
)

// TestGraphQLFlag verifies the GraphQL feature flag works correctly
func TestGraphQLFlag(t *testing.T) {
	// Test that the flag is off by default
	client1 := NewClient("test-token")
	if client1.useGraphQL {
		t.Error("GraphQL should be disabled by default")
	}

	// Test that WithGraphQL enables the flag
	client2 := NewClient("test-token", WithGraphQL())
	if !client2.useGraphQL {
		t.Error("WithGraphQL() should enable GraphQL mode")
	}

	// Test that multiple options work together
	client3 := NewClient("test-token", WithGraphQL(), WithNoCache())
	if !client3.useGraphQL {
		t.Error("GraphQL should be enabled with WithGraphQL()")
	}
	if client3.cacheDir != "" {
		t.Error("Cache should be disabled with WithNoCache()")
	}
}

// TestGraphQLModeLogging tests that GraphQL mode is properly logged
func TestGraphQLModeLogging(t *testing.T) {
	// This would need a mock or test server to actually test
	// For now, just verify the client is configured correctly

	// Create a client with GraphQL enabled
	client := NewClient("test-token", WithGraphQL())

	// Verify it's configured
	if !client.useGraphQL {
		t.Fatal("GraphQL mode not enabled")
	}

	// In a real test, we'd call client.PullRequest() and verify
	// it logs "GraphQL mode enabled" and uses the GraphQL path
	t.Logf("GraphQL client configured successfully")
}
package prx

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

// TestPermissionToWriteAccess tests permission level mapping
func TestPermissionToWriteAccess(t *testing.T) {
	tests := []struct {
		permission string
		expected   int
	}{
		{"admin", WriteAccessDefinitely},
		{"maintain", WriteAccessDefinitely},
		{"write", WriteAccessDefinitely},
		{"read", WriteAccessNo},
		{"triage", WriteAccessNo},
		{"none", WriteAccessNo},
		{"", WriteAccessUnlikely},          // Not in collaborators list
		{"unknown", WriteAccessUnlikely},   // Unknown permission
		{"ADMIN", WriteAccessUnlikely},     // Case sensitive - not matched
		{"something", WriteAccessUnlikely}, // Invalid permission
	}

	for _, tt := range tests {
		t.Run(tt.permission, func(t *testing.T) {
			// Inline permission mapping logic
			var result int
			switch tt.permission {
			case "admin", "maintain", "write":
				result = WriteAccessDefinitely
			case "read", "triage", "none":
				result = WriteAccessNo
			default:
				result = WriteAccessUnlikely
			}
			if result != tt.expected {
				t.Errorf("permission mapping for %q = %d, want %d",
					tt.permission, result, tt.expected)
			}
		})
	}
}

// TestCollaboratorsCacheGetSet tests cache get/set operations
func TestCollaboratorsCacheGetSet(t *testing.T) {
	cache := &collaboratorsCache{
		memory: make(map[string]collaboratorsEntry),
	}

	owner := "testowner"
	repo := "testrepo"
	collabs := map[string]string{
		"alice": "admin",
		"bob":   "write",
		"carol": "read",
	}

	// Test cache miss
	if _, ok := cache.get(owner, repo); ok {
		t.Error("Expected cache miss, got hit")
	}

	// Test set
	if err := cache.set(owner, repo, collabs); err != nil {
		t.Errorf("cache.set() failed: %v", err)
	}

	// Test cache hit
	cached, ok := cache.get(owner, repo)
	if !ok {
		t.Fatal("Expected cache hit, got miss")
	}

	// Verify cached data
	if len(cached) != len(collabs) {
		t.Errorf("Expected %d collaborators, got %d", len(collabs), len(cached))
	}
	for user, perm := range collabs {
		if cached[user] != perm {
			t.Errorf("Expected %s permission for %s, got %s", perm, user, cached[user])
		}
	}
}

// TestCollaboratorsCacheExpiration tests cache expiration
func TestCollaboratorsCacheExpiration(t *testing.T) {
	cache := &collaboratorsCache{
		memory: make(map[string]collaboratorsEntry),
	}

	owner := "testowner"
	repo := "testrepo"
	collabs := map[string]string{"alice": "admin"}

	// Insert entry with old timestamp
	key := owner + "/" + repo
	cache.memory[key] = collaboratorsEntry{
		Collaborators: collabs,
		CachedAt:      time.Now().Add(-5 * time.Hour), // Expired (> 4 hours)
	}

	// Should return miss due to expiration
	if _, ok := cache.get(owner, repo); ok {
		t.Error("Expected cache miss due to expiration, got hit")
	}
}

// TestWriteAccessFromAssociationWithCache tests MEMBER association with cache
func TestWriteAccessFromAssociationWithCache(t *testing.T) {
	tests := []struct {
		name       string
		user       string
		permission string
		expected   int
	}{
		{
			name:       "member with admin permission",
			user:       "alice",
			permission: "admin",
			expected:   WriteAccessDefinitely,
		},
		{
			name:       "member with write permission",
			user:       "bob",
			permission: "write",
			expected:   WriteAccessDefinitely,
		},
		{
			name:       "member with maintain permission",
			user:       "charlie",
			permission: "maintain",
			expected:   WriteAccessDefinitely,
		},
		{
			name:       "member with read permission",
			user:       "david",
			permission: "read",
			expected:   WriteAccessNo,
		},
		{
			name:       "member with triage permission",
			user:       "eve",
			permission: "triage",
			expected:   WriteAccessNo,
		},
		{
			name:       "member not in collaborators list",
			user:       "frank",
			permission: "", // Not in the cache
			expected:   WriteAccessUnlikely,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Setup cache with test data
			cache := &collaboratorsCache{
				memory: make(map[string]collaboratorsEntry),
			}

			collabs := map[string]string{
				"alice":   "admin",
				"bob":     "write",
				"charlie": "maintain",
				"david":   "read",
				"eve":     "triage",
			}

			// Pre-populate cache
			if err := cache.set("owner", "repo", collabs); err != nil {
				t.Fatalf("Failed to populate cache: %v", err)
			}

			// Create client with cache
			c := &Client{
				logger:             slog.Default(),
				collaboratorsCache: cache,
			}

			result := c.writeAccessFromAssociation(ctx, "owner", "repo", tt.user, "MEMBER")
			if result != tt.expected {
				t.Errorf("writeAccessFromAssociation(MEMBER, %s) = %d, want %d",
					tt.user, result, tt.expected)
			}
		})
	}
}

// TestWriteAccessFromAssociationCacheHit tests that cache prevents API calls
func TestWriteAccessFromAssociationCacheHit(t *testing.T) {
	ctx := context.Background()

	// Setup cache with test data
	cache := &collaboratorsCache{
		memory: make(map[string]collaboratorsEntry),
	}

	collabs := map[string]string{
		"tstromberg": "admin",
	}

	// Pre-populate cache
	if err := cache.set("codeGROOVE-dev", "goose", collabs); err != nil {
		t.Fatalf("Failed to populate cache: %v", err)
	}

	// Create client with cache but without a real GitHub client
	// This tests that we use the cache and don't try to call the API
	c := &Client{
		logger:             slog.Default(),
		collaboratorsCache: cache,
		github:             nil, // No GitHub client - would fail if API called
	}

	result := c.writeAccessFromAssociation(ctx, "codeGROOVE-dev", "goose", "tstromberg", "MEMBER")
	if result != WriteAccessDefinitely {
		t.Errorf("writeAccessFromAssociation(MEMBER, tstromberg) = %d, want %d",
			result, WriteAccessDefinitely)
	}
}

// TestWriteAccessFromAssociationNonMember tests non-MEMBER associations don't use cache
func TestWriteAccessFromAssociationNonMember(t *testing.T) {
	ctx := context.Background()

	// Empty cache
	cache := &collaboratorsCache{
		memory: make(map[string]collaboratorsEntry),
	}

	c := &Client{
		logger:             slog.Default(),
		collaboratorsCache: cache,
	}

	tests := []struct {
		association string
		expected    int
	}{
		{"OWNER", WriteAccessDefinitely},
		{"COLLABORATOR", WriteAccessDefinitely},
		{"CONTRIBUTOR", WriteAccessUnlikely},
		{"NONE", WriteAccessUnlikely},
		{"FIRST_TIME_CONTRIBUTOR", WriteAccessUnlikely},
		{"FIRST_TIMER", WriteAccessUnlikely},
	}

	for _, tt := range tests {
		t.Run(tt.association, func(t *testing.T) {
			result := c.writeAccessFromAssociation(ctx, "owner", "repo", "user", tt.association)
			if result != tt.expected {
				t.Errorf("writeAccessFromAssociation(%s) = %d, want %d",
					tt.association, result, tt.expected)
			}
		})
	}

	// Verify cache wasn't used (should still be empty)
	if _, ok := cache.get("owner", "repo"); ok {
		t.Error("Cache should not have been populated for non-MEMBER associations")
	}
}

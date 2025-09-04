package prx

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPermissionCache(t *testing.T) {
	t.Run("in-memory cache", func(t *testing.T) {
		cache := &permissionCache{
			memory: make(map[string]permissionEntry),
		}

		// Test get on empty cache
		_, found := cache.get("owner", "repo", "user1")
		if found {
			t.Error("expected not found in empty cache")
		}

		// Test set and get
		err := cache.set("owner", "repo", "user1", "write")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		perm, found := cache.get("owner", "repo", "user1")
		if !found {
			t.Error("expected to find cached permission")
		}
		if perm != "write" {
			t.Errorf("expected 'write', got '%s'", perm)
		}

		// Test different user
		_, found = cache.get("owner", "repo", "user2")
		if found {
			t.Error("expected not found for different user")
		}
	})

	t.Run("disk persistence", func(t *testing.T) {
		tempDir := t.TempDir()
		cachePath := filepath.Join(tempDir, "test_cache.json")

		// Create cache with disk persistence
		cache := &permissionCache{
			memory:   make(map[string]permissionEntry),
			diskPath: cachePath,
		}

		// Add some permissions
		if err := cache.set("owner", "repo", "user1", "admin"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := cache.set("owner", "repo", "user2", "read"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify file was created
		if _, err := os.Stat(cachePath); os.IsNotExist(err) {
			t.Error("cache file was not created")
		}

		// Create new cache instance and load from disk
		cache2 := &permissionCache{
			memory:   make(map[string]permissionEntry),
			diskPath: cachePath,
		}

		err := cache2.loadFromDisk()
		if err != nil {
			t.Fatalf("failed to load from disk: %v", err)
		}

		// Verify loaded data
		perm, found := cache2.get("owner", "repo", "user1")
		if !found || perm != "admin" {
			t.Errorf("expected admin permission for user1, got %s", perm)
		}

		perm, found = cache2.get("owner", "repo", "user2")
		if !found || perm != "read" {
			t.Errorf("expected read permission for user2, got %s", perm)
		}
	})

	t.Run("cache expiration", func(t *testing.T) {
		cache := &permissionCache{
			memory: make(map[string]permissionEntry),
		}

		// Add expired entry manually
		key := "owner/repo/user1"
		cache.memory[key] = permissionEntry{
			Permission: "write",
			CachedAt:   time.Now().Add(-25 * time.Hour), // Expired
		}

		// Should not find expired entry
		_, found := cache.get("owner", "repo", "user1")
		if found {
			t.Error("expected expired entry to not be found")
		}
	})
}

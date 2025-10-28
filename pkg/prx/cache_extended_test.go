package prx

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewCacheClient(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name    string
		dir     string
		wantErr bool
	}{
		{
			name:    "valid absolute path",
			dir:     filepath.Join(tmpDir, "cache"),
			wantErr: false,
		},
		{
			name:    "relative path should error",
			dir:     "relative/path",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewCacheClient("test-token", tt.dir)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if client == nil {
					t.Errorf("Expected client but got nil")
				}
				if client.cacheDir != tt.dir {
					t.Errorf("Expected cache dir %s, got %s", tt.dir, client.cacheDir)
				}
			}
		})
	}
}

func TestCacheClient_CacheKey(t *testing.T) {
	tmpDir := t.TempDir()
	client, err := NewCacheClient("test-token", tmpDir)
	if err != nil {
		t.Fatalf("Failed to create cache client: %v", err)
	}

	tests := []struct {
		name  string
		parts []string
	}{
		{
			name:  "simple key",
			parts: []string{"pr", "owner", "repo", "1"},
		},
		{
			name:  "different key",
			parts: []string{"pr", "owner", "repo", "2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key1 := client.cacheKey(tt.parts...)
			key2 := client.cacheKey(tt.parts...)

			// Same inputs should produce same key
			if key1 != key2 {
				t.Errorf("Same inputs produced different keys")
			}

			// Key should be a hex string (sha256)
			if len(key1) != 64 {
				t.Errorf("Expected 64 character hex string, got %d characters", len(key1))
			}
		})
	}

	// Different inputs should produce different keys
	key1 := client.cacheKey("pr", "owner", "repo", "1")
	key2 := client.cacheKey("pr", "owner", "repo", "2")
	if key1 == key2 {
		t.Errorf("Different inputs produced same key")
	}
}

func TestCacheClient_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	client, err := NewCacheClient("test-token", tmpDir)
	if err != nil {
		t.Fatalf("Failed to create cache client: %v", err)
	}

	ctx := context.Background()
	key := "test-key"

	entry := cacheEntry{
		UpdatedAt: time.Now().Add(-1 * time.Hour),
		CachedAt:  time.Now(),
		Data:      json.RawMessage(`{"test": "data", "number": 123}`),
	}

	// Save cache
	err = client.saveCache(ctx, key, entry)
	if err != nil {
		t.Fatalf("Failed to save cache: %v", err)
	}

	// Verify file was created
	cachePath := filepath.Join(tmpDir, key+".json")
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		t.Fatalf("Cache file was not created")
	}

	// Load cache
	var loaded cacheEntry
	if !client.loadCache(ctx, key, &loaded) {
		t.Fatalf("Failed to load cache")
	}

	// Verify data
	if !loaded.UpdatedAt.Equal(entry.UpdatedAt) {
		t.Errorf("UpdatedAt mismatch: expected %v, got %v", entry.UpdatedAt, loaded.UpdatedAt)
	}
	// Compare JSON content, not formatting
	var entryJSON, loadedJSON map[string]interface{}
	if err := json.Unmarshal(entry.Data, &entryJSON); err != nil {
		t.Fatalf("Failed to unmarshal entry data: %v", err)
	}
	if err := json.Unmarshal(loaded.Data, &loadedJSON); err != nil {
		t.Fatalf("Failed to unmarshal loaded data: %v", err)
	}
	if fmt.Sprintf("%v", entryJSON) != fmt.Sprintf("%v", loadedJSON) {
		t.Errorf("Data mismatch: expected %v, got %v", entryJSON, loadedJSON)
	}
}

func TestCacheClient_LoadNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	client, err := NewCacheClient("test-token", tmpDir)
	if err != nil {
		t.Fatalf("Failed to create cache client: %v", err)
	}

	ctx := context.Background()

	var entry cacheEntry
	if client.loadCache(ctx, "nonexistent", &entry) {
		t.Errorf("Expected load to fail for nonexistent key")
	}
}

func TestCacheClient_LoadInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	client, err := NewCacheClient("test-token", tmpDir)
	if err != nil {
		t.Fatalf("Failed to create cache client: %v", err)
	}

	// Write invalid JSON to cache file
	key := "invalid"
	cachePath := filepath.Join(tmpDir, key+".json")
	err = os.WriteFile(cachePath, []byte(`{invalid json}`), 0o600)
	if err != nil {
		t.Fatalf("Failed to write invalid cache file: %v", err)
	}

	ctx := context.Background()
	var entry cacheEntry
	if client.loadCache(ctx, key, &entry) {
		t.Errorf("Expected load to fail for invalid JSON")
	}
}

func TestCacheClient_CleanOldCaches(t *testing.T) {
	tmpDir := t.TempDir()
	client, err := NewCacheClient("test-token", tmpDir)
	if err != nil {
		t.Fatalf("Failed to create cache client: %v", err)
	}

	// Create some old cache files
	oldFile := filepath.Join(tmpDir, "old.json")
	err = os.WriteFile(oldFile, []byte(`{"test": "old"}`), 0o600)
	if err != nil {
		t.Fatalf("Failed to create old file: %v", err)
	}

	// Set the modification time to be very old
	oldTime := time.Now().Add(-30 * 24 * time.Hour)
	err = os.Chtimes(oldFile, oldTime, oldTime)
	if err != nil {
		t.Fatalf("Failed to set old file time: %v", err)
	}

	// Create a recent cache file
	recentFile := filepath.Join(tmpDir, "recent.json")
	err = os.WriteFile(recentFile, []byte(`{"test": "recent"}`), 0o600)
	if err != nil {
		t.Fatalf("Failed to create recent file: %v", err)
	}

	// Clean old caches
	client.cleanOldCaches()

	// Old file should be removed
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Errorf("Old file was not removed")
	}

	// Recent file should still exist
	if _, err := os.Stat(recentFile); os.IsNotExist(err) {
		t.Errorf("Recent file was removed")
	}
}

func TestCacheClient_CleanOldCaches_SkipsDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	client, err := NewCacheClient("test-token", tmpDir)
	if err != nil {
		t.Fatalf("Failed to create cache client: %v", err)
	}

	// Create a subdirectory
	subDir := filepath.Join(tmpDir, "subdir")
	err = os.Mkdir(subDir, 0o700)
	if err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

	// Clean should not error on directories
	client.cleanOldCaches()

	// Directory should still exist
	if _, err := os.Stat(subDir); os.IsNotExist(err) {
		t.Errorf("Subdirectory was removed")
	}
}

func TestCacheClient_CleanOldCaches_SkipsNonJSON(t *testing.T) {
	tmpDir := t.TempDir()
	client, err := NewCacheClient("test-token", tmpDir)
	if err != nil {
		t.Fatalf("Failed to create cache client: %v", err)
	}

	// Create a non-JSON file
	nonJSONFile := filepath.Join(tmpDir, "test.txt")
	err = os.WriteFile(nonJSONFile, []byte("test"), 0o600)
	if err != nil {
		t.Fatalf("Failed to create non-JSON file: %v", err)
	}

	// Set the modification time to be very old
	oldTime := time.Now().Add(-30 * 24 * time.Hour)
	err = os.Chtimes(nonJSONFile, oldTime, oldTime)
	if err != nil {
		t.Fatalf("Failed to set old file time: %v", err)
	}

	// Clean should skip non-JSON files
	client.cleanOldCaches()

	// Non-JSON file should still exist
	if _, err := os.Stat(nonJSONFile); os.IsNotExist(err) {
		t.Errorf("Non-JSON file was removed")
	}
}

func TestCacheClient_SaveCache_AtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	client, err := NewCacheClient("test-token", tmpDir)
	if err != nil {
		t.Fatalf("Failed to create cache client: %v", err)
	}

	ctx := context.Background()
	key := "atomic-test"

	entry := cacheEntry{
		CachedAt: time.Now(),
		Data:     json.RawMessage(`{"test": "data"}`),
	}

	// Save cache
	err = client.saveCache(ctx, key, entry)
	if err != nil {
		t.Fatalf("Failed to save cache: %v", err)
	}

	// Verify no .tmp file exists (atomic write should have cleaned up)
	tmpPath := filepath.Join(tmpDir, key+".json.tmp")
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Errorf("Temporary file was not cleaned up")
	}

	// Verify final file exists
	cachePath := filepath.Join(tmpDir, key+".json")
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		t.Errorf("Cache file was not created")
	}
}

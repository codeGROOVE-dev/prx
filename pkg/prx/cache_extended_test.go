package prx

import (
	"path/filepath"
	"testing"
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
				defer func() {
					if closeErr := client.Close(); closeErr != nil {
						t.Errorf("Failed to close client: %v", closeErr)
					}
				}()
			}
		})
	}
}

func TestPRCacheKey(t *testing.T) {
	tests := []struct {
		name     string
		owner    string
		repo     string
		prNumber int
	}{
		{
			name:     "simple key",
			owner:    "owner",
			repo:     "repo",
			prNumber: 1,
		},
		{
			name:     "different key",
			owner:    "owner",
			repo:     "repo",
			prNumber: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key1 := prCacheKey(tt.owner, tt.repo, tt.prNumber)
			key2 := prCacheKey(tt.owner, tt.repo, tt.prNumber)

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
	key1 := prCacheKey("owner", "repo", 1)
	key2 := prCacheKey("owner", "repo", 2)
	if key1 == key2 {
		t.Errorf("Different inputs produced same key")
	}
}

func TestCollaboratorsCacheKey(t *testing.T) {
	key1 := collaboratorsCacheKey("owner", "repo")
	key2 := collaboratorsCacheKey("owner", "repo")

	if key1 != key2 {
		t.Errorf("Same inputs produced different keys: %s vs %s", key1, key2)
	}

	key3 := collaboratorsCacheKey("other", "repo")
	if key1 == key3 {
		t.Errorf("Different inputs produced same key")
	}
}

func TestCacheClientClose(t *testing.T) {
	tmpDir := t.TempDir()
	client, err := NewCacheClient("test-token", tmpDir)
	if err != nil {
		t.Fatalf("Failed to create cache client: %v", err)
	}

	// Close should not error
	if err := client.Close(); err != nil {
		t.Errorf("Close returned error: %v", err)
	}

	// Closing again should be safe
	if err := client.Close(); err != nil {
		t.Errorf("Second close returned error: %v", err)
	}
}

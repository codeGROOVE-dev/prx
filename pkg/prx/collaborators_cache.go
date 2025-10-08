package prx

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	collaboratorsCacheDuration = 4 * time.Hour
	collaboratorsCacheFile     = "collaborators_cache.json"
)

// collaboratorsCache caches repository collaborators list both in memory and on disk.
type collaboratorsCache struct {
	memory   map[string]collaboratorsEntry
	diskPath string
	mu       sync.RWMutex
}

// collaboratorsEntry represents a cached collaborators list for a repository.
type collaboratorsEntry struct {
	CachedAt      time.Time         `json:"cached_at"`
	Collaborators map[string]string `json:"collaborators"` // username -> permission
}

// newCollaboratorsCache creates a new collaborators cache.
func newCollaboratorsCache(cacheDir string) *collaboratorsCache {
	cc := &collaboratorsCache{
		memory:   make(map[string]collaboratorsEntry),
		diskPath: filepath.Join(cacheDir, collaboratorsCacheFile),
	}

	// Load existing cache from disk
	if err := cc.loadFromDisk(); err != nil {
		// Log error but don't fail - cache can start fresh
		_ = err
	}

	return cc
}

// get retrieves a cached collaborators list if it exists and is not expired.
//
//nolint:unused // Called from graphql_complete.go
func (cc *collaboratorsCache) get(owner, repo string) (map[string]string, bool) {
	key := fmt.Sprintf("%s/%s", owner, repo)

	cc.mu.RLock()
	defer cc.mu.RUnlock()

	entry, exists := cc.memory[key]
	if !exists {
		return nil, false
	}

	// Check if cache entry is expired
	if time.Since(entry.CachedAt) > collaboratorsCacheDuration {
		return nil, false
	}

	return entry.Collaborators, true
}

// set stores a collaborators list in the cache.
//
//nolint:unused // Called from graphql_complete.go
func (cc *collaboratorsCache) set(owner, repo string, collaborators map[string]string) error {
	key := fmt.Sprintf("%s/%s", owner, repo)

	cc.mu.Lock()
	cc.memory[key] = collaboratorsEntry{
		Collaborators: collaborators,
		CachedAt:      time.Now(),
	}
	cc.mu.Unlock()

	// Save to disk after releasing the lock
	return cc.saveToDisk()
}

// loadFromDisk loads the cache from disk.
func (cc *collaboratorsCache) loadFromDisk() error {
	// Skip if no disk path is set (in-memory only mode)
	if cc.diskPath == "" {
		return nil
	}

	data, err := os.ReadFile(cc.diskPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist yet, that's ok
		}
		return fmt.Errorf("reading collaborators cache: %w", err)
	}

	var cache map[string]collaboratorsEntry
	if err := json.Unmarshal(data, &cache); err != nil {
		return fmt.Errorf("parsing collaborators cache: %w", err)
	}

	cc.mu.Lock()
	defer cc.mu.Unlock()

	// Only load non-expired entries
	for key, entry := range cache {
		if time.Since(entry.CachedAt) <= collaboratorsCacheDuration {
			cc.memory[key] = entry
		}
	}

	return nil
}

// saveToDisk saves the current cache to disk.
//
//nolint:unused // Called from set()
func (cc *collaboratorsCache) saveToDisk() error {
	// Skip if no disk path is set (in-memory only mode)
	if cc.diskPath == "" {
		return nil
	}

	// Create a copy to avoid holding the lock during I/O
	cc.mu.RLock()
	cacheCopy := make(map[string]collaboratorsEntry, len(cc.memory))
	for k, v := range cc.memory {
		cacheCopy[k] = v
	}
	cc.mu.RUnlock()

	data, err := json.Marshal(cacheCopy)
	if err != nil {
		return fmt.Errorf("marshaling collaborators cache: %w", err)
	}

	// Write to temp file first, then rename for atomicity
	tempFile := cc.diskPath + ".tmp"
	if err := os.WriteFile(tempFile, data, 0o600); err != nil {
		return fmt.Errorf("writing collaborators cache: %w", err)
	}

	if err := os.Rename(tempFile, cc.diskPath); err != nil {
		return fmt.Errorf("renaming collaborators cache: %w", err)
	}

	return nil
}

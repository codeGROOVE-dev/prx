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

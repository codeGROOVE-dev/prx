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
	permissionCacheDuration = 24 * time.Hour
	permissionCacheFile     = "permission_cache.json"
)

// permissionCache caches user repository permissions both in memory and on disk.
type permissionCache struct {
	mu       sync.RWMutex
	memory   map[string]permissionEntry
	diskPath string
}

// permissionEntry represents a cached permission.
type permissionEntry struct {
	Permission string    `json:"permission"`
	CachedAt   time.Time `json:"cached_at"`
}

// newPermissionCache creates a new permission cache.
func newPermissionCache(cacheDir string) (*permissionCache, error) {
	pc := &permissionCache{
		memory:   make(map[string]permissionEntry),
		diskPath: filepath.Join(cacheDir, permissionCacheFile),
	}

	// Load existing cache from disk
	if err := pc.loadFromDisk(); err != nil {
		// Log error but don't fail - cache can start fresh
		// This is expected on first run
		_ = err
	}

	return pc, nil
}

// get retrieves a cached permission if it exists and is not expired.
func (pc *permissionCache) get(owner, repo, username string) (string, bool) {
	key := fmt.Sprintf("%s/%s/%s", owner, repo, username)

	pc.mu.RLock()
	defer pc.mu.RUnlock()

	entry, exists := pc.memory[key]
	if !exists {
		return "", false
	}

	// Check if cache entry is expired
	if time.Since(entry.CachedAt) > permissionCacheDuration {
		return "", false
	}

	return entry.Permission, true
}

// set stores a permission in the cache.
func (pc *permissionCache) set(owner, repo, username, permission string) error {
	key := fmt.Sprintf("%s/%s/%s", owner, repo, username)

	pc.mu.Lock()
	pc.memory[key] = permissionEntry{
		Permission: permission,
		CachedAt:   time.Now(),
	}
	pc.mu.Unlock()

	// Save to disk after releasing the lock
	return pc.saveToDisk()
}

// loadFromDisk loads the cache from disk.
func (pc *permissionCache) loadFromDisk() error {
	// Skip if no disk path is set (in-memory only mode)
	if pc.diskPath == "" {
		return nil
	}

	data, err := os.ReadFile(pc.diskPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist yet, that's ok
		}
		return fmt.Errorf("reading permission cache: %w", err)
	}

	var cache map[string]permissionEntry
	if err := json.Unmarshal(data, &cache); err != nil {
		return fmt.Errorf("parsing permission cache: %w", err)
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()

	// Only load non-expired entries
	now := time.Now()
	for key, entry := range cache {
		if now.Sub(entry.CachedAt) <= permissionCacheDuration {
			pc.memory[key] = entry
		}
	}

	return nil
}

// saveToDisk saves the current cache to disk.
func (pc *permissionCache) saveToDisk() error {
	// Skip if no disk path is set (in-memory only mode)
	if pc.diskPath == "" {
		return nil
	}

	// Create a copy to avoid holding the lock during I/O
	pc.mu.RLock()
	cacheCopy := make(map[string]permissionEntry, len(pc.memory))
	for k, v := range pc.memory {
		cacheCopy[k] = v
	}
	pc.mu.RUnlock()

	data, err := json.Marshal(cacheCopy)
	if err != nil {
		return fmt.Errorf("marshaling permission cache: %w", err)
	}

	// Write to temp file first, then rename for atomicity
	tempFile := pc.diskPath + ".tmp"
	if err := os.WriteFile(tempFile, data, 0600); err != nil {
		return fmt.Errorf("writing permission cache: %w", err)
	}

	if err := os.Rename(tempFile, pc.diskPath); err != nil {
		return fmt.Errorf("renaming permission cache: %w", err)
	}

	return nil
}

// cleanup removes expired entries from memory and disk.
func (pc *permissionCache) cleanup() error {
	pc.mu.Lock()
	now := time.Now()
	for key, entry := range pc.memory {
		if now.Sub(entry.CachedAt) > permissionCacheDuration {
			delete(pc.memory, key)
		}
	}
	pc.mu.Unlock()

	// Save to disk after releasing the lock
	return pc.saveToDisk()
}

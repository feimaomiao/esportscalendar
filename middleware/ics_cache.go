package middleware

import (
	"container/list"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"
)

const (
	maxCacheSize = 256
	// Forces the cache to refresh each hour
	cacheRefreshTime = time.Hour
	cacheDir         = "/tmp/esportscalendar-ics-cache"
)

type cacheEntry struct {
	hash       string
	filePath   string
	lastUpdate time.Time
	element    *list.Element
}

type ICSCache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
	lruList *list.List
	logger  *zap.Logger
}

func NewICSCache(logger *zap.Logger) (*ICSCache, error) {
	// Create cache directory if it doesn't exist
	if err := os.MkdirAll(cacheDir, 0750); err != nil {
		return nil, err
	}

	return &ICSCache{
		entries: make(map[string]*cacheEntry),
		lruList: list.New(),
		logger:  logger,
		mu:      sync.RWMutex{},
	}, nil
}

// Get retrieves cached ICS content if valid (not expired).
func (c *ICSCache) Get(hash string) (string, bool) {
	c.mu.RLock()
	entry, exists := c.entries[hash]
	c.mu.RUnlock()

	if !exists {
		return "", false
	}

	// Check if cache is expired
	if time.Since(entry.lastUpdate) > cacheRefreshTime {
		c.logger.Debug("Cache expired", zap.String("hash", hash))
		return "", false
	}

	// Read from file
	content, err := os.ReadFile(entry.filePath)
	if err != nil {
		c.logger.Error("Failed to read cached file", zap.Error(err), zap.String("hash", hash))
		return "", false
	}

	// Move to front of LRU list
	c.mu.Lock()
	c.lruList.MoveToFront(entry.element)
	c.mu.Unlock()

	c.logger.Debug("Cache hit", zap.String("hash", hash))
	return string(content), true
}

// Set stores ICS content in cache.
func (c *ICSCache) Set(hash string, content string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if entry already exists
	if entry, exists := c.entries[hash]; exists {
		// Update existing entry
		if err := os.WriteFile(entry.filePath, []byte(content), 0600); err != nil {
			c.logger.Error("Failed to write cache file", zap.Error(err), zap.String("hash", hash))
			return err
		}
		entry.lastUpdate = time.Now()
		c.lruList.MoveToFront(entry.element)
		c.logger.Debug("Cache updated", zap.String("hash", hash))
		return nil
	}

	// Evict LRU entry if cache is full
	if c.lruList.Len() >= maxCacheSize {
		c.evictLRU()
	}

	// Create new cache file
	filePath := filepath.Join(cacheDir, hash+".ics")
	if err := os.WriteFile(filePath, []byte(content), 0600); err != nil {
		c.logger.Error("Failed to write cache file", zap.Error(err), zap.String("hash", hash))
		return err
	}

	// Add to cache
	element := c.lruList.PushFront(hash)
	c.entries[hash] = &cacheEntry{
		hash:       hash,
		filePath:   filePath,
		lastUpdate: time.Now(),
		element:    element,
	}

	c.logger.Debug("Cache entry created", zap.String("hash", hash), zap.Int("cache_size", len(c.entries)))
	return nil
}

// evictLRU removes the least recently used entry.
// Must be called with lock held.
func (c *ICSCache) evictLRU() {
	element := c.lruList.Back()
	if element == nil {
		return
	}

	hash, success := element.Value.(string)
	if !success {
		c.logger.Warn("Failed to get hash from LRU element")
		return
	}
	entry := c.entries[hash]

	// Remove file
	if err := os.Remove(entry.filePath); err != nil {
		c.logger.Warn("Failed to remove cache file", zap.Error(err), zap.String("hash", hash))
	}

	// Remove from map and list.
	delete(c.entries, hash)
	c.lruList.Remove(element)

	c.logger.Debug("Cache entry evicted", zap.String("hash", hash))
}

// Clear removes all cache entries.
func (c *ICSCache) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for hash, entry := range c.entries {
		if err := os.Remove(entry.filePath); err != nil {
			c.logger.Warn("Failed to remove cache file", zap.Error(err), zap.String("hash", hash))
		}
	}

	c.entries = make(map[string]*cacheEntry)
	c.lruList = list.New()

	c.logger.Info("Cache cleared")
	return nil
}

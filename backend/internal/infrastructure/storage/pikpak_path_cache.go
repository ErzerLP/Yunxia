package storage

import (
	"strings"
	"sync"
	"time"
)

// PikPakPathCache 保存 source 内路径到 provider 文件元数据的短期映射。
type PikPakPathCache struct {
	mu      sync.RWMutex
	now     func() time.Time
	entries map[pikPakPathCacheKey]pikPakPathCacheEntry
}

type pikPakPathCacheKey struct {
	sourceID uint
	rootID   string
	path     string
}

type pikPakPathCacheEntry struct {
	file      PikPakFile
	expiresAt time.Time
}

// NewPikPakPathCache 创建路径缓存。
func NewPikPakPathCache() *PikPakPathCache {
	return &PikPakPathCache{
		now:     time.Now,
		entries: make(map[pikPakPathCacheKey]pikPakPathCacheEntry),
	}
}

func (c *PikPakPathCache) get(sourceID uint, rootID string, virtualPath string) (PikPakFile, bool) {
	if c == nil {
		return PikPakFile{}, false
	}
	key := pikPakPathCacheKey{sourceID: sourceID, rootID: strings.TrimSpace(rootID), path: normalizeCachePath(virtualPath)}
	now := c.timeNow()

	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		return PikPakFile{}, false
	}
	if !entry.expiresAt.IsZero() && now.After(entry.expiresAt) {
		c.mu.Lock()
		if current, exists := c.entries[key]; exists && current.expiresAt.Equal(entry.expiresAt) {
			delete(c.entries, key)
		}
		c.mu.Unlock()
		return PikPakFile{}, false
	}
	return entry.file, true
}

func (c *PikPakPathCache) set(sourceID uint, rootID string, virtualPath string, file PikPakFile, ttl time.Duration) {
	if c == nil || ttl <= 0 {
		return
	}
	key := pikPakPathCacheKey{sourceID: sourceID, rootID: strings.TrimSpace(rootID), path: normalizeCachePath(virtualPath)}
	c.mu.Lock()
	if c.entries == nil {
		c.entries = make(map[pikPakPathCacheKey]pikPakPathCacheEntry)
	}
	c.entries[key] = pikPakPathCacheEntry{
		file:      file,
		expiresAt: c.timeNow().Add(ttl),
	}
	c.mu.Unlock()
}

func (c *PikPakPathCache) clearSource(sourceID uint, rootID string) {
	if c == nil {
		return
	}
	rootID = strings.TrimSpace(rootID)
	c.mu.Lock()
	for key := range c.entries {
		if key.sourceID == sourceID && key.rootID == rootID {
			delete(c.entries, key)
		}
	}
	c.mu.Unlock()
}

func (c *PikPakPathCache) timeNow() time.Time {
	if c == nil || c.now == nil {
		return time.Now()
	}
	return c.now()
}

func normalizeCachePath(virtualPath string) string {
	virtualPath = strings.TrimSpace(virtualPath)
	if virtualPath == "" || virtualPath == "." {
		return "/"
	}
	if !strings.HasPrefix(virtualPath, "/") {
		virtualPath = "/" + virtualPath
	}
	for strings.Contains(virtualPath, "//") {
		virtualPath = strings.ReplaceAll(virtualPath, "//", "/")
	}
	if len(virtualPath) > 1 {
		virtualPath = strings.TrimRight(virtualPath, "/")
	}
	return virtualPath
}

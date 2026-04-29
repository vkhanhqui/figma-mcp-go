package internal

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// imagecachePersistFilename is the file inside the cache directory where
// content-hash → imageHash entries are written. The path is anchored to
// ~/.figma-mcp-go/cache/ to keep server-side state out of the repo and
// away from XDG cache eviction policies that might purge it aggressively.
const imagecachePersistFilename = "imagehash.json"

var imagecacheLogger = log.New(os.Stderr, "[imagecache] ", 0)

// PersistentImageCache wraps ImageCache with disk-backed persistence.
//
// On startup, Load() rehydrates entries from a JSON file. On every Put(),
// the in-memory map is updated immediately and a debounced flush is
// scheduled (default 2s). The flush writes to a temp file then atomically
// renames into place so a crash mid-write can never leave a corrupted JSON.
//
// Reasoning for keying by raw contentHash (not figmaFileKey+contentHash):
// Figma's imageHash is itself content-derived (MD5 of the raw bytes), so
// the mapping is invariant across files. The plugin still verifies via
// figma.getImageByHash before reusing — if the hash is gone in the current
// file, the bridge evicts and retries with bytes (see bridge.go Forget).
type PersistentImageCache struct {
	*ImageCache

	path        string
	flushDelay  time.Duration
	flushTimer  *time.Timer
	flushPend   bool
	flushMu     sync.Mutex
	closed      bool
	stopFlushCh chan struct{}
}

// DefaultImageCachePath returns the path to the persistent cache file under
// the user's home directory. Returns "" if the home dir cannot be resolved
// (rare; we degrade to in-memory only in that case).
func DefaultImageCachePath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".figma-mcp-go", "cache", imagecachePersistFilename)
}

// NewPersistentImageCache returns a cache that flushes to `path` (if non-empty).
// If path is empty, the cache behaves like an in-memory ImageCache.
func NewPersistentImageCache(path string) *PersistentImageCache {
	return &PersistentImageCache{
		ImageCache:  NewImageCache(),
		path:        path,
		flushDelay:  2 * time.Second,
		stopFlushCh: make(chan struct{}),
	}
}

// Load rehydrates the cache from disk. Returns nil if the file does not
// exist (first run); returns an error only on parse / read failure so the
// caller can decide whether to abort startup or proceed with empty cache.
func (p *PersistentImageCache) Load() error {
	if p == nil || p.path == "" {
		return nil
	}
	data, err := os.ReadFile(p.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read %s: %w", p.path, err)
	}
	if len(data) == 0 {
		return nil
	}
	var loaded map[string]string
	if err := json.Unmarshal(data, &loaded); err != nil {
		return fmt.Errorf("parse %s: %w", p.path, err)
	}
	p.ImageCache.mu.Lock()
	for k, v := range loaded {
		if k == "" || v == "" {
			continue
		}
		p.ImageCache.m[k] = v
	}
	count := len(p.ImageCache.m)
	p.ImageCache.mu.Unlock()
	imagecacheLogger.Printf("loaded %d entries from %s", count, p.path)
	return nil
}

// Put updates the in-memory cache and schedules a debounced flush.
func (p *PersistentImageCache) Put(contentHash, imageHash string) {
	if p == nil {
		return
	}
	p.ImageCache.Put(contentHash, imageHash)
	p.scheduleFlush()
}

// Forget evicts an entry and schedules a flush so the on-disk file stays in
// sync. Only worth flushing if the entry actually existed.
func (p *PersistentImageCache) Forget(contentHash string) {
	if p == nil {
		return
	}
	p.ImageCache.mu.Lock()
	_, existed := p.ImageCache.m[contentHash]
	delete(p.ImageCache.m, contentHash)
	p.ImageCache.mu.Unlock()
	if existed {
		p.scheduleFlush()
	}
}

// scheduleFlush coalesces rapid writes into a single disk flush after
// flushDelay. Multiple Put() calls within the window queue a single write.
func (p *PersistentImageCache) scheduleFlush() {
	if p == nil || p.path == "" {
		return
	}
	p.flushMu.Lock()
	defer p.flushMu.Unlock()
	if p.closed {
		return
	}
	if p.flushTimer != nil {
		p.flushTimer.Stop()
	}
	p.flushTimer = time.AfterFunc(p.flushDelay, func() {
		if err := p.Flush(); err != nil {
			imagecacheLogger.Printf("flush error: %v", err)
		}
	})
	p.flushPend = true
}

// Flush writes the cache to disk now. Atomic via temp file + rename so the
// destination is never partially written. Safe to call concurrently with
// scheduleFlush; the snapshot is taken under the read lock.
func (p *PersistentImageCache) Flush() error {
	if p == nil || p.path == "" {
		return nil
	}
	p.ImageCache.mu.RLock()
	snapshot := make(map[string]string, len(p.ImageCache.m))
	for k, v := range p.ImageCache.m {
		snapshot[k] = v
	}
	p.ImageCache.mu.RUnlock()

	if err := os.MkdirAll(filepath.Dir(p.path), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	data, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(p.path), ".imagehash-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op if rename succeeded

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpPath, p.path); err != nil {
		return fmt.Errorf("rename: %w", err)
	}

	p.flushMu.Lock()
	p.flushPend = false
	p.flushMu.Unlock()
	return nil
}

// Close stops the flush timer and writes any pending entries to disk
// synchronously. Idempotent.
func (p *PersistentImageCache) Close() error {
	if p == nil {
		return nil
	}
	p.flushMu.Lock()
	if p.closed {
		p.flushMu.Unlock()
		return nil
	}
	p.closed = true
	if p.flushTimer != nil {
		p.flushTimer.Stop()
	}
	pend := p.flushPend
	p.flushMu.Unlock()
	if pend {
		return p.Flush()
	}
	return nil
}

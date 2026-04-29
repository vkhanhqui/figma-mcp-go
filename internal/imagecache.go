package internal

import "sync"

// ImageCacheStore is the read/write surface the Bridge depends on. The
// in-memory ImageCache and the disk-backed PersistentImageCache both satisfy
// it, so callers can swap implementations without code changes.
type ImageCacheStore interface {
	Get(contentHash string) (string, bool)
	Put(contentHash, imageHash string)
	Forget(contentHash string)
	Len() int
}

// ImageCache maps the SHA-256 of raw image bytes to the Figma imageHash that
// figma.createImage assigned the first time those bytes were imported. The
// cache is per-Bridge (i.e. per-leader, per-plugin-connection) and does not
// persist across reconnects — Figma's image hash is a content-derived MD5
// internal to the file, but reusing it inside one session avoids re-uploading
// the bytes over WebSocket on every `import_image` call.
//
// On cache hit, the bridge replaces `imageData` with `imageHash` in the
// outgoing params; the plugin then attaches an existing fill without decoding
// or calling figma.createImage again.
type ImageCache struct {
	mu sync.RWMutex
	m  map[string]string
}

// NewImageCache returns an empty cache.
func NewImageCache() *ImageCache {
	return &ImageCache{m: make(map[string]string)}
}

// Get returns the cached imageHash for a content hash. The bool reports hit/miss.
func (c *ImageCache) Get(contentHash string) (string, bool) {
	if c == nil || contentHash == "" {
		return "", false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.m[contentHash]
	return v, ok
}

// Put stores the imageHash for a content hash. No-op if either key is empty.
func (c *ImageCache) Put(contentHash, imageHash string) {
	if c == nil || contentHash == "" || imageHash == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[contentHash] = imageHash
}

// Forget evicts a content hash. Called when the plugin reports an imageHash
// that no longer exists in the file (e.g. user deleted all references).
func (c *ImageCache) Forget(contentHash string) {
	if c == nil || contentHash == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, contentHash)
}

// Len returns the current number of cached entries — used for metrics.
func (c *ImageCache) Len() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.m)
}

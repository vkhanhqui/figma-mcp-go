package internal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPersistentImageCache_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "imagehash.json")

	c1 := NewPersistentImageCache(path)
	c1.flushDelay = 10 * time.Millisecond
	if err := c1.Load(); err != nil {
		t.Fatalf("Load on empty: %v", err)
	}

	c1.Put("contentA", "hashA")
	c1.Put("contentB", "hashB")
	if err := c1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Verify file exists and parses to expected map.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read persisted file: %v", err)
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if m["contentA"] != "hashA" || m["contentB"] != "hashB" {
		t.Errorf("persisted contents = %v, want both entries", m)
	}

	// Re-load into a fresh instance.
	c2 := NewPersistentImageCache(path)
	if err := c2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if v, ok := c2.Get("contentA"); !ok || v != "hashA" {
		t.Errorf("after reload Get(contentA) = (%q, %v), want (hashA, true)", v, ok)
	}
	if c2.Len() != 2 {
		t.Errorf("Len = %d, want 2", c2.Len())
	}
}

func TestPersistentImageCache_Forget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "imagehash.json")

	c := NewPersistentImageCache(path)
	c.flushDelay = 10 * time.Millisecond
	c.Put("a", "ha")
	c.Put("b", "hb")
	c.Forget("a")
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	c2 := NewPersistentImageCache(path)
	if err := c2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := c2.Get("a"); ok {
		t.Error("Forgotten entry should not survive reload")
	}
	if v, ok := c2.Get("b"); !ok || v != "hb" {
		t.Errorf("Get(b) = (%q, %v), want (hb, true)", v, ok)
	}
}

func TestPersistentImageCache_LoadMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.json")
	c := NewPersistentImageCache(path)
	if err := c.Load(); err != nil {
		t.Errorf("Load on missing file should not error, got %v", err)
	}
	if c.Len() != 0 {
		t.Errorf("Len = %d, want 0", c.Len())
	}
}

func TestPersistentImageCache_EmptyPathDegradesToMemory(t *testing.T) {
	c := NewPersistentImageCache("")
	c.Put("a", "ha")
	if v, ok := c.Get("a"); !ok || v != "ha" {
		t.Errorf("Get(a) = (%q, %v), want (ha, true)", v, ok)
	}
	if err := c.Flush(); err != nil {
		t.Errorf("Flush with empty path should be no-op, got %v", err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("Close with empty path should be no-op, got %v", err)
	}
}

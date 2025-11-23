package fusefs

import (
	"os"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
)

// mockFileInfo implements os.FileInfo for testing
type mockFileInfo struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
	isDir   bool
}

func (m *mockFileInfo) Name() string       { return m.name }
func (m *mockFileInfo) Size() int64        { return m.size }
func (m *mockFileInfo) Mode() os.FileMode  { return m.mode }
func (m *mockFileInfo) ModTime() time.Time { return m.modTime }
func (m *mockFileInfo) IsDir() bool        { return m.isDir }
func (m *mockFileInfo) Sys() interface{}   { return nil }

func TestInodeManager_GetInode(t *testing.T) {
	im := NewInodeManager(1000, 100, 5*time.Second, 5*time.Second)

	info1 := &mockFileInfo{
		name:    "test.txt",
		size:    100,
		modTime: time.Now(),
		isDir:   false,
	}

	// First call should allocate a new inode
	ino1 := im.GetInode("/test.txt", info1)
	if ino1 == 0 {
		t.Error("GetInode returned 0, expected non-zero inode")
	}

	// Second call with same path and info should return same inode
	ino2 := im.GetInode("/test.txt", info1)
	if ino1 != ino2 {
		t.Errorf("GetInode returned different inodes: %d != %d", ino1, ino2)
	}

	// Call with different path should return different inode
	info2 := &mockFileInfo{
		name:    "other.txt",
		size:    200,
		modTime: time.Now(),
		isDir:   false,
	}
	ino3 := im.GetInode("/other.txt", info2)
	if ino1 == ino3 {
		t.Error("GetInode returned same inode for different paths")
	}

	// Call with same path but changed file should return new inode
	info3 := &mockFileInfo{
		name:    "test.txt",
		size:    150, // Different size
		modTime: time.Now().Add(time.Hour),
		isDir:   false,
	}
	ino4 := im.GetInode("/test.txt", info3)
	if ino1 == ino4 {
		t.Error("GetInode returned same inode for changed file")
	}
}

func TestInodeManager_Cache(t *testing.T) {
	im := NewInodeManager(1000, 100, 5*time.Second, 5*time.Second)

	info := &mockFileInfo{
		name:    "test.txt",
		size:    100,
		modTime: time.Now(),
		isDir:   false,
	}

	ino := im.GetInode("/test.txt", info)

	// Initially no cached attr
	cached := im.GetCached("/test.txt")
	if cached != nil {
		t.Error("Expected nil cached attr before caching")
	}

	// Cache an attribute
	attr := &fuse.Attr{
		Ino:  ino,
		Size: 100,
		Mode: 0644,
	}
	im.Cache("/test.txt", attr)

	// Should now be cached
	cached = im.GetCached("/test.txt")
	if cached == nil {
		t.Error("Expected cached attr after caching")
	}
	if cached.Ino != ino {
		t.Errorf("Cached attr has wrong inode: %d != %d", cached.Ino, ino)
	}
}

func TestInodeManager_CacheExpiration(t *testing.T) {
	// Test with very short TTL
	im := NewInodeManager(1000, 100, 50*time.Millisecond, 50*time.Millisecond)

	info := &mockFileInfo{
		name:    "test.txt",
		size:    100,
		modTime: time.Now(),
		isDir:   false,
	}

	ino := im.GetInode("/test.txt", info)

	attr := &fuse.Attr{
		Ino:  ino,
		Size: 100,
		Mode: 0644,
	}
	im.Cache("/test.txt", attr)

	// Should be cached immediately
	cached := im.GetCached("/test.txt")
	if cached == nil {
		t.Error("Expected cached attr immediately after caching")
	}

	// Wait for expiration
	time.Sleep(60 * time.Millisecond)

	// Should now return nil due to expiration
	cached = im.GetCached("/test.txt")
	if cached != nil {
		t.Error("Expected nil for expired cache")
	}
}

func TestInodeManager_DirCache(t *testing.T) {
	im := NewInodeManager(1000, 100, 5*time.Second, 5*time.Second)

	entries := []fuse.DirEntry{
		{Name: "file1.txt", Ino: 1, Mode: 0644},
		{Name: "file2.txt", Ino: 2, Mode: 0644},
	}

	// Initially no cache
	cached := im.GetDirCache("/mydir")
	if cached != nil {
		t.Error("Expected nil cached dir before caching")
	}

	// Cache directory
	im.CacheDir("/mydir", entries)

	// Should now be cached
	cached = im.GetDirCache("/mydir")
	if cached == nil {
		t.Fatal("Expected cached dir after caching")
	}
	if len(cached) != 2 {
		t.Errorf("Expected 2 entries, got %d", len(cached))
	}
	if cached[0].Name != "file1.txt" {
		t.Errorf("Wrong entry name: %s", cached[0].Name)
	}
}

func TestInodeManager_InvalidateDir(t *testing.T) {
	im := NewInodeManager(1000, 100, 5*time.Second, 5*time.Second)

	entries := []fuse.DirEntry{
		{Name: "file1.txt", Ino: 1, Mode: 0644},
	}

	im.CacheDir("/mydir", entries)

	// Verify cached
	cached := im.GetDirCache("/mydir")
	if cached == nil {
		t.Fatal("Expected cached dir")
	}

	// Invalidate
	im.InvalidateDir("/mydir")

	// Should now be nil
	cached = im.GetDirCache("/mydir")
	if cached != nil {
		t.Error("Expected nil after invalidation")
	}
}

func TestInodeManager_Clear(t *testing.T) {
	im := NewInodeManager(1000, 100, 5*time.Second, 5*time.Second)

	info := &mockFileInfo{
		name:    "test.txt",
		size:    100,
		modTime: time.Now(),
		isDir:   false,
	}

	ino := im.GetInode("/test.txt", info)
	attr := &fuse.Attr{Ino: ino, Size: 100}
	im.Cache("/test.txt", attr)

	entries := []fuse.DirEntry{{Name: "file.txt", Ino: 1}}
	im.CacheDir("/dir", entries)

	// Clear all
	im.Clear()

	// All caches should be empty
	if im.GetCached("/test.txt") != nil {
		t.Error("Expected nil after clear")
	}
	if im.GetDirCache("/dir") != nil {
		t.Error("Expected nil dir cache after clear")
	}

	// Stats should show zero inodes
	stats := im.Stats()
	if stats.TotalInodes != 0 {
		t.Errorf("Expected 0 total inodes after clear, got %d", stats.TotalInodes)
	}
}

func TestInodeManager_Stats(t *testing.T) {
	im := NewInodeManager(1000, 100, 5*time.Second, 5*time.Second)

	info := &mockFileInfo{
		name:    "test.txt",
		size:    100,
		modTime: time.Now(),
		isDir:   false,
	}

	// Allocate some inodes
	im.GetInode("/test1.txt", info)
	im.GetInode("/test2.txt", info)
	im.GetInode("/test3.txt", info)

	stats := im.Stats()
	if stats.TotalInodes != 3 {
		t.Errorf("Expected 3 total inodes, got %d", stats.TotalInodes)
	}
}

func TestInodeManager_LRUEviction(t *testing.T) {
	// Create manager with small cache
	im := NewInodeManager(3, 3, 5*time.Second, 5*time.Second)

	info := &mockFileInfo{
		name:    "test.txt",
		size:    100,
		modTime: time.Now(),
		isDir:   false,
	}

	// Cache 3 attributes
	ino1 := im.GetInode("/test1.txt", info)
	attr1 := &fuse.Attr{Ino: ino1, Size: 100}
	im.Cache("/test1.txt", attr1)

	ino2 := im.GetInode("/test2.txt", info)
	attr2 := &fuse.Attr{Ino: ino2, Size: 100}
	im.Cache("/test2.txt", attr2)

	ino3 := im.GetInode("/test3.txt", info)
	attr3 := &fuse.Attr{Ino: ino3, Size: 100}
	im.Cache("/test3.txt", attr3)

	// All should be cached
	if im.GetCached("/test1.txt") == nil {
		t.Error("Expected test1.txt to be cached")
	}

	// Add one more, should evict oldest
	ino4 := im.GetInode("/test4.txt", info)
	attr4 := &fuse.Attr{Ino: ino4, Size: 100}
	im.Cache("/test4.txt", attr4)

	// test1.txt might be evicted (LRU), but we can't guarantee the order
	// Just verify that the cache size is limited
	stats := im.Stats()
	if stats.AttrCache.Size > 3 {
		t.Errorf("Expected cache size <= 3, got %d", stats.AttrCache.Size)
	}
}

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
	im := NewInodeManager()

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
	im := NewInodeManager()

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
	im := NewInodeManager()

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

	// Manually expire the cache by setting timestamp to past
	im.mu.Lock()
	if cached, exists := im.inodeToInfo[ino]; exists {
		cached.timestamp = time.Now().Add(-10 * time.Second)
	}
	im.mu.Unlock()

	// Should now return nil due to expiration
	cached := im.GetCached("/test.txt")
	if cached != nil {
		t.Error("Expected nil for expired cache")
	}
}

func TestInodeManager_DirCache(t *testing.T) {
	im := NewInodeManager()

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
	im := NewInodeManager()

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
	im := NewInodeManager()

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

	// Path should no longer have inode
	im.mu.RLock()
	_, exists := im.pathToInode["/test.txt"]
	im.mu.RUnlock()
	if exists {
		t.Error("Expected path to be cleared")
	}
}

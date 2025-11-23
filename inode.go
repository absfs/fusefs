package fusefs

import (
	"os"
	"sync"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
)

// InodeManager manages the mapping between filesystem paths and inode numbers.
//
// It provides:
//   - Stable inode allocation for paths
//   - LRU cache for inode attributes with configurable size and TTL
//   - LRU cache for directory listings with configurable size and TTL
//   - Detection of file changes (via mtime and size)
//   - Sharded locks for improved concurrency
//
// All methods are thread-safe and can be called concurrently.
type InodeManager struct {
	// Path to inode mapping (stable, not evicted)
	pathMu      sync.RWMutex
	pathToInode map[string]uint64
	inodeToPath map[uint64]string
	nextInode   uint64

	// Attribute cache with LRU eviction
	attrCache *lruCache
	attrTTL   time.Duration

	// Directory listing cache with LRU eviction
	dirCache *lruCache
	dirTTL   time.Duration

	// File metadata for change detection (stable, not evicted)
	metaMu       sync.RWMutex
	inodeToMeta  map[uint64]*inodeMeta
}

// inodeMeta stores metadata for change detection
type inodeMeta struct {
	modTime time.Time
	size    int64
}

// cachedAttr stores cached file attributes
type cachedAttr struct {
	attr      *fuse.Attr
	timestamp time.Time
}

// dirCacheEntry stores cached directory listings
type dirCacheEntry struct {
	entries   []fuse.DirEntry
	timestamp time.Time
}

// NewInodeManager creates a new inode manager with the specified cache configuration.
func NewInodeManager(attrCacheSize, dirCacheSize int, attrTTL, dirTTL time.Duration) *InodeManager {
	return &InodeManager{
		pathToInode: make(map[string]uint64),
		inodeToPath: make(map[uint64]string),
		nextInode:   1, // Start at 1, reserve 0
		attrCache:   newLRUCache(attrCacheSize, attrTTL),
		attrTTL:     attrTTL,
		dirCache:    newLRUCache(dirCacheSize, dirTTL),
		dirTTL:      dirTTL,
		inodeToMeta: make(map[uint64]*inodeMeta),
	}
}

// GetInode returns the inode number for a given path and FileInfo
func (im *InodeManager) GetInode(path string, info os.FileInfo) uint64 {
	im.pathMu.Lock()
	defer im.pathMu.Unlock()

	// Check if path already has inode
	if ino, exists := im.pathToInode[path]; exists {
		// Verify file hasn't changed (check mtime, size)
		if im.isSameFile(ino, info) {
			return ino
		}
		// File changed, allocate new inode
		delete(im.inodeToPath, ino)
		im.deleteMetaLocked(ino)
	}

	// Allocate new inode
	im.nextInode++
	ino := im.nextInode

	im.pathToInode[path] = ino
	im.inodeToPath[ino] = path
	im.storeMetaLocked(ino, info)

	return ino
}

// isSameFile checks if the cached inode represents the same file
func (im *InodeManager) isSameFile(ino uint64, info os.FileInfo) bool {
	im.metaMu.RLock()
	defer im.metaMu.RUnlock()

	meta, exists := im.inodeToMeta[ino]
	if !exists {
		return false
	}

	// Compare modification time and size
	return meta.modTime.Equal(info.ModTime()) && meta.size == info.Size()
}

// storeMetaLocked stores metadata (assumes pathMu is held)
func (im *InodeManager) storeMetaLocked(ino uint64, info os.FileInfo) {
	im.metaMu.Lock()
	defer im.metaMu.Unlock()

	im.inodeToMeta[ino] = &inodeMeta{
		modTime: info.ModTime(),
		size:    info.Size(),
	}
}

// deleteMetaLocked deletes metadata (assumes pathMu is held)
func (im *InodeManager) deleteMetaLocked(ino uint64) {
	im.metaMu.Lock()
	defer im.metaMu.Unlock()

	delete(im.inodeToMeta, ino)
}

// GetCached returns a cached attribute if available and not expired
func (im *InodeManager) GetCached(path string) *fuse.Attr {
	value, ok := im.attrCache.Get(path)
	if !ok {
		return nil
	}

	cached := value.(*cachedAttr)
	return cached.attr
}

// Cache stores an attribute in the cache
func (im *InodeManager) Cache(path string, attr *fuse.Attr) {
	cached := &cachedAttr{
		attr:      attr,
		timestamp: time.Now(),
	}
	im.attrCache.Put(path, cached)
}

// CacheDir stores a directory listing in the cache
func (im *InodeManager) CacheDir(path string, entries []fuse.DirEntry) {
	entry := &dirCacheEntry{
		entries:   entries,
		timestamp: time.Now(),
	}
	im.dirCache.Put(path, entry)
}

// GetDirCache returns a cached directory listing if available and not expired
func (im *InodeManager) GetDirCache(path string) []fuse.DirEntry {
	value, ok := im.dirCache.Get(path)
	if !ok {
		return nil
	}

	entry := value.(*dirCacheEntry)
	return entry.entries
}

// InvalidateDir removes a directory from the cache
func (im *InodeManager) InvalidateDir(path string) {
	im.dirCache.Delete(path)
}

// InvalidateAttr removes an attribute from the cache
func (im *InodeManager) InvalidateAttr(path string) {
	im.attrCache.Delete(path)
}

// Clear removes all cached data
func (im *InodeManager) Clear() {
	im.pathMu.Lock()
	defer im.pathMu.Unlock()

	im.pathToInode = make(map[string]uint64)
	im.inodeToPath = make(map[uint64]string)
	im.attrCache.Clear()
	im.dirCache.Clear()

	im.metaMu.Lock()
	im.inodeToMeta = make(map[uint64]*inodeMeta)
	im.metaMu.Unlock()
}

// Stats returns cache statistics
func (im *InodeManager) Stats() InodeManagerStats {
	im.pathMu.RLock()
	totalInodes := len(im.pathToInode)
	im.pathMu.RUnlock()

	return InodeManagerStats{
		TotalInodes: totalInodes,
		AttrCache:   im.attrCache.Stats(),
		DirCache:    im.dirCache.Stats(),
	}
}

// InodeManagerStats contains statistics about the inode manager
type InodeManagerStats struct {
	TotalInodes int        // Total number of allocated inodes
	AttrCache   CacheStats // Attribute cache statistics
	DirCache    CacheStats // Directory cache statistics
}

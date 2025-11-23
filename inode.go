package fusefs

import (
	"os"
	"sync"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
)

// InodeManager manages the mapping between filesystem paths and inode numbers
type InodeManager struct {
	mu          sync.RWMutex
	pathToInode map[string]uint64
	inodeToPath map[uint64]string
	inodeToInfo map[uint64]*cachedAttr
	nextInode   uint64
	dirCache    map[string]*dirCacheEntry
}

// cachedAttr stores cached file attributes
type cachedAttr struct {
	attr      *fuse.Attr
	timestamp time.Time
	modTime   time.Time
	size      int64
}

// dirCacheEntry stores cached directory listings
type dirCacheEntry struct {
	entries   []fuse.DirEntry
	timestamp time.Time
	ttl       time.Duration
}

// NewInodeManager creates a new inode manager
func NewInodeManager() *InodeManager {
	return &InodeManager{
		pathToInode: make(map[string]uint64),
		inodeToPath: make(map[uint64]string),
		inodeToInfo: make(map[uint64]*cachedAttr),
		nextInode:   1, // Start at 1, reserve 0
		dirCache:    make(map[string]*dirCacheEntry),
	}
}

// GetInode returns the inode number for a given path and FileInfo
func (im *InodeManager) GetInode(path string, info os.FileInfo) uint64 {
	im.mu.Lock()
	defer im.mu.Unlock()

	// Check if path already has inode
	if ino, exists := im.pathToInode[path]; exists {
		// Verify file hasn't changed (check mtime, size)
		if im.isSameFile(ino, info) {
			return ino
		}
		// File changed, allocate new inode
		delete(im.inodeToPath, ino)
		delete(im.inodeToInfo, ino)
	}

	// Allocate new inode
	im.nextInode++
	ino := im.nextInode

	im.pathToInode[path] = ino
	im.inodeToPath[ino] = path
	im.inodeToInfo[ino] = &cachedAttr{
		timestamp: time.Now(),
		modTime:   info.ModTime(),
		size:      info.Size(),
	}

	return ino
}

// isSameFile checks if the cached inode represents the same file
func (im *InodeManager) isSameFile(ino uint64, info os.FileInfo) bool {
	cached, exists := im.inodeToInfo[ino]
	if !exists {
		return false
	}

	// Compare modification time and size
	return cached.modTime.Equal(info.ModTime()) && cached.size == info.Size()
}

// GetCached returns a cached attribute if available and not expired
func (im *InodeManager) GetCached(path string) *fuse.Attr {
	im.mu.RLock()
	defer im.mu.RUnlock()

	ino, exists := im.pathToInode[path]
	if !exists {
		return nil
	}

	cached := im.inodeToInfo[ino]
	if cached == nil || cached.attr == nil {
		return nil
	}

	// Check if cache expired (5 seconds TTL)
	if time.Since(cached.timestamp) > 5*time.Second {
		return nil
	}

	return cached.attr
}

// Cache stores an attribute in the cache
func (im *InodeManager) Cache(path string, attr *fuse.Attr) {
	im.mu.Lock()
	defer im.mu.Unlock()

	ino := attr.Ino
	if cached, exists := im.inodeToInfo[ino]; exists {
		cached.attr = attr
		cached.timestamp = time.Now()
	}
}

// CacheDir stores a directory listing in the cache
func (im *InodeManager) CacheDir(path string, entries []fuse.DirEntry) {
	im.mu.Lock()
	defer im.mu.Unlock()

	im.dirCache[path] = &dirCacheEntry{
		entries:   entries,
		timestamp: time.Now(),
		ttl:       5 * time.Second, // Configurable TTL
	}
}

// GetDirCache returns a cached directory listing if available and not expired
func (im *InodeManager) GetDirCache(path string) []fuse.DirEntry {
	im.mu.RLock()
	defer im.mu.RUnlock()

	entry := im.dirCache[path]
	if entry == nil {
		return nil
	}

	// Check if cache expired
	if time.Since(entry.timestamp) > entry.ttl {
		return nil
	}

	return entry.entries
}

// InvalidateDir removes a directory from the cache
func (im *InodeManager) InvalidateDir(path string) {
	im.mu.Lock()
	defer im.mu.Unlock()

	delete(im.dirCache, path)
}

// Clear removes all cached data
func (im *InodeManager) Clear() {
	im.mu.Lock()
	defer im.mu.Unlock()

	im.pathToInode = make(map[string]uint64)
	im.inodeToPath = make(map[uint64]string)
	im.inodeToInfo = make(map[uint64]*cachedAttr)
	im.dirCache = make(map[string]*dirCacheEntry)
}

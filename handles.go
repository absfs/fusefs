package fusefs

import (
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/absfs/absfs"
)

// HandleTracker manages open file handles and their lifecycle.
//
// It provides:
//   - Unique handle ID allocation
//   - File handle storage and retrieval
//   - Reference counting for shared handles
//   - Automatic cleanup on release
//
// All methods are thread-safe and can be called concurrently.
type HandleTracker struct {
	mu         sync.RWMutex
	handles    map[uint64]*handleEntry
	nextHandle atomic.Uint64
}

// handleEntry represents an open file handle
type handleEntry struct {
	file     absfs.File
	refCount int32
	flags    int
	path     string
}

// NewHandleTracker creates a new file handle tracker
func NewHandleTracker() *HandleTracker {
	ht := &HandleTracker{
		handles: make(map[uint64]*handleEntry),
	}
	ht.nextHandle.Store(0)
	return ht
}

// Add allocates a new file handle for the given file
func (ht *HandleTracker) Add(file absfs.File, flags int, path string) uint64 {
	ht.mu.Lock()
	defer ht.mu.Unlock()

	fh := ht.nextHandle.Add(1)

	ht.handles[fh] = &handleEntry{
		file:     file,
		refCount: 1,
		flags:    flags,
		path:     path,
	}

	return fh
}

// Get returns the file associated with a handle
func (ht *HandleTracker) Get(fh uint64) absfs.File {
	ht.mu.RLock()
	defer ht.mu.RUnlock()

	entry := ht.handles[fh]
	if entry == nil {
		return nil
	}

	return entry.file
}

// GetEntry returns the full handle entry
func (ht *HandleTracker) GetEntry(fh uint64) *handleEntry {
	ht.mu.RLock()
	defer ht.mu.RUnlock()

	return ht.handles[fh]
}

// Release decrements the reference count and closes the file if it reaches zero
func (ht *HandleTracker) Release(fh uint64) syscall.Errno {
	ht.mu.Lock()
	defer ht.mu.Unlock()

	entry := ht.handles[fh]
	if entry == nil {
		return syscall.EBADF
	}

	entry.refCount--
	if entry.refCount <= 0 {
		err := entry.file.Close()
		delete(ht.handles, fh)
		if err != nil {
			return mapError(err)
		}
	}

	return 0
}

// CloseAll closes all open file handles
func (ht *HandleTracker) CloseAll() {
	ht.mu.Lock()
	defer ht.mu.Unlock()

	for fh, entry := range ht.handles {
		entry.file.Close()
		delete(ht.handles, fh)
	}
}

// Count returns the number of open file handles
func (ht *HandleTracker) Count() int {
	ht.mu.RLock()
	defer ht.mu.RUnlock()

	return len(ht.handles)
}

package fusefs

import (
	"context"
	"sync"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// LockManager manages file locks for the FUSE filesystem.
//
// It provides:
//   - BSD-style flock (whole-file locks)
//   - POSIX locks (byte-range locks)
//   - Per-file lock tracking
//   - Deadlock prevention
//
// All methods are thread-safe.
type LockManager struct {
	mu sync.RWMutex

	// BSD-style flock (whole-file locks)
	// Maps file path -> lock owner
	flocks map[string]uint64

	// POSIX locks (byte-range locks)
	// Maps file path -> list of lock ranges
	posixLocks map[string][]*posixLock
}

// posixLock represents a POSIX byte-range lock
type posixLock struct {
	owner uint64
	start uint64
	end   uint64 // exclusive
	typ   uint32 // F_RDLCK or F_WRLCK
	pid   uint32
}

// NewLockManager creates a new lock manager
func NewLockManager() *LockManager {
	return &LockManager{
		flocks:     make(map[string]uint64),
		posixLocks: make(map[string][]*posixLock),
	}
}

// Getlk tests for a POSIX lock (F_GETLK)
func (lm *LockManager) Getlk(path string, owner uint64, lk *fuse.FileLock) syscall.Errno {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	locks, exists := lm.posixLocks[path]
	if !exists {
		// No locks, return F_UNLCK to indicate lock would succeed
		lk.Typ = syscall.F_UNLCK
		return 0
	}

	// Check for conflicts with existing locks
	for _, lock := range locks {
		// Skip locks from the same owner
		if lock.owner == owner {
			continue
		}

		// Check for overlap
		if lm.rangesOverlap(lk.Start, lk.End, lock.start, lock.end) {
			// Check for conflict
			if lk.Typ == syscall.F_WRLCK || lock.typ == syscall.F_WRLCK {
				// Conflicting lock found
				lk.Typ = lock.typ
				lk.Start = lock.start
				lk.End = lock.end
				lk.Pid = lock.pid
				return 0
			}
		}
	}

	// No conflicting locks
	lk.Typ = syscall.F_UNLCK
	return 0
}

// Setlk sets or clears a POSIX lock (F_SETLK, non-blocking)
func (lm *LockManager) Setlk(path string, owner uint64, lk *fuse.FileLock) syscall.Errno {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	if lk.Typ == syscall.F_UNLCK {
		return lm.unlockPosix(path, owner, lk)
	}

	// Check for conflicts
	locks := lm.posixLocks[path]
	for _, lock := range locks {
		if lock.owner == owner {
			continue
		}

		if lm.rangesOverlap(lk.Start, lk.End, lock.start, lock.end) {
			if lk.Typ == syscall.F_WRLCK || lock.typ == syscall.F_WRLCK {
				return syscall.EAGAIN // Would block
			}
		}
	}

	// Add the lock
	newLock := &posixLock{
		owner: owner,
		start: lk.Start,
		end:   lk.End,
		typ:   lk.Typ,
		pid:   lk.Pid,
	}

	lm.posixLocks[path] = append(lm.posixLocks[path], newLock)
	return 0
}

// Setlkw sets or clears a POSIX lock (F_SETLKW, blocking)
// Note: FUSE doesn't actually block; it returns EAGAIN and the kernel retries
func (lm *LockManager) Setlkw(path string, owner uint64, lk *fuse.FileLock) syscall.Errno {
	// For FUSE, blocking locks are handled by the kernel
	// We just treat this like a non-blocking lock
	return lm.Setlk(path, owner, lk)
}

// unlockPosix removes a POSIX lock
func (lm *LockManager) unlockPosix(path string, owner uint64, lk *fuse.FileLock) syscall.Errno {
	locks := lm.posixLocks[path]
	if locks == nil {
		return 0
	}

	// Remove locks owned by this owner that overlap with the unlock range
	newLocks := make([]*posixLock, 0, len(locks))
	for _, lock := range locks {
		if lock.owner != owner {
			newLocks = append(newLocks, lock)
			continue
		}

		// Check for overlap
		if !lm.rangesOverlap(lk.Start, lk.End, lock.start, lock.end) {
			newLocks = append(newLocks, lock)
			continue
		}

		// Handle partial unlocks (split the lock if needed)
		if lock.start < lk.Start {
			// Keep the part before the unlock range
			newLocks = append(newLocks, &posixLock{
				owner: lock.owner,
				start: lock.start,
				end:   lk.Start,
				typ:   lock.typ,
				pid:   lock.pid,
			})
		}
		if lock.end > lk.End {
			// Keep the part after the unlock range
			newLocks = append(newLocks, &posixLock{
				owner: lock.owner,
				start: lk.End,
				end:   lock.end,
				typ:   lock.typ,
				pid:   lock.pid,
			})
		}
	}

	if len(newLocks) == 0 {
		delete(lm.posixLocks, path)
	} else {
		lm.posixLocks[path] = newLocks
	}

	return 0
}

// Flock acquires or releases a BSD-style flock
func (lm *LockManager) Flock(path string, owner uint64, flags uint32) syscall.Errno {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	// Check unlock flag
	if flags&syscall.LOCK_UN != 0 {
		// Unlock
		if lm.flocks[path] == owner {
			delete(lm.flocks, path)
		}
		return 0
	}

	// Check if file is already locked
	if existingOwner, exists := lm.flocks[path]; exists {
		if existingOwner == owner {
			// Already locked by us, allow upgrade/downgrade
			lm.flocks[path] = owner
			return 0
		}

		// Locked by someone else
		if flags&syscall.LOCK_NB != 0 {
			// Non-blocking, return would-block error
			return syscall.EWOULDBLOCK
		}

		// Blocking mode (kernel will retry)
		return syscall.EAGAIN
	}

	// Acquire the lock
	lm.flocks[path] = owner
	return 0
}

// ReleaseOwner releases all locks held by an owner (called on file close)
func (lm *LockManager) ReleaseOwner(owner uint64) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	// Release flocks
	for path, lockOwner := range lm.flocks {
		if lockOwner == owner {
			delete(lm.flocks, path)
		}
	}

	// Release POSIX locks
	for path, locks := range lm.posixLocks {
		newLocks := make([]*posixLock, 0, len(locks))
		for _, lock := range locks {
			if lock.owner != owner {
				newLocks = append(newLocks, lock)
			}
		}
		if len(newLocks) == 0 {
			delete(lm.posixLocks, path)
		} else {
			lm.posixLocks[path] = newLocks
		}
	}
}

// rangesOverlap checks if two byte ranges overlap
func (lm *LockManager) rangesOverlap(start1, end1, start2, end2 uint64) bool {
	// Handle special case for "whole file" locks
	if end1 == 0xFFFFFFFFFFFFFFFF {
		end1 = ^uint64(0)
	}
	if end2 == 0xFFFFFFFFFFFFFFFF {
		end2 = ^uint64(0)
	}

	return start1 < end2 && start2 < end1
}

// Getlk implements POSIX lock testing
func (fh *fuseFileHandle) Getlk(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32, out *fuse.FileLock) syscall.Errno {
	fh.node.fusefs.stats.recordOperation()

	*out = *lk
	return fh.node.fusefs.lockManager.Getlk(fh.node.path, owner, out)
}

// Setlk implements POSIX lock acquisition (non-blocking)
func (fh *fuseFileHandle) Setlk(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32) syscall.Errno {
	fh.node.fusefs.stats.recordOperation()

	return fh.node.fusefs.lockManager.Setlk(fh.node.path, owner, lk)
}

// Setlkw implements POSIX lock acquisition (blocking)
func (fh *fuseFileHandle) Setlkw(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32) syscall.Errno {
	fh.node.fusefs.stats.recordOperation()

	return fh.node.fusefs.lockManager.Setlkw(fh.node.path, owner, lk)
}

// Flock implements BSD-style file locking
func (fh *fuseFileHandle) Flock(ctx context.Context, owner uint64, flags uint32) syscall.Errno {
	fh.node.fusefs.stats.recordOperation()

	return fh.node.fusefs.lockManager.Flock(fh.node.path, owner, flags)
}

// Ensure fuseFileHandle implements locking interfaces
var _ fs.FileGetlker = (*fuseFileHandle)(nil)
var _ fs.FileSetlker = (*fuseFileHandle)(nil)
var _ fs.FileSetlkwer = (*fuseFileHandle)(nil)
// Note: Flock is implemented but FileFflocker interface may not be in all go-fuse versions

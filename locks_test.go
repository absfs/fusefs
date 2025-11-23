package fusefs

import (
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v2/fuse"
)

func TestLockManager_Flock(t *testing.T) {
	lm := NewLockManager()

	// Acquire exclusive lock
	err := lm.Flock("/test.txt", 1, syscall.LOCK_EX)
	if err != 0 {
		t.Errorf("Failed to acquire exclusive lock: %v", err)
	}

	// Try to acquire conflicting lock (non-blocking)
	err = lm.Flock("/test.txt", 2, syscall.LOCK_EX|syscall.LOCK_NB)
	if err != syscall.EWOULDBLOCK {
		t.Errorf("Expected EWOULDBLOCK, got %v", err)
	}

	// Release lock
	err = lm.Flock("/test.txt", 1, syscall.LOCK_UN)
	if err != 0 {
		t.Errorf("Failed to release lock: %v", err)
	}

	// Now second lock should succeed
	err = lm.Flock("/test.txt", 2, syscall.LOCK_EX|syscall.LOCK_NB)
	if err != 0 {
		t.Errorf("Failed to acquire lock after release: %v", err)
	}
}

func TestLockManager_FlockShared(t *testing.T) {
	lm := NewLockManager()

	// Acquire shared lock
	err := lm.Flock("/test.txt", 1, syscall.LOCK_SH)
	if err != 0 {
		t.Errorf("Failed to acquire shared lock: %v", err)
	}

	// Another shared lock should succeed
	err = lm.Flock("/test.txt", 2, syscall.LOCK_SH)
	if err != 0 {
		t.Errorf("Failed to acquire second shared lock: %v", err)
	}

	// Exclusive lock should fail
	err = lm.Flock("/test.txt", 3, syscall.LOCK_EX|syscall.LOCK_NB)
	if err != syscall.EWOULDBLOCK {
		t.Errorf("Expected EWOULDBLOCK for exclusive lock with shared locks held, got %v", err)
	}
}

func TestLockManager_FlockUpgrade(t *testing.T) {
	lm := NewLockManager()

	// Acquire shared lock
	err := lm.Flock("/test.txt", 1, syscall.LOCK_SH)
	if err != 0 {
		t.Errorf("Failed to acquire shared lock: %v", err)
	}

	// Upgrade to exclusive (same owner)
	err = lm.Flock("/test.txt", 1, syscall.LOCK_EX)
	if err != 0 {
		t.Errorf("Failed to upgrade lock: %v", err)
	}
}

func TestLockManager_PosixGetlk(t *testing.T) {
	lm := NewLockManager()

	// No locks initially
	lk := &fuse.FileLock{
		Start: 0,
		End:   100,
		Typ:   syscall.F_WRLCK,
		Pid:   1234,
	}

	err := lm.Getlk("/test.txt", 1, lk)
	if err != 0 {
		t.Errorf("Getlk failed: %v", err)
	}
	if lk.Typ != syscall.F_UNLCK {
		t.Errorf("Expected F_UNLCK, got %d", lk.Typ)
	}
}

func TestLockManager_PosixSetlk(t *testing.T) {
	lm := NewLockManager()

	// Acquire write lock
	lk := &fuse.FileLock{
		Start: 0,
		End:   100,
		Typ:   syscall.F_WRLCK,
		Pid:   1234,
	}

	err := lm.Setlk("/test.txt", 1, lk)
	if err != 0 {
		t.Errorf("Failed to acquire write lock: %v", err)
	}

	// Try to acquire conflicting lock
	lk2 := &fuse.FileLock{
		Start: 50,
		End:   150,
		Typ:   syscall.F_WRLCK,
		Pid:   5678,
	}

	err = lm.Setlk("/test.txt", 2, lk2)
	if err != syscall.EAGAIN {
		t.Errorf("Expected EAGAIN for conflicting lock, got %v", err)
	}
}

func TestLockManager_PosixReadWriteLock(t *testing.T) {
	lm := NewLockManager()

	// Acquire read lock
	rlk := &fuse.FileLock{
		Start: 0,
		End:   100,
		Typ:   syscall.F_RDLCK,
		Pid:   1234,
	}

	err := lm.Setlk("/test.txt", 1, rlk)
	if err != 0 {
		t.Errorf("Failed to acquire read lock: %v", err)
	}

	// Another read lock should succeed
	rlk2 := &fuse.FileLock{
		Start: 50,
		End:   150,
		Typ:   syscall.F_RDLCK,
		Pid:   5678,
	}

	err = lm.Setlk("/test.txt", 2, rlk2)
	if err != 0 {
		t.Errorf("Failed to acquire second read lock: %v", err)
	}

	// Write lock should fail
	wlk := &fuse.FileLock{
		Start: 75,
		End:   125,
		Typ:   syscall.F_WRLCK,
		Pid:   9999,
	}

	err = lm.Setlk("/test.txt", 3, wlk)
	if err != syscall.EAGAIN {
		t.Errorf("Expected EAGAIN for write lock conflicting with read locks, got %v", err)
	}
}

func TestLockManager_PosixUnlock(t *testing.T) {
	lm := NewLockManager()

	// Acquire lock
	lk := &fuse.FileLock{
		Start: 0,
		End:   100,
		Typ:   syscall.F_WRLCK,
		Pid:   1234,
	}

	err := lm.Setlk("/test.txt", 1, lk)
	if err != 0 {
		t.Errorf("Failed to acquire lock: %v", err)
	}

	// Unlock
	ulk := &fuse.FileLock{
		Start: 0,
		End:   100,
		Typ:   syscall.F_UNLCK,
		Pid:   1234,
	}

	err = lm.Setlk("/test.txt", 1, ulk)
	if err != 0 {
		t.Errorf("Failed to unlock: %v", err)
	}

	// Now another lock should succeed
	lk2 := &fuse.FileLock{
		Start: 0,
		End:   100,
		Typ:   syscall.F_WRLCK,
		Pid:   5678,
	}

	err = lm.Setlk("/test.txt", 2, lk2)
	if err != 0 {
		t.Errorf("Failed to acquire lock after unlock: %v", err)
	}
}

func TestLockManager_PosixPartialUnlock(t *testing.T) {
	lm := NewLockManager()

	// Acquire lock on range 0-200
	lk := &fuse.FileLock{
		Start: 0,
		End:   200,
		Typ:   syscall.F_WRLCK,
		Pid:   1234,
	}

	err := lm.Setlk("/test.txt", 1, lk)
	if err != 0 {
		t.Errorf("Failed to acquire lock: %v", err)
	}

	// Unlock middle part (50-150)
	ulk := &fuse.FileLock{
		Start: 50,
		End:   150,
		Typ:   syscall.F_UNLCK,
		Pid:   1234,
	}

	err = lm.Setlk("/test.txt", 1, ulk)
	if err != 0 {
		t.Errorf("Failed to unlock middle: %v", err)
	}

	// Lock in the middle should now succeed
	mlk := &fuse.FileLock{
		Start: 75,
		End:   125,
		Typ:   syscall.F_WRLCK,
		Pid:   5678,
	}

	err = lm.Setlk("/test.txt", 2, mlk)
	if err != 0 {
		t.Errorf("Failed to acquire lock in unlocked middle: %v", err)
	}

	// But locks at edges should still conflict
	elk1 := &fuse.FileLock{
		Start: 10,
		End:   60,
		Typ:   syscall.F_WRLCK,
		Pid:   9999,
	}

	err = lm.Setlk("/test.txt", 3, elk1)
	if err != syscall.EAGAIN {
		t.Errorf("Expected conflict with edge lock 1, got %v", err)
	}
}

func TestLockManager_ReleaseOwner(t *testing.T) {
	lm := NewLockManager()

	// Acquire flock
	err := lm.Flock("/test1.txt", 1, syscall.LOCK_EX)
	if err != 0 {
		t.Errorf("Failed to acquire flock: %v", err)
	}

	// Acquire POSIX lock
	lk := &fuse.FileLock{
		Start: 0,
		End:   100,
		Typ:   syscall.F_WRLCK,
		Pid:   1234,
	}

	err = lm.Setlk("/test2.txt", 1, lk)
	if err != 0 {
		t.Errorf("Failed to acquire POSIX lock: %v", err)
	}

	// Release all locks for owner 1
	lm.ReleaseOwner(1)

	// Both locks should be released
	err = lm.Flock("/test1.txt", 2, syscall.LOCK_EX|syscall.LOCK_NB)
	if err != 0 {
		t.Errorf("Flock should be released, got %v", err)
	}

	lk2 := &fuse.FileLock{
		Start: 0,
		End:   100,
		Typ:   syscall.F_WRLCK,
		Pid:   5678,
	}

	err = lm.Setlk("/test2.txt", 2, lk2)
	if err != 0 {
		t.Errorf("POSIX lock should be released, got %v", err)
	}
}

func TestLockManager_NoConflictSameOwner(t *testing.T) {
	lm := NewLockManager()

	// Acquire lock
	lk := &fuse.FileLock{
		Start: 0,
		End:   100,
		Typ:   syscall.F_WRLCK,
		Pid:   1234,
	}

	err := lm.Setlk("/test.txt", 1, lk)
	if err != 0 {
		t.Errorf("Failed to acquire lock: %v", err)
	}

	// Same owner should be able to acquire overlapping lock
	lk2 := &fuse.FileLock{
		Start: 50,
		End:   150,
		Typ:   syscall.F_WRLCK,
		Pid:   1234,
	}

	err = lm.Setlk("/test.txt", 1, lk2)
	if err != 0 {
		t.Errorf("Same owner should be able to acquire overlapping lock, got %v", err)
	}
}

func TestLockManager_RangeOverlap(t *testing.T) {
	lm := NewLockManager()

	tests := []struct {
		start1, end1, start2, end2 uint64
		shouldOverlap              bool
	}{
		{0, 100, 50, 150, true},   // Overlap
		{0, 100, 100, 200, false}, // Adjacent, no overlap
		{0, 100, 200, 300, false}, // No overlap
		{50, 150, 0, 100, true},   // Overlap (reverse)
		{0, 100, 25, 75, true},    // Contained
		{25, 75, 0, 100, true},    // Contains
	}

	for _, tt := range tests {
		result := lm.rangesOverlap(tt.start1, tt.end1, tt.start2, tt.end2)
		if result != tt.shouldOverlap {
			t.Errorf("rangesOverlap(%d, %d, %d, %d) = %v, want %v",
				tt.start1, tt.end1, tt.start2, tt.end2, result, tt.shouldOverlap)
		}
	}
}

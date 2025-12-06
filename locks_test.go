package fusefs

import (
	"sync"
	"sync/atomic"
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

// Phase 2.1: Shared Lock Test Cases

func TestLockManager_FlockMultipleShared(t *testing.T) {
	lm := NewLockManager()

	// Acquire 5 shared locks from different owners
	for i := uint64(1); i <= 5; i++ {
		err := lm.Flock("/test.txt", i, syscall.LOCK_SH)
		if err != 0 {
			t.Errorf("Failed to acquire shared lock %d: %v", i, err)
		}
	}

	// All should coexist - exclusive lock should fail
	err := lm.Flock("/test.txt", 100, syscall.LOCK_EX|syscall.LOCK_NB)
	if err != syscall.EWOULDBLOCK {
		t.Errorf("Expected EWOULDBLOCK with 5 shared locks held, got %v", err)
	}

	// Release one shared lock - others should still block exclusive
	err = lm.Flock("/test.txt", 1, syscall.LOCK_UN)
	if err != 0 {
		t.Errorf("Failed to release shared lock: %v", err)
	}

	err = lm.Flock("/test.txt", 100, syscall.LOCK_EX|syscall.LOCK_NB)
	if err != syscall.EWOULDBLOCK {
		t.Errorf("Expected EWOULDBLOCK with 4 shared locks still held, got %v", err)
	}
}

func TestLockManager_FlockSharedExclusiveConflict(t *testing.T) {
	lm := NewLockManager()

	// Acquire shared lock
	err := lm.Flock("/test.txt", 1, syscall.LOCK_SH)
	if err != 0 {
		t.Fatalf("Failed to acquire shared lock: %v", err)
	}

	// Exclusive lock from different owner should fail
	err = lm.Flock("/test.txt", 2, syscall.LOCK_EX|syscall.LOCK_NB)
	if err != syscall.EWOULDBLOCK {
		t.Errorf("Expected EWOULDBLOCK, got %v", err)
	}

	// Blocking mode should return EAGAIN
	err = lm.Flock("/test.txt", 2, syscall.LOCK_EX)
	if err != syscall.EAGAIN {
		t.Errorf("Expected EAGAIN for blocking mode, got %v", err)
	}
}

func TestLockManager_FlockExclusiveBlocksShared(t *testing.T) {
	lm := NewLockManager()

	// Acquire exclusive lock
	err := lm.Flock("/test.txt", 1, syscall.LOCK_EX)
	if err != 0 {
		t.Fatalf("Failed to acquire exclusive lock: %v", err)
	}

	// Shared lock from different owner should fail
	err = lm.Flock("/test.txt", 2, syscall.LOCK_SH|syscall.LOCK_NB)
	if err != syscall.EWOULDBLOCK {
		t.Errorf("Expected EWOULDBLOCK for shared lock with exclusive held, got %v", err)
	}

	// Blocking mode should return EAGAIN
	err = lm.Flock("/test.txt", 2, syscall.LOCK_SH)
	if err != syscall.EAGAIN {
		t.Errorf("Expected EAGAIN for blocking mode, got %v", err)
	}
}

func TestLockManager_FlockSharedRelease(t *testing.T) {
	lm := NewLockManager()

	// Acquire 3 shared locks
	for i := uint64(1); i <= 3; i++ {
		err := lm.Flock("/test.txt", i, syscall.LOCK_SH)
		if err != 0 {
			t.Fatalf("Failed to acquire shared lock %d: %v", i, err)
		}
	}

	// Release locks one by one
	for i := uint64(1); i <= 3; i++ {
		err := lm.Flock("/test.txt", i, syscall.LOCK_UN)
		if err != 0 {
			t.Errorf("Failed to release shared lock %d: %v", i, err)
		}

		// If not all released, exclusive should still fail
		if i < 3 {
			err = lm.Flock("/test.txt", 100, syscall.LOCK_EX|syscall.LOCK_NB)
			if err != syscall.EWOULDBLOCK {
				t.Errorf("Expected EWOULDBLOCK after releasing %d locks, got %v", i, err)
			}
		}
	}

	// Now exclusive lock should succeed
	err := lm.Flock("/test.txt", 100, syscall.LOCK_EX|syscall.LOCK_NB)
	if err != 0 {
		t.Errorf("Exclusive lock should succeed after all shared released: %v", err)
	}
}

func TestLockManager_FlockUpgradeFromShared(t *testing.T) {
	lm := NewLockManager()

	// Acquire shared locks from two owners
	err := lm.Flock("/test.txt", 1, syscall.LOCK_SH)
	if err != 0 {
		t.Fatalf("Failed to acquire shared lock 1: %v", err)
	}

	err = lm.Flock("/test.txt", 2, syscall.LOCK_SH)
	if err != 0 {
		t.Fatalf("Failed to acquire shared lock 2: %v", err)
	}

	// Owner 1 cannot upgrade to exclusive while owner 2 holds shared
	err = lm.Flock("/test.txt", 1, syscall.LOCK_EX|syscall.LOCK_NB)
	if err != syscall.EWOULDBLOCK {
		t.Errorf("Expected EWOULDBLOCK for upgrade with other shared holders, got %v", err)
	}

	// Release owner 2's lock
	err = lm.Flock("/test.txt", 2, syscall.LOCK_UN)
	if err != 0 {
		t.Fatalf("Failed to release owner 2's lock: %v", err)
	}

	// Now owner 1 can upgrade
	err = lm.Flock("/test.txt", 1, syscall.LOCK_EX)
	if err != 0 {
		t.Errorf("Failed to upgrade lock after other shared released: %v", err)
	}
}

func TestLockManager_FlockDowngrade(t *testing.T) {
	lm := NewLockManager()

	// Acquire exclusive lock
	err := lm.Flock("/test.txt", 1, syscall.LOCK_EX)
	if err != 0 {
		t.Fatalf("Failed to acquire exclusive lock: %v", err)
	}

	// Downgrade to shared
	err = lm.Flock("/test.txt", 1, syscall.LOCK_SH)
	if err != 0 {
		t.Errorf("Failed to downgrade to shared: %v", err)
	}

	// Another shared lock should now succeed
	err = lm.Flock("/test.txt", 2, syscall.LOCK_SH)
	if err != 0 {
		t.Errorf("Second shared lock should succeed after downgrade: %v", err)
	}
}

// Phase 2.2: Concurrent Lock Tests

func TestLockManager_FlockConcurrentShared(t *testing.T) {
	lm := NewLockManager()
	var wg sync.WaitGroup
	successCount := int32(0)
	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(owner uint64) {
			defer wg.Done()
			err := lm.Flock("/test.txt", owner, syscall.LOCK_SH)
			if err == 0 {
				atomic.AddInt32(&successCount, 1)
			}
		}(uint64(i + 1))
	}

	wg.Wait()

	if successCount != int32(numGoroutines) {
		t.Errorf("Expected all %d shared locks to succeed, got %d", numGoroutines, successCount)
	}
}

func TestLockManager_FlockConcurrentExclusive(t *testing.T) {
	lm := NewLockManager()
	var wg sync.WaitGroup
	successCount := int32(0)
	numGoroutines := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(owner uint64) {
			defer wg.Done()
			err := lm.Flock("/test.txt", owner, syscall.LOCK_EX|syscall.LOCK_NB)
			if err == 0 {
				atomic.AddInt32(&successCount, 1)
			}
		}(uint64(i + 1))
	}

	wg.Wait()

	// Only one exclusive lock should succeed
	if successCount != 1 {
		t.Errorf("Expected exactly 1 exclusive lock to succeed, got %d", successCount)
	}
}

func TestLockManager_FlockConcurrentMixed(t *testing.T) {
	lm := NewLockManager()
	var wg sync.WaitGroup
	sharedSuccess := int32(0)
	exclusiveSuccess := int32(0)

	// First acquire a shared lock
	err := lm.Flock("/test.txt", 1, syscall.LOCK_SH)
	if err != 0 {
		t.Fatalf("Failed to acquire initial shared lock: %v", err)
	}
	atomic.AddInt32(&sharedSuccess, 1)

	// Launch concurrent requests
	for i := 2; i <= 100; i++ {
		wg.Add(1)
		go func(owner uint64, isShared bool) {
			defer wg.Done()
			if isShared {
				err := lm.Flock("/test.txt", owner, syscall.LOCK_SH)
				if err == 0 {
					atomic.AddInt32(&sharedSuccess, 1)
				}
			} else {
				err := lm.Flock("/test.txt", owner, syscall.LOCK_EX|syscall.LOCK_NB)
				if err == 0 {
					atomic.AddInt32(&exclusiveSuccess, 1)
				}
			}
		}(uint64(i), i%3 != 0) // Every 3rd request is exclusive
	}

	wg.Wait()

	// All shared locks should succeed
	expectedShared := int32(1) // Initial lock
	for i := 2; i <= 100; i++ {
		if i%3 != 0 {
			expectedShared++
		}
	}

	if sharedSuccess != expectedShared {
		t.Errorf("Expected %d shared locks to succeed, got %d", expectedShared, sharedSuccess)
	}

	// No exclusive locks should succeed (shared locks are held)
	if exclusiveSuccess != 0 {
		t.Errorf("Expected 0 exclusive locks to succeed, got %d", exclusiveSuccess)
	}
}

func TestLockManager_FlockRaceConditions(t *testing.T) {
	lm := NewLockManager()
	var wg sync.WaitGroup
	iterations := 1000

	// Stress test with rapid acquire/release cycles
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(owner uint64) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				// Acquire
				lm.Flock("/test.txt", owner, syscall.LOCK_SH)
				// Release
				lm.Flock("/test.txt", owner, syscall.LOCK_UN)
			}
		}(uint64(i + 1))
	}

	wg.Wait()

	// After all releases, exclusive lock should succeed
	err := lm.Flock("/test.txt", 100, syscall.LOCK_EX|syscall.LOCK_NB)
	if err != 0 {
		t.Errorf("Exclusive lock should succeed after stress test: %v", err)
	}
}

// Phase 2.3: Edge Case Tests

func TestLockManager_FlockSameOwnerMultipleCalls(t *testing.T) {
	lm := NewLockManager()

	// Same owner acquiring same lock multiple times
	for i := 0; i < 10; i++ {
		err := lm.Flock("/test.txt", 1, syscall.LOCK_SH)
		if err != 0 {
			t.Errorf("Call %d: Failed to acquire same shared lock: %v", i, err)
		}
	}

	// Should only need one unlock
	err := lm.Flock("/test.txt", 1, syscall.LOCK_UN)
	if err != 0 {
		t.Errorf("Failed to unlock: %v", err)
	}

	// Now another owner should be able to get exclusive
	err = lm.Flock("/test.txt", 2, syscall.LOCK_EX|syscall.LOCK_NB)
	if err != 0 {
		t.Errorf("Exclusive lock should succeed after unlock: %v", err)
	}
}

func TestLockManager_FlockUnlockNonExistent(t *testing.T) {
	lm := NewLockManager()

	// Unlock when no lock exists - should succeed silently
	err := lm.Flock("/test.txt", 1, syscall.LOCK_UN)
	if err != 0 {
		t.Errorf("Unlock of non-existent lock should succeed: %v", err)
	}
}

func TestLockManager_FlockUnlockWrongOwner(t *testing.T) {
	lm := NewLockManager()

	// Acquire lock
	err := lm.Flock("/test.txt", 1, syscall.LOCK_EX)
	if err != 0 {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	// Unlock with wrong owner - should succeed but not release the lock
	err = lm.Flock("/test.txt", 2, syscall.LOCK_UN)
	if err != 0 {
		t.Errorf("Unlock with wrong owner should succeed: %v", err)
	}

	// Original lock should still be held
	err = lm.Flock("/test.txt", 3, syscall.LOCK_EX|syscall.LOCK_NB)
	if err != syscall.EWOULDBLOCK {
		t.Errorf("Lock should still be held, got %v", err)
	}
}

func TestLockManager_FlockMultipleFiles(t *testing.T) {
	lm := NewLockManager()

	// Same owner can lock multiple files
	files := []string{"/file1.txt", "/file2.txt", "/file3.txt"}

	for _, f := range files {
		err := lm.Flock(f, 1, syscall.LOCK_EX)
		if err != 0 {
			t.Errorf("Failed to lock %s: %v", f, err)
		}
	}

	// Different owner can't lock any of them
	for _, f := range files {
		err := lm.Flock(f, 2, syscall.LOCK_EX|syscall.LOCK_NB)
		if err != syscall.EWOULDBLOCK {
			t.Errorf("Expected EWOULDBLOCK for %s, got %v", f, err)
		}
	}

	// Release all
	lm.ReleaseOwner(1)

	// Now all should be available
	for _, f := range files {
		err := lm.Flock(f, 2, syscall.LOCK_EX|syscall.LOCK_NB)
		if err != 0 {
			t.Errorf("Failed to lock %s after release: %v", f, err)
		}
	}
}

// Phase 2.4: POSIX Lock Edge Cases

func TestLockManager_PosixWholeFileLock(t *testing.T) {
	lm := NewLockManager()

	// Lock entire file (start=0, end=max)
	lk := &fuse.FileLock{
		Start: 0,
		End:   ^uint64(0),
		Typ:   syscall.F_WRLCK,
		Pid:   1234,
	}

	err := lm.Setlk("/test.txt", 1, lk)
	if err != 0 {
		t.Errorf("Failed to acquire whole-file lock: %v", err)
	}

	// Any other lock should conflict
	lk2 := &fuse.FileLock{
		Start: 1000,
		End:   2000,
		Typ:   syscall.F_WRLCK,
		Pid:   5678,
	}

	err = lm.Setlk("/test.txt", 2, lk2)
	if err != syscall.EAGAIN {
		t.Errorf("Expected EAGAIN for any range with whole-file lock, got %v", err)
	}
}

func TestLockManager_PosixZeroLengthLock(t *testing.T) {
	lm := NewLockManager()

	// Lock with start=end (zero length)
	lk := &fuse.FileLock{
		Start: 100,
		End:   100,
		Typ:   syscall.F_WRLCK,
		Pid:   1234,
	}

	err := lm.Setlk("/test.txt", 1, lk)
	if err != 0 {
		t.Errorf("Failed to acquire zero-length lock: %v", err)
	}

	// Adjacent lock should not conflict
	lk2 := &fuse.FileLock{
		Start: 100,
		End:   200,
		Typ:   syscall.F_WRLCK,
		Pid:   5678,
	}

	err = lm.Setlk("/test.txt", 2, lk2)
	if err != 0 {
		t.Errorf("Adjacent lock should succeed: %v", err)
	}
}

func TestLockManager_PosixLockPastEOF(t *testing.T) {
	lm := NewLockManager()

	// Lock a range far past any realistic EOF
	lk := &fuse.FileLock{
		Start: 1 << 50,
		End:   1<<50 + 1000,
		Typ:   syscall.F_WRLCK,
		Pid:   1234,
	}

	err := lm.Setlk("/test.txt", 1, lk)
	if err != 0 {
		t.Errorf("Failed to acquire lock past EOF: %v", err)
	}

	// Lock at beginning should not conflict
	lk2 := &fuse.FileLock{
		Start: 0,
		End:   1000,
		Typ:   syscall.F_WRLCK,
		Pid:   5678,
	}

	err = lm.Setlk("/test.txt", 2, lk2)
	if err != 0 {
		t.Errorf("Non-overlapping lock should succeed: %v", err)
	}
}

func TestLockManager_PosixGetlkConflict(t *testing.T) {
	lm := NewLockManager()

	// Acquire a write lock
	lk := &fuse.FileLock{
		Start: 100,
		End:   200,
		Typ:   syscall.F_WRLCK,
		Pid:   1234,
	}

	err := lm.Setlk("/test.txt", 1, lk)
	if err != 0 {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	// Test for conflicting lock
	testLk := &fuse.FileLock{
		Start: 150,
		End:   250,
		Typ:   syscall.F_WRLCK,
		Pid:   5678,
	}

	err = lm.Getlk("/test.txt", 2, testLk)
	if err != 0 {
		t.Errorf("Getlk failed: %v", err)
	}

	// Should return the conflicting lock info
	if testLk.Typ != syscall.F_WRLCK {
		t.Errorf("Expected F_WRLCK conflict, got %d", testLk.Typ)
	}
	if testLk.Start != 100 || testLk.End != 200 {
		t.Errorf("Expected range 100-200, got %d-%d", testLk.Start, testLk.End)
	}
}

func TestLockManager_PosixConcurrent(t *testing.T) {
	lm := NewLockManager()
	var wg sync.WaitGroup
	successCount := int32(0)

	// Concurrent read locks on overlapping ranges should succeed
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(owner uint64) {
			defer wg.Done()
			lk := &fuse.FileLock{
				Start: 0,
				End:   1000,
				Typ:   syscall.F_RDLCK,
				Pid:   uint32(owner),
			}
			err := lm.Setlk("/test.txt", owner, lk)
			if err == 0 {
				atomic.AddInt32(&successCount, 1)
			}
		}(uint64(i + 1))
	}

	wg.Wait()

	if successCount != 100 {
		t.Errorf("Expected all 100 read locks to succeed, got %d", successCount)
	}
}

func TestLockManager_Setlkw(t *testing.T) {
	lm := NewLockManager()

	// Acquire a lock
	lk := &fuse.FileLock{
		Start: 0,
		End:   100,
		Typ:   syscall.F_WRLCK,
		Pid:   1234,
	}

	err := lm.Setlk("/test.txt", 1, lk)
	if err != 0 {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	// Setlkw should return EAGAIN (kernel will retry)
	lk2 := &fuse.FileLock{
		Start: 50,
		End:   150,
		Typ:   syscall.F_WRLCK,
		Pid:   5678,
	}

	err = lm.Setlkw("/test.txt", 2, lk2)
	if err != syscall.EAGAIN {
		t.Errorf("Expected EAGAIN for blocking lock, got %v", err)
	}
}

func TestLockManager_ReleaseOwnerSharedLocks(t *testing.T) {
	lm := NewLockManager()

	// Multiple owners with shared locks
	for i := uint64(1); i <= 5; i++ {
		err := lm.Flock("/test.txt", i, syscall.LOCK_SH)
		if err != 0 {
			t.Fatalf("Failed to acquire shared lock %d: %v", i, err)
		}
	}

	// Release just owner 3
	lm.ReleaseOwner(3)

	// Exclusive lock should still fail (other shared holders exist)
	err := lm.Flock("/test.txt", 100, syscall.LOCK_EX|syscall.LOCK_NB)
	if err != syscall.EWOULDBLOCK {
		t.Errorf("Expected EWOULDBLOCK, got %v", err)
	}

	// Release all remaining
	for i := uint64(1); i <= 5; i++ {
		if i != 3 {
			lm.ReleaseOwner(i)
		}
	}

	// Now exclusive should succeed
	err = lm.Flock("/test.txt", 100, syscall.LOCK_EX|syscall.LOCK_NB)
	if err != 0 {
		t.Errorf("Exclusive should succeed after all released: %v", err)
	}
}

// Benchmarks

func BenchmarkLockManager_Flock(b *testing.B) {
	lm := NewLockManager()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lm.Flock("/test.txt", uint64(i), syscall.LOCK_EX)
		lm.Flock("/test.txt", uint64(i), syscall.LOCK_UN)
	}
}

func BenchmarkLockManager_FlockShared(b *testing.B) {
	lm := NewLockManager()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lm.Flock("/test.txt", uint64(i), syscall.LOCK_SH)
	}
}

func BenchmarkLockManager_PosixLock(b *testing.B) {
	lm := NewLockManager()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lk := &fuse.FileLock{
			Start: uint64(i * 100),
			End:   uint64((i + 1) * 100),
			Typ:   syscall.F_WRLCK,
			Pid:   uint32(i),
		}
		lm.Setlk("/test.txt", uint64(i), lk)
	}
}

func BenchmarkLockManager_ConcurrentShared(b *testing.B) {
	lm := NewLockManager()
	var wg sync.WaitGroup

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wg.Add(10)
		for j := 0; j < 10; j++ {
			go func(owner uint64) {
				defer wg.Done()
				lm.Flock("/test.txt", owner, syscall.LOCK_SH)
			}(uint64(i*10 + j))
		}
		wg.Wait()
		// Clean up for next iteration
		for j := 0; j < 10; j++ {
			lm.Flock("/test.txt", uint64(i*10+j), syscall.LOCK_UN)
		}
	}
}

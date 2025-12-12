package fusefs

import (
	"errors"
	"io/fs"
	"os"
	"sync"
	"syscall"
	"testing"
)

// mockFile implements absfs.File for testing
type mockFile struct {
	name   string
	closed bool
	data   []byte
	pos    int64
}

func newMockFile(name string) *mockFile {
	return &mockFile{name: name}
}

func (m *mockFile) Name() string {
	return m.name
}

func (m *mockFile) Read(p []byte) (n int, err error) {
	if m.closed {
		return 0, os.ErrClosed
	}
	return 0, nil
}

func (m *mockFile) Write(p []byte) (n int, err error) {
	if m.closed {
		return 0, os.ErrClosed
	}
	m.data = append(m.data, p...)
	return len(p), nil
}

func (m *mockFile) Close() error {
	if m.closed {
		return os.ErrClosed
	}
	m.closed = true
	return nil
}

func (m *mockFile) Sync() error {
	if m.closed {
		return os.ErrClosed
	}
	return nil
}

func (m *mockFile) Stat() (os.FileInfo, error) {
	return nil, errors.New("not implemented")
}

func (m *mockFile) Readdir(count int) ([]os.FileInfo, error) {
	return nil, errors.New("not implemented")
}

func (m *mockFile) ReadDir(n int) ([]fs.DirEntry, error) {
	return nil, errors.New("not implemented")
}

func (m *mockFile) Seek(offset int64, whence int) (int64, error) {
	m.pos = offset
	return m.pos, nil
}

func (m *mockFile) ReadAt(b []byte, off int64) (n int, err error) {
	return 0, errors.New("not implemented")
}

func (m *mockFile) WriteAt(b []byte, off int64) (n int, err error) {
	return 0, errors.New("not implemented")
}

func (m *mockFile) WriteString(s string) (n int, err error) {
	return m.Write([]byte(s))
}

func (m *mockFile) Truncate(size int64) error {
	return errors.New("not implemented")
}

func (m *mockFile) Readdirnames(n int) (names []string, err error) {
	return nil, errors.New("not implemented")
}

func TestHandleTracker_AddGet(t *testing.T) {
	ht := NewHandleTracker()

	file := newMockFile("/test.txt")

	// Add a file handle
	fh := ht.Add(file, os.O_RDONLY, "/test.txt")
	if fh == 0 {
		t.Error("Add returned 0, expected non-zero handle")
	}

	// Get should return the same file
	retrieved := ht.Get(fh)
	if retrieved == nil {
		t.Error("Get returned nil")
	}
	if retrieved.Name() != file.Name() {
		t.Error("Get returned different file than added")
	}

	// Get non-existent handle should return nil
	retrieved = ht.Get(999)
	if retrieved != nil {
		t.Error("Get(999) should return nil for non-existent handle")
	}
}

func TestHandleTracker_MultipleHandles(t *testing.T) {
	ht := NewHandleTracker()

	file1 := newMockFile("/file1.txt")
	file2 := newMockFile("/file2.txt")

	fh1 := ht.Add(file1, os.O_RDONLY, "/file1.txt")
	fh2 := ht.Add(file2, os.O_RDWR, "/file2.txt")

	if fh1 == fh2 {
		t.Error("Add returned same handle for different files")
	}

	if ht.Get(fh1).Name() != file1.Name() {
		t.Error("Get(fh1) returned wrong file")
	}
	if ht.Get(fh2).Name() != file2.Name() {
		t.Error("Get(fh2) returned wrong file")
	}
}

func TestHandleTracker_Release(t *testing.T) {
	ht := NewHandleTracker()

	file := newMockFile("/test.txt")
	fh := ht.Add(file, os.O_RDONLY, "/test.txt")

	// Release should close the file
	errno := ht.Release(fh)
	if errno != 0 {
		t.Errorf("Release returned error: %v", errno)
	}

	if !file.closed {
		t.Error("File should be closed after release")
	}

	// Get should now return nil
	if ht.Get(fh) != nil {
		t.Error("Get should return nil after release")
	}

	// Releasing again should return EBADF
	errno = ht.Release(fh)
	if errno != syscall.EBADF {
		t.Errorf("Release(non-existent) = %v, want EBADF", errno)
	}
}

func TestHandleTracker_Count(t *testing.T) {
	ht := NewHandleTracker()

	if ht.Count() != 0 {
		t.Error("Initial count should be 0")
	}

	file1 := newMockFile("/file1.txt")
	file2 := newMockFile("/file2.txt")

	fh1 := ht.Add(file1, os.O_RDONLY, "/file1.txt")
	if ht.Count() != 1 {
		t.Errorf("Count = %d, want 1", ht.Count())
	}

	ht.Add(file2, os.O_RDONLY, "/file2.txt")
	if ht.Count() != 2 {
		t.Errorf("Count = %d, want 2", ht.Count())
	}

	ht.Release(fh1)
	if ht.Count() != 1 {
		t.Errorf("Count = %d, want 1 after release", ht.Count())
	}
}

func TestHandleTracker_CloseAll(t *testing.T) {
	ht := NewHandleTracker()

	file1 := newMockFile("/file1.txt")
	file2 := newMockFile("/file2.txt")
	file3 := newMockFile("/file3.txt")

	ht.Add(file1, os.O_RDONLY, "/file1.txt")
	ht.Add(file2, os.O_RDONLY, "/file2.txt")
	ht.Add(file3, os.O_RDONLY, "/file3.txt")

	if ht.Count() != 3 {
		t.Errorf("Count = %d, want 3", ht.Count())
	}

	// Close all
	ht.CloseAll()

	if ht.Count() != 0 {
		t.Errorf("Count = %d, want 0 after CloseAll", ht.Count())
	}

	if !file1.closed || !file2.closed || !file3.closed {
		t.Error("All files should be closed")
	}
}

func TestHandleTracker_GetEntry(t *testing.T) {
	ht := NewHandleTracker()

	path := "/test.txt"
	file := newMockFile(path)
	flags := os.O_RDWR

	fh := ht.Add(file, flags, path)

	entry := ht.GetEntry(fh)
	if entry == nil {
		t.Fatal("GetEntry returned nil")
	}

	if entry.file.Name() != file.Name() {
		t.Error("Entry has wrong file")
	}
	if entry.flags != flags {
		t.Errorf("Entry flags = %d, want %d", entry.flags, flags)
	}
	if entry.path != path {
		t.Errorf("Entry path = %s, want %s", entry.path, path)
	}
	if entry.refCount != 1 {
		t.Errorf("Entry refCount = %d, want 1", entry.refCount)
	}
}

// Additional Handle Tests for Phase 3

func TestHandleTracker_ConcurrentAdd(t *testing.T) {
	ht := NewHandleTracker()
	var wg sync.WaitGroup
	numGoroutines := 100

	handles := make(chan uint64, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			file := newMockFile("/file.txt")
			fh := ht.Add(file, os.O_RDONLY, "/file.txt")
			handles <- fh
		}(i)
	}

	wg.Wait()
	close(handles)

	// All handles should be unique
	seen := make(map[uint64]bool)
	for fh := range handles {
		if seen[fh] {
			t.Errorf("Duplicate handle: %d", fh)
		}
		seen[fh] = true
	}

	if ht.Count() != numGoroutines {
		t.Errorf("Count = %d, want %d", ht.Count(), numGoroutines)
	}
}

func TestHandleTracker_ConcurrentRelease(t *testing.T) {
	ht := NewHandleTracker()
	numHandles := 100

	// Add handles
	handles := make([]uint64, numHandles)
	for i := 0; i < numHandles; i++ {
		file := newMockFile("/file.txt")
		handles[i] = ht.Add(file, os.O_RDONLY, "/file.txt")
	}

	// Concurrently release them
	var wg sync.WaitGroup
	for _, fh := range handles {
		wg.Add(1)
		go func(h uint64) {
			defer wg.Done()
			ht.Release(h)
		}(fh)
	}

	wg.Wait()

	if ht.Count() != 0 {
		t.Errorf("Count = %d, want 0 after all releases", ht.Count())
	}
}

func TestHandleTracker_ConcurrentGetAndRelease(t *testing.T) {
	ht := NewHandleTracker()
	var wg sync.WaitGroup

	file := newMockFile("/test.txt")
	fh := ht.Add(file, os.O_RDONLY, "/test.txt")

	// Concurrent gets while the handle exists
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = ht.Get(fh)
			_ = ht.GetEntry(fh)
			_ = ht.Count()
		}()
	}

	wg.Wait()

	// Release the handle
	ht.Release(fh)

	// More concurrent gets after release
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result := ht.Get(fh)
			if result != nil {
				t.Error("Expected nil for released handle")
			}
		}()
	}

	wg.Wait()
}

func TestHandleTracker_GetEntryNonExistent(t *testing.T) {
	ht := NewHandleTracker()

	entry := ht.GetEntry(999)
	if entry != nil {
		t.Error("Expected nil for non-existent handle")
	}
}

func TestHandleTracker_UniqueHandleIDs(t *testing.T) {
	ht := NewHandleTracker()

	// Add and release many handles to ensure IDs keep incrementing
	handles := make(map[uint64]bool)
	for i := 0; i < 1000; i++ {
		file := newMockFile("/test.txt")
		fh := ht.Add(file, os.O_RDONLY, "/test.txt")
		if handles[fh] {
			t.Errorf("Handle %d was reused", fh)
		}
		handles[fh] = true
		ht.Release(fh)
	}
}

// mockFileWithError is a mock file that returns an error on Close
type mockFileWithError struct {
	mockFile
	closeErr error
}

func (m *mockFileWithError) Close() error {
	if m.closeErr != nil {
		return m.closeErr
	}
	return m.mockFile.Close()
}

func TestHandleTracker_ReleaseWithCloseError(t *testing.T) {
	ht := NewHandleTracker()

	file := &mockFileWithError{
		mockFile: mockFile{name: "/test.txt"},
		closeErr: os.ErrPermission,
	}

	fh := ht.Add(file, os.O_RDONLY, "/test.txt")

	errno := ht.Release(fh)
	if errno != syscall.EACCES {
		t.Errorf("Expected EACCES on close error, got %v", errno)
	}

	// Handle should still be removed
	if ht.Get(fh) != nil {
		t.Error("Handle should be removed even after close error")
	}
}

func BenchmarkHandleTracker_Add(b *testing.B) {
	ht := NewHandleTracker()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		file := newMockFile("/test.txt")
		ht.Add(file, os.O_RDONLY, "/test.txt")
	}
}

func BenchmarkHandleTracker_Get(b *testing.B) {
	ht := NewHandleTracker()
	file := newMockFile("/test.txt")
	fh := ht.Add(file, os.O_RDONLY, "/test.txt")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ht.Get(fh)
	}
}

func BenchmarkHandleTracker_AddRelease(b *testing.B) {
	ht := NewHandleTracker()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		file := newMockFile("/test.txt")
		fh := ht.Add(file, os.O_RDONLY, "/test.txt")
		ht.Release(fh)
	}
}

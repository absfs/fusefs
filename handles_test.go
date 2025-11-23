package fusefs

import (
	"errors"
	"os"
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

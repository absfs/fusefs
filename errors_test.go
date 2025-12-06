package fusefs

import (
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
	"testing"
)

func TestMapError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected syscall.Errno
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: 0,
		},
		{
			name:     "os.ErrNotExist",
			err:      os.ErrNotExist,
			expected: syscall.ENOENT,
		},
		{
			name:     "os.ErrExist",
			err:      os.ErrExist,
			expected: syscall.EEXIST,
		},
		{
			name:     "os.ErrPermission",
			err:      os.ErrPermission,
			expected: syscall.EACCES,
		},
		{
			name:     "os.ErrClosed",
			err:      os.ErrClosed,
			expected: syscall.EBADF,
		},
		{
			name:     "os.ErrInvalid",
			err:      os.ErrInvalid,
			expected: syscall.EINVAL,
		},
		{
			name:     "io.EOF",
			err:      io.EOF,
			expected: 0,
		},
		{
			name:     "wrapped os.ErrNotExist",
			err:      errors.Join(errors.New("wrapper"), os.ErrNotExist),
			expected: syscall.ENOENT,
		},
		{
			name:     "syscall.Errno directly",
			err:      syscall.ENOSPC,
			expected: syscall.ENOSPC,
		},
		{
			name:     "unknown error",
			err:      errors.New("unknown error"),
			expected: syscall.EIO,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapError(tt.err)
			if result != tt.expected {
				t.Errorf("mapError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

// Additional Error Tests for Phase 3

func TestMapError_WrappedErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected syscall.Errno
	}{
		{
			name:     "fmt.Errorf wrapped os.ErrNotExist",
			err:      fmt.Errorf("failed: %w", os.ErrNotExist),
			expected: syscall.ENOENT,
		},
		{
			name:     "double wrapped os.ErrExist",
			err:      fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", os.ErrExist)),
			expected: syscall.EEXIST,
		},
		{
			name:     "wrapped syscall.Errno",
			err:      fmt.Errorf("syscall failed: %w", syscall.ENOSPC),
			expected: syscall.ENOSPC,
		},
		{
			name:     "wrapped io.EOF",
			err:      fmt.Errorf("read failed: %w", io.EOF),
			expected: 0,
		},
		{
			name:     "wrapped os.ErrPermission",
			err:      fmt.Errorf("access denied: %w", os.ErrPermission),
			expected: syscall.EACCES,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapError(tt.err)
			if result != tt.expected {
				t.Errorf("mapError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestMapError_SyscallErrnoPassthrough(t *testing.T) {
	// Test that syscall.Errno values that are NOT mapped to standard os errors
	// pass through correctly. Note: some errnos like EPERM map to os.ErrPermission
	// which then becomes EACCES, so we only test errnos that don't have such mappings.
	errnos := []syscall.Errno{
		syscall.ESRCH,
		syscall.EINTR,
		syscall.EIO,
		syscall.ENXIO,
		syscall.ENOEXEC,
		syscall.ECHILD,
		syscall.EAGAIN,
		syscall.ENOMEM,
		syscall.EFAULT,
		syscall.EBUSY,
		syscall.EXDEV,
		syscall.ENODEV,
		syscall.ENOTDIR,
		syscall.EISDIR,
		syscall.ENFILE,
		syscall.EMFILE,
		syscall.ENOTTY,
		syscall.EFBIG,
		syscall.ENOSPC,
		syscall.ESPIPE,
		syscall.EROFS,
		syscall.EMLINK,
		syscall.EPIPE,
		syscall.EDOM,
		syscall.ERANGE,
		syscall.EDEADLK,
		syscall.ENAMETOOLONG,
		syscall.ENOLCK,
		syscall.ENOSYS,
		syscall.ELOOP,
	}

	for _, errno := range errnos {
		result := mapError(errno)
		if result != errno {
			t.Errorf("mapError(%v) = %v, want %v", errno, result, errno)
		}
	}
}

func TestMapError_PathError(t *testing.T) {
	// Test os.PathError which wraps syscall.Errno
	pathErr := &os.PathError{
		Op:   "open",
		Path: "/nonexistent",
		Err:  syscall.ENOENT,
	}

	result := mapError(pathErr)
	if result != syscall.ENOENT {
		t.Errorf("mapError(PathError) = %v, want ENOENT", result)
	}
}

func TestMapError_LinkError(t *testing.T) {
	// Test os.LinkError which can also wrap errno
	linkErr := &os.LinkError{
		Op:  "rename",
		Old: "/old",
		New: "/new",
		Err: syscall.EXDEV,
	}

	result := mapError(linkErr)
	if result != syscall.EXDEV {
		t.Errorf("mapError(LinkError) = %v, want EXDEV", result)
	}
}

func BenchmarkMapError_Nil(b *testing.B) {
	for i := 0; i < b.N; i++ {
		mapError(nil)
	}
}

func BenchmarkMapError_KnownError(b *testing.B) {
	for i := 0; i < b.N; i++ {
		mapError(os.ErrNotExist)
	}
}

func BenchmarkMapError_Errno(b *testing.B) {
	for i := 0; i < b.N; i++ {
		mapError(syscall.ENOSPC)
	}
}

func BenchmarkMapError_UnknownError(b *testing.B) {
	err := errors.New("unknown")
	for i := 0; i < b.N; i++ {
		mapError(err)
	}
}

package fusefs

import (
	"errors"
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

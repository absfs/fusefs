package fusefs

import (
	"errors"
	"io"
	"os"
	"syscall"
)

// mapError translates absfs errors to appropriate FUSE error codes
func mapError(err error) syscall.Errno {
	if err == nil {
		return 0
	}

	// Handle standard errors
	switch {
	case errors.Is(err, os.ErrNotExist):
		return syscall.ENOENT
	case errors.Is(err, os.ErrExist):
		return syscall.EEXIST
	case errors.Is(err, os.ErrPermission):
		return syscall.EACCES
	case errors.Is(err, os.ErrClosed):
		return syscall.EBADF
	case errors.Is(err, os.ErrInvalid):
		return syscall.EINVAL
	case errors.Is(err, io.EOF):
		return 0 // EOF is not an error for FUSE
	}

	// Check for syscall.Errno in error chain
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno
	}

	// Default to generic I/O error
	return syscall.EIO
}

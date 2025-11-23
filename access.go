package fusefs

import (
	"context"
	"os"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// Access constants for checking file permissions
const (
	F_OK = 0 // Test for existence
	X_OK = 1 // Test for execute permission
	W_OK = 2 // Test for write permission
	R_OK = 4 // Test for read permission
)

// Access checks if the caller has permission to access the file/directory.
//
// This implements the access() system call, which checks permissions based on:
//   - File mode (owner, group, other permissions)
//   - User ID and group ID of the caller
//   - Access mask (read, write, execute)
//
// The mask parameter is a bitwise OR of:
//   - R_OK (4): Test for read permission
//   - W_OK (2): Test for write permission
//   - X_OK (1): Test for execute permission
//   - F_OK (0): Test for existence only
//
// If DefaultPermissions mount option is set, the kernel performs permission
// checks and this method may not be called. Otherwise, this method is
// responsible for enforcing permission checks.
func (n *fuseNode) Access(ctx context.Context, mask uint32) syscall.Errno {
	n.fusefs.stats.recordOperation()

	if n.fusefs.checkUnmounting() {
		return syscall.ENOTCONN
	}

	// If DefaultPermissions is set, kernel handles permissions
	// We still implement Access for filesystems that don't use it
	if n.fusefs.opts.DefaultPermissions {
		// Kernel is handling permissions, always allow
		return 0
	}

	// Get file info to check permissions
	info, err := n.fusefs.absFS.Stat(n.path)
	if err != nil {
		n.fusefs.stats.recordError()
		return mapError(err)
	}

	// Get caller credentials from context
	caller, ok := fuse.FromContext(ctx)
	if !ok {
		// No caller info, deny access
		return syscall.EACCES
	}

	// Check if F_OK (existence only)
	if mask == F_OK {
		// File exists, access granted
		return 0
	}

	// Get file mode
	mode := info.Mode()
	perm := mode.Perm()

	// Determine which permission bits to check
	var checkPerm os.FileMode

	// Check if caller is owner
	if isOwner(info, caller.Uid) {
		// Use owner permissions
		checkPerm = (perm >> 6) & 0x7
	} else if isGroup(info, caller.Gid) {
		// Use group permissions
		checkPerm = (perm >> 3) & 0x7
	} else {
		// Use other permissions
		checkPerm = perm & 0x7
	}

	// Check read permission
	if mask&R_OK != 0 {
		if checkPerm&0x4 == 0 {
			return syscall.EACCES
		}
	}

	// Check write permission
	if mask&W_OK != 0 {
		if checkPerm&0x2 == 0 {
			return syscall.EACCES
		}
	}

	// Check execute permission
	if mask&X_OK != 0 {
		if checkPerm&0x1 == 0 {
			return syscall.EACCES
		}
	}

	return 0
}

// isOwner checks if the UID owns the file
func isOwner(info os.FileInfo, uid uint32) bool {
	// Try to extract UID from FileInfo
	// This is platform-specific and may not always work
	if sys := info.Sys(); sys != nil {
		if stat, ok := sys.(*syscall.Stat_t); ok {
			return stat.Uid == uid
		}
	}

	// If we can't determine ownership, assume caller is owner
	// This is safe since DefaultPermissions is the recommended approach
	return true
}

// isGroup checks if the GID matches the file's group
func isGroup(info os.FileInfo, gid uint32) bool {
	// Try to extract GID from FileInfo
	if sys := info.Sys(); sys != nil {
		if stat, ok := sys.(*syscall.Stat_t); ok {
			return stat.Gid == gid
		}
	}

	// If we can't determine group, assume not in group
	return false
}

// Ensure fuseNode implements Access interface
var _ fs.NodeAccesser = (*fuseNode)(nil)

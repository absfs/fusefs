package fusefs

import (
	"context"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// StatFSer is an optional interface that absfs implementations can implement
// to provide filesystem-level statistics.
//
// If the underlying filesystem doesn't implement this interface, reasonable
// defaults are returned.
//
// Statistics include:
//   - Total blocks and free blocks
//   - Total inodes and free inodes
//   - Block size
//   - Maximum filename length
//   - Filesystem ID
type StatFSer interface {
	// StatFS returns filesystem statistics
	//
	// Returns:
	//   - total: Total number of blocks in the filesystem
	//   - free: Number of free blocks
	//   - avail: Number of blocks available to non-root users
	//   - totalInodes: Total number of inodes
	//   - freeInodes: Number of free inodes
	//   - blockSize: Size of each block in bytes
	//   - nameMax: Maximum filename length
	//   - error: Any error encountered
	StatFS() (total, free, avail, totalInodes, freeInodes uint64, blockSize uint32, nameMax uint32, err error)
}

// Statfs returns filesystem statistics
func (n *fuseNode) Statfs(ctx context.Context, out *fuse.StatfsOut) syscall.Errno {
	n.fusefs.stats.recordOperation()

	if n.fusefs.checkUnmounting() {
		return syscall.ENOTCONN
	}

	// Check if filesystem implements StatFSer
	if statfser, ok := n.fusefs.absFS.(StatFSer); ok {
		total, free, avail, totalInodes, freeInodes, blockSize, nameMax, err := statfser.StatFS()
		if err != nil {
			n.fusefs.stats.recordError()
			return mapError(err)
		}

		out.Blocks = total
		out.Bfree = free
		out.Bavail = avail
		out.Files = totalInodes
		out.Ffree = freeInodes
		out.Bsize = blockSize
		out.NameLen = nameMax
		out.Frsize = blockSize // Fragment size same as block size
		return 0
	}

	// Filesystem doesn't implement StatFSer, return defaults
	// These are reasonable defaults for a virtual filesystem
	out.Blocks = 1024 * 1024 * 1024 // 1 billion blocks (virtual)
	out.Bfree = 1024 * 1024 * 1024  // All free (virtual)
	out.Bavail = 1024 * 1024 * 1024 // All available
	out.Files = 1024 * 1024          // 1 million inodes
	out.Ffree = 1024 * 1024          // All free
	out.Bsize = 4096                 // 4KB blocks (standard)
	out.NameLen = 255                // Standard max filename length
	out.Frsize = 4096                // Fragment size

	return 0
}

// Ensure fuseNode implements Statfs interface
var _ fs.NodeStatfser = (*fuseNode)(nil)

// Package fusefs provides a FUSE (Filesystem in Userspace) adapter for mounting
// any absfs.FileSystem as a real filesystem on Linux, macOS, and Windows.
//
// This package bridges the gap between the abstract filesystem interface
// (absfs.FileSystem) and FUSE, enabling any absfs implementation to be mounted
// as a real filesystem that can be accessed by any application on the system.
//
// # Features
//
//   - Mount any absfs.FileSystem implementation as a FUSE filesystem
//   - Full support for read and write operations
//   - Directory operations (create, remove, rename)
//   - File metadata operations (chmod, chown, chtimes, truncate)
//   - Symbolic link and hard link support (if underlying FS supports it)
//   - Attribute and directory entry caching for performance
//   - Statistics tracking (operations, bytes read/written, errors)
//   - Graceful unmounting with resource cleanup
//
// # Usage
//
// Basic usage:
//
//	// Create a filesystem (e.g., memfs, osfs, s3fs, etc.)
//	fs := memfs.NewFS()
//
//	// Configure mount options
//	opts := fusefs.DefaultMountOptions("/tmp/mymount")
//	opts.AllowOther = true
//	opts.FSName = "myfs"
//
//	// Mount the filesystem
//	fuseFS, err := fusefs.Mount(fs, opts)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer fuseFS.Unmount()
//
//	// The filesystem is now mounted and accessible at /tmp/mymount
//	// Wait for unmount signal or do other work
//	fuseFS.Wait()
//
// # Platform Support
//
//   - Linux: Requires FUSE kernel module (usually pre-installed)
//   - macOS: Requires macFUSE (https://osxfuse.github.io/)
//   - Windows: Requires WinFsp (https://winfsp.dev/) - experimental
//
// # Performance
//
// The package implements several optimizations:
//
//   - Attribute caching with configurable TTL
//   - Directory entry caching to reduce readdir calls
//   - Concurrent operation support with fine-grained locking
//   - Efficient inode management with stable inode numbers
//
// # Thread Safety
//
// All operations are thread-safe and support concurrent access from multiple
// goroutines and the FUSE kernel driver.
package fusefs

import (
	"sync/atomic"

	"github.com/absfs/absfs"
	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// FuseFS represents a mounted FUSE filesystem
type FuseFS struct {
	// absFS is the underlying abstract filesystem
	absFS absfs.FileSystem

	// opts contains mount options
	opts *MountOptions

	// server is the FUSE server instance
	server *fuse.Server

	// inodeManager manages inode allocation and caching
	inodeManager *InodeManager

	// handleTracker manages open file handles
	handleTracker *HandleTracker

	// lockManager manages file locks (flock and POSIX locks)
	lockManager *LockManager

	// stats collects filesystem statistics
	stats *statsCollector

	// unmounting indicates if the filesystem is being unmounted
	unmounting atomic.Bool

	// Root node for go-fuse
	root *fuseNode
}

// fuseNode implements the fs.InodeEmbedder interface for go-fuse v2
type fuseNode struct {
	fs.Inode
	fusefs *FuseFS
	path   string
}

// Ensure fuseNode implements required interfaces
var _ fs.NodeLookuper = (*fuseNode)(nil)
var _ fs.NodeOpener = (*fuseNode)(nil)
var _ fs.NodeReaddirer = (*fuseNode)(nil)
var _ fs.NodeGetattrer = (*fuseNode)(nil)
var _ fs.NodeCreater = (*fuseNode)(nil)
var _ fs.NodeMkdirer = (*fuseNode)(nil)
var _ fs.NodeUnlinker = (*fuseNode)(nil)
var _ fs.NodeRmdirer = (*fuseNode)(nil)
var _ fs.NodeRenamer = (*fuseNode)(nil)
var _ fs.NodeSetattrer = (*fuseNode)(nil)
var _ fs.NodeFsyncer = (*fuseNode)(nil)
var _ fs.NodeSymlinker = (*fuseNode)(nil)
var _ fs.NodeLinker = (*fuseNode)(nil)
var _ fs.NodeReadlinker = (*fuseNode)(nil)

// newFuseFS creates a new FUSE filesystem adapter
func newFuseFS(absFS absfs.FileSystem, opts *MountOptions) *FuseFS {
	fuseFS := &FuseFS{
		absFS: absFS,
		opts:  opts,
		inodeManager: NewInodeManager(
			opts.MaxCachedInodes,
			opts.MaxCachedDirs,
			opts.AttrCacheTTL,
			opts.DirCacheTTL,
		),
		handleTracker: NewHandleTracker(),
		lockManager:   NewLockManager(),
		stats:         newStatsCollector(),
	}

	fuseFS.root = &fuseNode{
		fusefs: fuseFS,
		path:   "/",
	}

	return fuseFS
}

// Stats returns a snapshot of current filesystem statistics.
//
// The returned Stats structure contains:
//   - Operations: Total number of FUSE operations performed
//   - BytesRead: Total bytes read from the filesystem
//   - BytesWritten: Total bytes written to the filesystem
//   - Errors: Total number of errors encountered
//   - OpenFiles: Number of currently open file handles
//   - Mountpoint: The path where the filesystem is mounted
//   - InodeStats: Cache statistics from the inode manager
//
// Statistics are collected atomically and this method is safe to call
// from multiple goroutines.
func (f *FuseFS) Stats() Stats {
	stats := f.stats.snapshot()
	stats.Mountpoint = f.opts.Mountpoint
	stats.OpenFiles = f.handleTracker.Count()
	stats.InodeStats = f.inodeManager.Stats()
	return stats
}

// checkUnmounting returns an error if the filesystem is unmounting
func (f *FuseFS) checkUnmounting() bool {
	return f.unmounting.Load()
}

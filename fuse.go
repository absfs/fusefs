// Package fusefs provides a FUSE adapter for mounting any absfs.FileSystem
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

// newFuseFS creates a new FUSE filesystem adapter
func newFuseFS(absFS absfs.FileSystem, opts *MountOptions) *FuseFS {
	fuseFS := &FuseFS{
		absFS:         absFS,
		opts:          opts,
		inodeManager:  NewInodeManager(),
		handleTracker: NewHandleTracker(),
		stats:         newStatsCollector(),
	}

	fuseFS.root = &fuseNode{
		fusefs: fuseFS,
		path:   "/",
	}

	return fuseFS
}

// Stats returns filesystem statistics
func (f *FuseFS) Stats() Stats {
	stats := f.stats.snapshot()
	stats.Mountpoint = f.opts.Mountpoint
	stats.OpenFiles = f.handleTracker.Count()
	return stats
}

// checkUnmounting returns an error if the filesystem is unmounting
func (f *FuseFS) checkUnmounting() bool {
	return f.unmounting.Load()
}

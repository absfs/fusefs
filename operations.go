package fusefs

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// Lookup looks up a child node by name
func (n *fuseNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	n.fusefs.stats.recordOperation()

	if n.fusefs.checkUnmounting() {
		return nil, syscall.ENOTCONN
	}

	// Build full path
	fullPath := filepath.Join(n.path, name)

	// Stat the file
	info, err := n.fusefs.absFS.Stat(fullPath)
	if err != nil {
		n.fusefs.stats.recordError()
		return nil, mapError(err)
	}

	// Get or allocate inode
	ino := n.fusefs.inodeManager.GetInode(fullPath, info)

	// Fill entry attributes
	n.fillAttr(&out.Attr, info, ino)
	out.SetEntryTimeout(n.fusefs.opts.EntryTimeout)
	out.SetAttrTimeout(n.fusefs.opts.AttrTimeout)

	// Create child node
	child := &fuseNode{
		fusefs: n.fusefs,
		path:   fullPath,
	}

	// Determine node mode
	mode := uint32(syscall.S_IFREG)
	if info.IsDir() {
		mode = syscall.S_IFDIR
	}

	// Get or create the inode
	childInode := n.NewInode(ctx, child, fs.StableAttr{
		Mode: mode,
		Ino:  ino,
	})

	return childInode, 0
}

// Getattr gets file attributes
func (n *fuseNode) Getattr(ctx context.Context, f fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	n.fusefs.stats.recordOperation()

	if n.fusefs.checkUnmounting() {
		return syscall.ENOTCONN
	}

	// Check cache first
	if cached := n.fusefs.inodeManager.GetCached(n.path); cached != nil {
		out.Attr = *cached
		out.SetTimeout(n.fusefs.opts.AttrTimeout)
		return 0
	}

	// Stat the file
	info, err := n.fusefs.absFS.Stat(n.path)
	if err != nil {
		n.fusefs.stats.recordError()
		return mapError(err)
	}

	// Get or allocate inode
	ino := n.fusefs.inodeManager.GetInode(n.path, info)

	// Fill attributes
	n.fillAttr(&out.Attr, info, ino)
	out.SetTimeout(n.fusefs.opts.AttrTimeout)

	// Cache for future lookups
	n.fusefs.inodeManager.Cache(n.path, &out.Attr)

	return 0
}

// Open opens a file
func (n *fuseNode) Open(ctx context.Context, flags uint32) (fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	n.fusefs.stats.recordOperation()

	if n.fusefs.checkUnmounting() {
		return nil, 0, syscall.ENOTCONN
	}

	// Map FUSE flags to absfs flags
	absFlags := n.mapOpenFlags(flags)

	// Open file through absfs
	file, err := n.fusefs.absFS.OpenFile(n.path, absFlags, 0)
	if err != nil {
		n.fusefs.stats.recordError()
		return nil, 0, mapError(err)
	}

	// Allocate file handle
	handle := n.fusefs.handleTracker.Add(file, absFlags, n.path)

	// Create file handle
	fileHandle := &fuseFileHandle{
		node:   n,
		handle: handle,
	}

	return fileHandle, 0, 0
}

// fuseFileHandle represents an open file handle
type fuseFileHandle struct {
	node   *fuseNode
	handle uint64
}

// Read reads data from the file
func (fh *fuseFileHandle) Read(ctx context.Context, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	fh.node.fusefs.stats.recordOperation()

	file := fh.node.fusefs.handleTracker.Get(fh.handle)
	if file == nil {
		fh.node.fusefs.stats.recordError()
		return nil, syscall.EBADF
	}

	// Seek to offset if file supports seeking
	if seeker, ok := file.(io.Seeker); ok {
		_, err := seeker.Seek(off, io.SeekStart)
		if err != nil {
			fh.node.fusefs.stats.recordError()
			return nil, mapError(err)
		}
	}

	// Read data
	n, err := file.Read(dest)
	if err != nil && err != io.EOF {
		fh.node.fusefs.stats.recordError()
		return nil, mapError(err)
	}

	fh.node.fusefs.stats.recordRead(n)
	return fuse.ReadResultData(dest[:n]), 0
}

// Write writes data to the file
func (fh *fuseFileHandle) Write(ctx context.Context, data []byte, off int64) (written uint32, errno syscall.Errno) {
	fh.node.fusefs.stats.recordOperation()

	file := fh.node.fusefs.handleTracker.Get(fh.handle)
	if file == nil {
		fh.node.fusefs.stats.recordError()
		return 0, syscall.EBADF
	}

	// Seek to offset if file supports seeking
	if seeker, ok := file.(io.Seeker); ok {
		_, err := seeker.Seek(off, io.SeekStart)
		if err != nil {
			fh.node.fusefs.stats.recordError()
			return 0, mapError(err)
		}
	}

	// Write data
	n, err := file.Write(data)
	if err != nil {
		fh.node.fusefs.stats.recordError()
		return 0, mapError(err)
	}

	fh.node.fusefs.stats.recordWrite(n)
	return uint32(n), 0
}

// Release closes the file handle
func (fh *fuseFileHandle) Release(ctx context.Context) syscall.Errno {
	fh.node.fusefs.stats.recordOperation()

	// Release any locks held by this file handle
	// The owner is derived from the handle ID for lock tracking
	fh.node.fusefs.lockManager.ReleaseOwner(fh.handle)

	return fh.node.fusefs.handleTracker.Release(fh.handle)
}

// Flush flushes cached data
func (fh *fuseFileHandle) Flush(ctx context.Context) syscall.Errno {
	fh.node.fusefs.stats.recordOperation()

	file := fh.node.fusefs.handleTracker.Get(fh.handle)
	if file == nil {
		return syscall.EBADF
	}

	// If file supports Sync, call it
	if syncer, ok := file.(interface{ Sync() error }); ok {
		if err := syncer.Sync(); err != nil {
			fh.node.fusefs.stats.recordError()
			return mapError(err)
		}
	}

	return 0
}

// Allocate pre-allocates space for the file (fallocate)
func (fh *fuseFileHandle) Allocate(ctx context.Context, off uint64, size uint64, mode uint32) syscall.Errno {
	fh.node.fusefs.stats.recordOperation()

	file := fh.node.fusefs.handleTracker.Get(fh.handle)
	if file == nil {
		fh.node.fusefs.stats.recordError()
		return syscall.EBADF
	}

	// Check if the underlying file supports allocation
	// This is typically a Linux-specific feature
	allocator, ok := file.(interface {
		Allocate(offset int64, length int64) error
	})
	if !ok {
		// If not supported, try to emulate with Truncate if mode is 0 (default allocation)
		if mode == 0 {
			truncater, ok := file.(interface{ Truncate(int64) error })
			if ok {
				info, err := file.Stat()
				if err != nil {
					fh.node.fusefs.stats.recordError()
					return mapError(err)
				}

				// Only extend the file, don't shrink it
				newSize := int64(off + size)
				if newSize > info.Size() {
					if err := truncater.Truncate(newSize); err != nil {
						fh.node.fusefs.stats.recordError()
						return mapError(err)
					}
				}
				return 0
			}
		}
		return syscall.ENOTSUP
	}

	// Call Allocate on the underlying file
	if err := allocator.Allocate(int64(off), int64(size)); err != nil {
		fh.node.fusefs.stats.recordError()
		return mapError(err)
	}

	return 0
}

// Readdir reads directory entries
func (n *fuseNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	n.fusefs.stats.recordOperation()

	if n.fusefs.checkUnmounting() {
		return nil, syscall.ENOTCONN
	}

	// Check directory cache
	if entries := n.fusefs.inodeManager.GetDirCache(n.path); entries != nil {
		return fs.NewListDirStream(n.convertDirEntries(entries)), 0
	}

	// Open directory and read entries
	dir, err := n.fusefs.absFS.Open(n.path)
	if err != nil {
		n.fusefs.stats.recordError()
		return nil, mapError(err)
	}
	defer dir.Close()

	// Read all directory entries
	infos, err := dir.Readdir(-1)
	if err != nil {
		n.fusefs.stats.recordError()
		return nil, mapError(err)
	}

	// Convert to FUSE directory entries
	fuseEntries := make([]fuse.DirEntry, 0, len(infos))
	for _, info := range infos {
		fullPath := filepath.Join(n.path, info.Name())
		ino := n.fusefs.inodeManager.GetInode(fullPath, info)

		mode := uint32(syscall.S_IFREG)
		if info.IsDir() {
			mode = syscall.S_IFDIR
		}

		fuseEntries = append(fuseEntries, fuse.DirEntry{
			Name: info.Name(),
			Ino:  ino,
			Mode: mode,
		})
	}

	// Cache directory listing
	n.fusefs.inodeManager.CacheDir(n.path, fuseEntries)

	return fs.NewListDirStream(n.convertDirEntries(fuseEntries)), 0
}

// Create creates a new file
func (n *fuseNode) Create(ctx context.Context, name string, flags uint32, mode uint32, out *fuse.EntryOut) (node *fs.Inode, fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	n.fusefs.stats.recordOperation()

	if n.fusefs.checkUnmounting() {
		return nil, nil, 0, syscall.ENOTCONN
	}

	// Build full path
	fullPath := filepath.Join(n.path, name)

	// Map FUSE flags to absfs flags
	absFlags := n.mapOpenFlags(flags) | os.O_CREATE

	// Create and open file
	file, err := n.fusefs.absFS.OpenFile(fullPath, absFlags, os.FileMode(mode))
	if err != nil {
		n.fusefs.stats.recordError()
		return nil, nil, 0, mapError(err)
	}

	// Invalidate parent directory cache
	n.fusefs.inodeManager.InvalidateDir(n.path)

	// Get file info
	info, err := n.fusefs.absFS.Stat(fullPath)
	if err != nil {
		file.Close()
		n.fusefs.stats.recordError()
		return nil, nil, 0, mapError(err)
	}

	// Allocate inode
	ino := n.fusefs.inodeManager.GetInode(fullPath, info)

	// Fill entry attributes
	n.fillAttr(&out.Attr, info, ino)
	out.SetEntryTimeout(n.fusefs.opts.EntryTimeout)
	out.SetAttrTimeout(n.fusefs.opts.AttrTimeout)

	// Create child node
	child := &fuseNode{
		fusefs: n.fusefs,
		path:   fullPath,
	}

	// Create the inode
	childInode := n.NewInode(ctx, child, fs.StableAttr{
		Mode: syscall.S_IFREG,
		Ino:  ino,
	})

	// Allocate file handle
	handle := n.fusefs.handleTracker.Add(file, absFlags, fullPath)

	// Create file handle
	fileHandle := &fuseFileHandle{
		node:   child,
		handle: handle,
	}

	return childInode, fileHandle, 0, 0
}

// Mkdir creates a new directory
func (n *fuseNode) Mkdir(ctx context.Context, name string, mode uint32, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	n.fusefs.stats.recordOperation()

	if n.fusefs.checkUnmounting() {
		return nil, syscall.ENOTCONN
	}

	// Build full path
	fullPath := filepath.Join(n.path, name)

	// Create directory
	err := n.fusefs.absFS.Mkdir(fullPath, os.FileMode(mode))
	if err != nil {
		n.fusefs.stats.recordError()
		return nil, mapError(err)
	}

	// Invalidate parent directory cache
	n.fusefs.inodeManager.InvalidateDir(n.path)

	// Get directory info
	info, err := n.fusefs.absFS.Stat(fullPath)
	if err != nil {
		n.fusefs.stats.recordError()
		return nil, mapError(err)
	}

	// Allocate inode
	ino := n.fusefs.inodeManager.GetInode(fullPath, info)

	// Fill entry attributes
	n.fillAttr(&out.Attr, info, ino)
	out.SetEntryTimeout(n.fusefs.opts.EntryTimeout)
	out.SetAttrTimeout(n.fusefs.opts.AttrTimeout)

	// Create child node
	child := &fuseNode{
		fusefs: n.fusefs,
		path:   fullPath,
	}

	// Create the inode
	childInode := n.NewInode(ctx, child, fs.StableAttr{
		Mode: syscall.S_IFDIR,
		Ino:  ino,
	})

	return childInode, 0
}

// Unlink removes a file
func (n *fuseNode) Unlink(ctx context.Context, name string) syscall.Errno {
	n.fusefs.stats.recordOperation()

	if n.fusefs.checkUnmounting() {
		return syscall.ENOTCONN
	}

	// Build full path
	fullPath := filepath.Join(n.path, name)

	// Remove file
	err := n.fusefs.absFS.Remove(fullPath)
	if err != nil {
		n.fusefs.stats.recordError()
		return mapError(err)
	}

	// Invalidate parent directory cache
	n.fusefs.inodeManager.InvalidateDir(n.path)

	return 0
}

// Rmdir removes a directory
func (n *fuseNode) Rmdir(ctx context.Context, name string) syscall.Errno {
	n.fusefs.stats.recordOperation()

	if n.fusefs.checkUnmounting() {
		return syscall.ENOTCONN
	}

	// Build full path
	fullPath := filepath.Join(n.path, name)

	// Remove directory
	err := n.fusefs.absFS.Remove(fullPath)
	if err != nil {
		n.fusefs.stats.recordError()
		return mapError(err)
	}

	// Invalidate parent directory cache
	n.fusefs.inodeManager.InvalidateDir(n.path)

	return 0
}

// Rename renames a file or directory
func (n *fuseNode) Rename(ctx context.Context, name string, newParent fs.InodeEmbedder, newName string, flags uint32) syscall.Errno {
	n.fusefs.stats.recordOperation()

	if n.fusefs.checkUnmounting() {
		return syscall.ENOTCONN
	}

	// Build paths
	oldPath := filepath.Join(n.path, name)

	newParentNode, ok := newParent.(*fuseNode)
	if !ok {
		return syscall.EINVAL
	}
	newPath := filepath.Join(newParentNode.path, newName)

	// Rename through absfs
	err := n.fusefs.absFS.Rename(oldPath, newPath)
	if err != nil {
		n.fusefs.stats.recordError()
		return mapError(err)
	}

	// Invalidate both directory caches
	n.fusefs.inodeManager.InvalidateDir(n.path)
	n.fusefs.inodeManager.InvalidateDir(newParentNode.path)

	return 0
}

// Setattr sets file attributes
func (n *fuseNode) Setattr(ctx context.Context, f fs.FileHandle, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	n.fusefs.stats.recordOperation()

	if n.fusefs.checkUnmounting() {
		return syscall.ENOTCONN
	}

	// Handle size changes (truncate)
	if sz, ok := in.GetSize(); ok {
		// If we have a file handle, truncate through it
		if f != nil {
			if fh, ok := f.(*fuseFileHandle); ok {
				file := n.fusefs.handleTracker.Get(fh.handle)
				if truncater, ok := file.(interface{ Truncate(int64) error }); ok {
					if err := truncater.Truncate(int64(sz)); err != nil {
						n.fusefs.stats.recordError()
						return mapError(err)
					}
				}
			}
		}
	}

	// Handle mode changes
	if mode, ok := in.GetMode(); ok {
		if chmodder, ok := n.fusefs.absFS.(interface {
			Chmod(string, os.FileMode) error
		}); ok {
			if err := chmodder.Chmod(n.path, os.FileMode(mode)); err != nil {
				n.fusefs.stats.recordError()
				return mapError(err)
			}
		}
	}

	// Handle time changes
	if mtime, ok := in.GetMTime(); ok {
		// Try to use Chtimes if the filesystem supports it
		// Note: We use atime = mtime for simplicity
		if err := n.fusefs.absFS.Chtimes(n.path, mtime, mtime); err != nil {
			// Ignore error if Chtimes is not supported
			_ = err
		}
	}

	// Get updated attributes
	return n.Getattr(ctx, f, out)
}

// Fsync ensures writes to the file are flushed to storage
func (n *fuseNode) Fsync(ctx context.Context, f fs.FileHandle, flags uint32) syscall.Errno {
	n.fusefs.stats.recordOperation()

	if n.fusefs.checkUnmounting() {
		return syscall.ENOTCONN
	}

	// If we have a file handle, sync through it
	if fh, ok := f.(*fuseFileHandle); ok {
		file := n.fusefs.handleTracker.Get(fh.handle)
		if file == nil {
			return syscall.EBADF
		}

		// Call Sync if the file supports it
		if syncer, ok := file.(interface{ Sync() error }); ok {
			if err := syncer.Sync(); err != nil {
				n.fusefs.stats.recordError()
				return mapError(err)
			}
		}
	}

	return 0
}

// Symlink creates a symbolic link
func (n *fuseNode) Symlink(ctx context.Context, target, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	n.fusefs.stats.recordOperation()

	if n.fusefs.checkUnmounting() {
		return nil, syscall.ENOTCONN
	}

	// Build full path
	fullPath := filepath.Join(n.path, name)

	// Check if filesystem supports symlinks
	symlinkFS, ok := n.fusefs.absFS.(interface {
		Symlink(oldname, newname string) error
	})
	if !ok {
		return nil, syscall.ENOTSUP
	}

	// Create symlink
	err := symlinkFS.Symlink(target, fullPath)
	if err != nil {
		n.fusefs.stats.recordError()
		return nil, mapError(err)
	}

	// Invalidate parent directory cache
	n.fusefs.inodeManager.InvalidateDir(n.path)

	// Get link info (using Lstat to get the link itself, not its target)
	var info os.FileInfo
	if lstatFS, ok := n.fusefs.absFS.(interface {
		Lstat(name string) (os.FileInfo, error)
	}); ok {
		info, err = lstatFS.Lstat(fullPath)
	} else {
		// Fall back to Stat if Lstat not available
		info, err = n.fusefs.absFS.Stat(fullPath)
	}

	if err != nil {
		n.fusefs.stats.recordError()
		return nil, mapError(err)
	}

	// Allocate inode
	ino := n.fusefs.inodeManager.GetInode(fullPath, info)

	// Fill entry attributes
	n.fillAttr(&out.Attr, info, ino)
	out.SetEntryTimeout(n.fusefs.opts.EntryTimeout)
	out.SetAttrTimeout(n.fusefs.opts.AttrTimeout)

	// Create child node
	child := &fuseNode{
		fusefs: n.fusefs,
		path:   fullPath,
	}

	// Create the inode
	childInode := n.NewInode(ctx, child, fs.StableAttr{
		Mode: syscall.S_IFLNK,
		Ino:  ino,
	})

	return childInode, 0
}

// Link creates a hard link
func (n *fuseNode) Link(ctx context.Context, target fs.InodeEmbedder, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	n.fusefs.stats.recordOperation()

	if n.fusefs.checkUnmounting() {
		return nil, syscall.ENOTCONN
	}

	// Get target node
	targetNode, ok := target.(*fuseNode)
	if !ok {
		return nil, syscall.EINVAL
	}

	// Build new path
	newPath := filepath.Join(n.path, name)

	// Check if filesystem supports hard links
	linkFS, ok := n.fusefs.absFS.(interface {
		Link(oldname, newname string) error
	})
	if !ok {
		return nil, syscall.ENOTSUP
	}

	// Create hard link
	err := linkFS.Link(targetNode.path, newPath)
	if err != nil {
		n.fusefs.stats.recordError()
		return nil, mapError(err)
	}

	// Invalidate parent directory cache
	n.fusefs.inodeManager.InvalidateDir(n.path)

	// Get file info
	info, err := n.fusefs.absFS.Stat(newPath)
	if err != nil {
		n.fusefs.stats.recordError()
		return nil, mapError(err)
	}

	// Use the same inode as the target (hard links share inodes)
	ino := n.fusefs.inodeManager.GetInode(newPath, info)

	// Fill entry attributes
	n.fillAttr(&out.Attr, info, ino)
	out.SetEntryTimeout(n.fusefs.opts.EntryTimeout)
	out.SetAttrTimeout(n.fusefs.opts.AttrTimeout)

	// Create child node
	child := &fuseNode{
		fusefs: n.fusefs,
		path:   newPath,
	}

	// Create the inode
	childInode := n.NewInode(ctx, child, fs.StableAttr{
		Mode: syscall.S_IFREG,
		Ino:  ino,
	})

	return childInode, 0
}

// Readlink reads the target of a symbolic link
func (n *fuseNode) Readlink(ctx context.Context) ([]byte, syscall.Errno) {
	n.fusefs.stats.recordOperation()

	if n.fusefs.checkUnmounting() {
		return nil, syscall.ENOTCONN
	}

	// Check if filesystem supports reading symlinks
	readlinkFS, ok := n.fusefs.absFS.(interface {
		Readlink(name string) (string, error)
	})
	if !ok {
		return nil, syscall.ENOTSUP
	}

	// Read the symlink target
	target, err := readlinkFS.Readlink(n.path)
	if err != nil {
		n.fusefs.stats.recordError()
		return nil, mapError(err)
	}

	return []byte(target), 0
}

// Helper methods

// fillAttr fills a FUSE Attr structure from os.FileInfo
func (n *fuseNode) fillAttr(attr *fuse.Attr, info os.FileInfo, ino uint64) {
	attr.Ino = ino
	attr.Size = uint64(info.Size())
	attr.Mode = uint32(info.Mode())
	attr.Mtime = uint64(info.ModTime().Unix())
	attr.Mtimensec = uint32(info.ModTime().Nanosecond())

	// Set UID/GID from options if provided
	if n.fusefs.opts.UID != 0 {
		attr.Uid = n.fusefs.opts.UID
	} else {
		attr.Uid = uint32(os.Getuid())
	}

	if n.fusefs.opts.GID != 0 {
		attr.Gid = n.fusefs.opts.GID
	} else {
		attr.Gid = uint32(os.Getgid())
	}

	// Set block information
	blocks := (attr.Size + 511) / 512
	attr.Blocks = blocks
	attr.Blksize = 4096
}

// mapOpenFlags maps FUSE open flags to absfs flags
func (n *fuseNode) mapOpenFlags(flags uint32) int {
	absFlags := 0

	// Read/Write mode
	if flags&syscall.O_WRONLY != 0 {
		absFlags |= os.O_WRONLY
	} else if flags&syscall.O_RDWR != 0 {
		absFlags |= os.O_RDWR
	} else {
		absFlags |= os.O_RDONLY
	}

	// Additional flags
	if flags&syscall.O_APPEND != 0 {
		absFlags |= os.O_APPEND
	}
	if flags&syscall.O_CREAT != 0 {
		absFlags |= os.O_CREATE
	}
	if flags&syscall.O_TRUNC != 0 {
		absFlags |= os.O_TRUNC
	}
	if flags&syscall.O_EXCL != 0 {
		absFlags |= os.O_EXCL
	}

	return absFlags
}

// convertDirEntries converts fuse.DirEntry to fs.DirEntry
func (n *fuseNode) convertDirEntries(entries []fuse.DirEntry) []fuse.DirEntry {
	return entries
}

// Ensure fuseFileHandle implements required interfaces
var _ fs.FileHandle = (*fuseFileHandle)(nil)
var _ fs.FileReader = (*fuseFileHandle)(nil)
var _ fs.FileWriter = (*fuseFileHandle)(nil)
var _ fs.FileReleaser = (*fuseFileHandle)(nil)
var _ fs.FileFlusher = (*fuseFileHandle)(nil)
var _ fs.FileAllocater = (*fuseFileHandle)(nil)

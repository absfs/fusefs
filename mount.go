package fusefs

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/absfs/absfs"
	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// Mount mounts an absfs.FileSystem at the specified mountpoint
func Mount(absFS absfs.FileSystem, opts *MountOptions) (*FuseFS, error) {
	if opts == nil {
		return nil, fmt.Errorf("mount options cannot be nil")
	}

	if opts.Mountpoint == "" {
		return nil, fmt.Errorf("mountpoint cannot be empty")
	}

	// Create mountpoint if it doesn't exist
	if err := os.MkdirAll(opts.Mountpoint, 0755); err != nil {
		return nil, fmt.Errorf("failed to create mountpoint: %w", err)
	}

	// Check if mountpoint is empty
	entries, err := os.ReadDir(opts.Mountpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to read mountpoint: %w", err)
	}
	if len(entries) > 0 {
		return nil, fmt.Errorf("mountpoint is not empty")
	}

	// Create FUSE filesystem
	fuseFS := newFuseFS(absFS, opts)

	// Build FUSE mount options
	fuseOpts := &fs.Options{
		MountOptions: fuse.MountOptions{
			Name:          opts.FSName,
			FsName:        opts.FSName,
			DirectMount:   false,
			Debug:         opts.Debug,
			AllowOther:    opts.AllowOther,
			Options:       opts.Options,
			MaxBackground: 12,
		},
		AttrTimeout:  &opts.AttrTimeout,
		EntryTimeout: &opts.EntryTimeout,
	}

	// Add read-only option if specified
	if opts.ReadOnly {
		fuseOpts.MountOptions.Options = append(fuseOpts.MountOptions.Options, "ro")
	}

	// Add default_permissions option if specified
	if opts.DefaultPermissions {
		fuseOpts.MountOptions.Options = append(fuseOpts.MountOptions.Options, "default_permissions")
	}

	// Add allow_root option if specified
	if opts.AllowRoot {
		fuseOpts.MountOptions.Options = append(fuseOpts.MountOptions.Options, "allow_root")
	}

	// Mount the filesystem
	server, err := fs.Mount(opts.Mountpoint, fuseFS.root, fuseOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to mount filesystem: %w", err)
	}

	fuseFS.server = server

	return fuseFS, nil
}

// Unmount unmounts the filesystem
func (f *FuseFS) Unmount() error {
	// Signal all operations to complete
	f.unmounting.Store(true)

	// Close all open file handles
	f.handleTracker.CloseAll()

	// Clear caches
	f.inodeManager.Clear()

	// Unmount FUSE filesystem
	if f.server != nil {
		return f.server.Unmount()
	}

	return nil
}

// Wait blocks until the filesystem is unmounted
func (f *FuseFS) Wait() error {
	if f.server == nil {
		return fmt.Errorf("filesystem not mounted")
	}

	f.server.Wait()
	return nil
}

// MountAndWait mounts a filesystem and waits for it to be unmounted
func MountAndWait(absFS absfs.FileSystem, opts *MountOptions) error {
	fuseFS, err := Mount(absFS, opts)
	if err != nil {
		return err
	}

	return fuseFS.Wait()
}

// IsMounted checks if a directory is a FUSE mountpoint
func IsMounted(path string) (bool, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false, err
	}

	var stat syscall.Stat_t
	if err := syscall.Stat(absPath, &stat); err != nil {
		return false, err
	}

	// Get parent directory stats
	parent := filepath.Dir(absPath)
	var parentStat syscall.Stat_t
	if err := syscall.Stat(parent, &parentStat); err != nil {
		return false, err
	}

	// If device IDs differ, it's a mount point
	return stat.Dev != parentStat.Dev, nil
}

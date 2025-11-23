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

// Mount mounts an absfs.FileSystem at the specified mountpoint and returns a
// FuseFS instance that can be used to unmount and query statistics.
//
// The function will:
//  1. Create the mountpoint directory if it doesn't exist
//  2. Verify the mountpoint is empty
//  3. Initialize the FUSE adapter with inode and handle tracking
//  4. Mount the filesystem using go-fuse v2 library
//
// The returned FuseFS instance should be unmounted when done using Unmount()
// or the filesystem can be left mounted and controlled externally.
//
// Example:
//
//	fs := memfs.NewFS()
//	opts := fusefs.DefaultMountOptions("/tmp/mymount")
//	fuseFS, err := fusefs.Mount(fs, opts)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer fuseFS.Unmount()
//
// Errors:
//   - Returns error if mountpoint is not empty
//   - Returns error if mount options are invalid
//   - Returns error if FUSE mount fails (e.g., FUSE not available, permissions)
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
			MaxReadAhead:  int(opts.MaxReadahead),
			MaxWrite:      int(opts.MaxWrite),
		},
		AttrTimeout:  &opts.AttrTimeout,
		EntryTimeout: &opts.EntryTimeout,
	}

	// Add read-only option if specified
	if opts.ReadOnly {
		fuseOpts.MountOptions.Options = append(fuseOpts.MountOptions.Options, "ro")
	}

	// Add DirectIO option if specified
	if opts.DirectIO {
		fuseOpts.MountOptions.Options = append(fuseOpts.MountOptions.Options, "direct_io")
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

// Unmount gracefully unmounts the filesystem and cleans up resources.
//
// This method:
//  1. Signals all pending operations to complete
//  2. Closes all open file handles
//  3. Clears all caches (inode, attribute, directory)
//  4. Unmounts the FUSE filesystem
//
// It is safe to call Unmount multiple times; subsequent calls will be no-ops.
//
// Example:
//
//	fuseFS, _ := fusefs.Mount(fs, opts)
//	defer fuseFS.Unmount()
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

// Wait blocks until the filesystem is unmounted externally (e.g., via fusermount -u)
// or until Unmount() is called from another goroutine.
//
// This is useful for keeping a mount alive until the user manually unmounts it:
//
//	fuseFS, _ := fusefs.Mount(fs, opts)
//	defer fuseFS.Unmount()
//	log.Println("Filesystem mounted, press Ctrl+C to unmount")
//	fuseFS.Wait()
func (f *FuseFS) Wait() error {
	if f.server == nil {
		return fmt.Errorf("filesystem not mounted")
	}

	f.server.Wait()
	return nil
}

// MountAndWait is a convenience function that mounts a filesystem and waits
// for it to be unmounted. This is equivalent to calling Mount() followed by Wait().
//
// This is the simplest way to mount a filesystem and keep it alive:
//
//	fs := memfs.NewFS()
//	opts := fusefs.DefaultMountOptions("/tmp/mymount")
//	if err := fusefs.MountAndWait(fs, opts); err != nil {
//	    log.Fatal(err)
//	}
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

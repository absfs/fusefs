package fusefs

import (
	"time"
)

// MountOptions configures the FUSE mount behavior and performance characteristics.
//
// Use DefaultMountOptions() to get a set of sensible defaults, then customize
// as needed for your use case.
type MountOptions struct {
	// Mountpoint is the directory where the filesystem will be mounted
	Mountpoint string

	// ReadOnly mounts the filesystem in read-only mode
	ReadOnly bool

	// AllowOther allows other users to access the mounted filesystem
	// Requires 'user_allow_other' in /etc/fuse.conf on Linux
	AllowOther bool

	// AllowRoot allows root to access the mounted filesystem
	AllowRoot bool

	// DefaultPermissions enables kernel permission checking
	DefaultPermissions bool

	// UID/GID override file ownership
	UID uint32
	GID uint32

	// DirectIO disables page cache for reads/writes
	DirectIO bool

	// MaxReadahead sets maximum readahead (bytes)
	MaxReadahead uint32

	// MaxWrite sets maximum write size (bytes)
	MaxWrite uint32

	// AsyncRead enables asynchronous reads
	AsyncRead bool

	// AttrTimeout sets attribute cache timeout
	AttrTimeout time.Duration

	// EntryTimeout sets directory entry cache timeout
	EntryTimeout time.Duration

	// FSName is the name shown in mount table
	FSName string

	// Options contains additional FUSE options
	Options []string

	// Debug enables debug logging
	Debug bool

	// Cache configuration for user-space caches
	// These control the behavior of internal caches, separate from kernel FUSE caching

	// AttrCacheTTL sets how long file attributes are cached in user-space
	// before being re-fetched from the underlying filesystem.
	// Default: 5 seconds
	AttrCacheTTL time.Duration

	// DirCacheTTL sets how long directory listings are cached in user-space.
	// Default: 5 seconds
	DirCacheTTL time.Duration

	// MaxCachedInodes limits the number of inodes kept in the cache.
	// When exceeded, least recently used entries are evicted.
	// Default: 10000, set to 0 for unlimited (not recommended)
	MaxCachedInodes int

	// MaxCachedDirs limits the number of directory listings kept in cache.
	// When exceeded, least recently used entries are evicted.
	// Default: 1000, set to 0 for unlimited (not recommended)
	MaxCachedDirs int
}

// DefaultMountOptions returns mount options with sensible defaults for general use.
//
// Default values:
//   - AttrTimeout: 1 second (balance between consistency and performance)
//   - EntryTimeout: 1 second
//   - MaxReadahead: 128KB (good for sequential reads)
//   - MaxWrite: 128KB
//   - DefaultPermissions: true (kernel enforces permissions)
//   - AsyncRead: true (better performance)
//
// Customize these values based on your use case:
//   - For remote filesystems: increase timeouts to reduce network calls
//   - For local filesystems: decrease timeouts for faster updates
//   - For sequential workloads: increase MaxReadahead
//   - For random access: decrease MaxReadahead
func DefaultMountOptions(mountpoint string) *MountOptions {
	return &MountOptions{
		Mountpoint:         mountpoint,
		ReadOnly:           false,
		AllowOther:         false,
		AllowRoot:          false,
		DefaultPermissions: true,
		DirectIO:           false,
		MaxReadahead:       128 * 1024, // 128KB
		MaxWrite:           128 * 1024, // 128KB
		AsyncRead:          true,
		AttrTimeout:        1 * time.Second,
		EntryTimeout:       1 * time.Second,
		FSName:             "fusefs",
		Debug:              false,
		AttrCacheTTL:       5 * time.Second,
		DirCacheTTL:        5 * time.Second,
		MaxCachedInodes:    10000,
		MaxCachedDirs:      1000,
	}
}

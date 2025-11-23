# fusefs

FUSE adapter for mounting any `absfs.FileSystem` as a real filesystem on Linux, macOS, and Windows.

## Overview

`fusefs` provides a bridge between the abstract filesystem interface (`absfs.FileSystem`) and FUSE (Filesystem in Userspace), enabling any absfs implementation to be mounted as a real filesystem that can be accessed by any application on the system.

This allows:
- Mounting remote filesystems (S3, WebDAV, SFTP) as local drives
- Creating encrypted filesystems accessible to all applications
- Composing complex filesystem stacks and exposing them system-wide
- Testing and debugging absfs implementations with real-world tools
- Building custom filesystems without kernel module development

## Architecture

### High-Level Design

```
┌─────────────────────────────────────────────────────────┐
│                   User Applications                      │
│         (ls, cat, vim, IDEs, file managers)             │
└─────────────────────────────────────────────────────────┘
                           │
                           ├── POSIX File Operations
                           ↓
┌─────────────────────────────────────────────────────────┐
│                    Kernel VFS Layer                      │
└─────────────────────────────────────────────────────────┘
                           │
                           ├── FUSE Protocol
                           ↓
┌─────────────────────────────────────────────────────────┐
│                   fusefs Adapter                         │
│  ┌────────────────────────────────────────────────┐    │
│  │         FUSE Operation Handlers                 │    │
│  │  (Lookup, GetAttr, Open, Read, Write, etc.)    │    │
│  └────────────────────────────────────────────────┘    │
│                           │                              │
│  ┌────────────────────────────────────────────────┐    │
│  │         Inode Manager                          │    │
│  │  - Path ↔ Inode mapping                       │    │
│  │  - Inode ↔ FileInfo caching                   │    │
│  │  - Directory entry caching                     │    │
│  └────────────────────────────────────────────────┘    │
│                           │                              │
│  ┌────────────────────────────────────────────────┐    │
│  │         File Handle Tracker                    │    │
│  │  - Open file handle management                 │    │
│  │  - Handle ↔ absfs.File mapping                │    │
│  │  - Reference counting                          │    │
│  └────────────────────────────────────────────────┘    │
│                           │                              │
│  ┌────────────────────────────────────────────────┐    │
│  │         Error Mapper                           │    │
│  │  - absfs.Error → FUSE errno                   │    │
│  │  - Platform-specific error codes               │    │
│  └────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────┘
                           │
                           ├── absfs.FileSystem Interface
                           ↓
┌─────────────────────────────────────────────────────────┐
│              Any absfs.FileSystem Implementation         │
│   (memfs, s3fs, encryptfs, composed stacks, etc.)       │
└─────────────────────────────────────────────────────────┘
```

### Component Responsibilities

#### FUSE Operation Handlers
Maps FUSE protocol operations to absfs.FileSystem methods:
- `Lookup()` → `Stat()`
- `GetAttr()` → `Stat()`
- `Open()` → `Open()`, `OpenFile()`
- `Read()` → `File.Read()`
- `Write()` → `File.Write()`
- `Readdir()` → `ReadDir()`
- `Mkdir()` → `Mkdir()`
- `Rmdir()` → `Remove()`
- `Unlink()` → `Remove()`
- `Rename()` → `Rename()`
- `Chmod()` → `Chmod()`
- `Chown()` → `Chown()`
- `Utimens()` → `Chtimes()`
- `Setxattr()` → Extended attribute interface (if supported)
- `Getxattr()` → Extended attribute interface (if supported)

#### Inode Manager
Manages the mapping between filesystem paths and inode numbers:
- Assigns stable inode numbers to paths
- Caches FileInfo for recently accessed paths
- Implements directory entry caching for performance
- Handles inode generation numbers for deleted/recreated files
- Thread-safe concurrent access

#### File Handle Tracker
Manages open file handles and their lifecycle:
- Allocates unique file handle IDs
- Maintains handle → `absfs.File` mapping
- Implements reference counting for shared handles
- Handles cleanup on close/unmount
- Tracks read/write position if needed

#### Error Mapper
Translates absfs errors to appropriate FUSE error codes:
- `absfs.ErrNotExist` → `syscall.ENOENT`
- `absfs.ErrExist` → `syscall.EEXIST`
- `absfs.ErrPermission` → `syscall.EACCES`
- `absfs.ErrIsDir` → `syscall.EISDIR`
- `absfs.ErrNotDir` → `syscall.ENOTDIR`
- `absfs.ErrNotEmpty` → `syscall.ENOTEMPTY`
- Platform-specific mappings for Windows/macOS

## FUSE Library Integration

### Primary Library: github.com/hanwen/go-fuse/v2

Selected for:
- Active maintenance and wide adoption
- High performance with low-level API
- Support for FUSE 7.8+ protocol features
- Good macOS (macFUSE) compatibility
- Clean API design

#### Alternative Libraries Considered

**bazil.org/fuse**
- Pros: Higher-level API, simpler to use
- Cons: Less active maintenance, lower performance
- Status: Not selected due to maintenance concerns

**github.com/winfsp/cgofuse**
- Pros: True cross-platform (includes native WinFsp support)
- Cons: CGo dependency, more complex build
- Status: Consider for future Windows-specific implementation

## Platform Support

### Linux
- **Requirements**: FUSE kernel module (included in modern kernels)
- **User-space**: libfuse or libfuse3
- **Installation**: Usually pre-installed; `apt install fuse` or `yum install fuse`
- **Protocol**: FUSE 7.8+
- **Status**: Primary platform, full support

### macOS
- **Requirements**: macFUSE (successor to osxfuse)
- **Installation**: Download from https://osxfuse.github.io/
- **Protocol**: FUSE 7.8 compatible
- **Limitations**:
  - Requires kernel extension (System Preferences → Security)
  - Some extended attributes differ from Linux
  - Performance characteristics vary
- **Status**: Supported

### Windows
- **Requirements**: WinFsp (Windows File System Proxy)
- **Installation**: Download from https://winfsp.dev/
- **Protocol**: FUSE-compatible API
- **Limitations**:
  - Different permission model (ACLs vs Unix modes)
  - Path separator differences
  - Some POSIX features unsupported
- **Status**: Future support (requires WinFsp-specific adapter)

## Implementation Details

### Operation Mapping

#### File Operations

**Open**
```go
func (fs *FuseFS) Open(path string, flags uint32) (fh uint64, errno syscall.Errno) {
    // Map FUSE flags to absfs flags
    absFlags := mapOpenFlags(flags)

    // Open file through absfs
    file, err := fs.absFS.OpenFile(path, absFlags, 0)
    if err != nil {
        return 0, mapError(err)
    }

    // Allocate file handle
    fh = fs.handleTracker.Add(file)
    return fh, 0
}
```

**Read**
```go
func (fs *FuseFS) Read(fh uint64, buf []byte, off int64) (int, syscall.Errno) {
    file := fs.handleTracker.Get(fh)
    if file == nil {
        return 0, syscall.EBADF
    }

    // Seek if necessary (absfs.File may not track position)
    if seeker, ok := file.(io.Seeker); ok {
        _, err := seeker.Seek(off, io.SeekStart)
        if err != nil {
            return 0, mapError(err)
        }
    }

    n, err := file.Read(buf)
    return n, mapError(err)
}
```

**Write**
```go
func (fs *FuseFS) Write(fh uint64, data []byte, off int64) (int, syscall.Errno) {
    file := fs.handleTracker.Get(fh)
    if file == nil {
        return 0, syscall.EBADF
    }

    // Seek to write position
    if seeker, ok := file.(io.Seeker); ok {
        _, err := seeker.Seek(off, io.SeekStart)
        if err != nil {
            return 0, mapError(err)
        }
    }

    n, err := file.Write(data)
    return n, mapError(err)
}
```

#### Metadata Operations

**Lookup/GetAttr**
```go
func (fs *FuseFS) GetAttr(path string) (*fuse.Attr, syscall.Errno) {
    // Check inode cache first
    if attr := fs.inodeManager.GetCached(path); attr != nil {
        return attr, 0
    }

    // Stat through absfs
    info, err := fs.absFS.Stat(path)
    if err != nil {
        return nil, mapError(err)
    }

    // Convert to FUSE attributes
    attr := fileInfoToAttr(info)

    // Assign/retrieve inode
    attr.Ino = fs.inodeManager.GetInode(path, info)

    // Cache for future lookups
    fs.inodeManager.Cache(path, attr)

    return attr, 0
}
```

**Readdir**
```go
func (fs *FuseFS) ReadDir(path string) ([]fuse.DirEntry, syscall.Errno) {
    // Check directory cache
    if entries := fs.inodeManager.GetDirCache(path); entries != nil {
        return entries, 0
    }

    // Read directory through absfs
    infos, err := fs.absFS.ReadDir(path)
    if err != nil {
        return nil, mapError(err)
    }

    // Convert to FUSE directory entries
    entries := make([]fuse.DirEntry, len(infos))
    for i, info := range infos {
        fullPath := fs.absFS.Join(path, info.Name())
        entries[i] = fuse.DirEntry{
            Name: info.Name(),
            Ino:  fs.inodeManager.GetInode(fullPath, info),
            Mode: fileInfoToMode(info),
        }
    }

    // Cache directory listing
    fs.inodeManager.CacheDir(path, entries)

    return entries, 0
}
```

### Inode Management

**Inode Allocation Strategy**
```go
type InodeManager struct {
    mu          sync.RWMutex
    pathToInode map[string]uint64
    inodeToPath map[uint64]string
    inodeToInfo map[uint64]*fuse.Attr
    nextInode   uint64
    dirCache    map[string]*dirCacheEntry
}

func (im *InodeManager) GetInode(path string, info absfs.FileInfo) uint64 {
    im.mu.Lock()
    defer im.mu.Unlock()

    // Check if path already has inode
    if ino, exists := im.pathToInode[path]; exists {
        // Verify file hasn't changed (check mtime, size)
        if im.isSameFile(ino, info) {
            return ino
        }
        // File changed, allocate new inode
        delete(im.inodeToPath, ino)
    }

    // Allocate new inode
    im.nextInode++
    ino := im.nextInode

    im.pathToInode[path] = ino
    im.inodeToPath[ino] = path

    return ino
}
```

**Directory Caching**
```go
type dirCacheEntry struct {
    entries   []fuse.DirEntry
    timestamp time.Time
    ttl       time.Duration
}

func (im *InodeManager) CacheDir(path string, entries []fuse.DirEntry) {
    im.mu.Lock()
    defer im.mu.Unlock()

    im.dirCache[path] = &dirCacheEntry{
        entries:   entries,
        timestamp: time.Now(),
        ttl:       5 * time.Second, // Configurable
    }
}

func (im *InodeManager) GetDirCache(path string) []fuse.DirEntry {
    im.mu.RLock()
    defer im.mu.RUnlock()

    entry := im.dirCache[path]
    if entry == nil {
        return nil
    }

    // Check if cache expired
    if time.Since(entry.timestamp) > entry.ttl {
        return nil
    }

    return entry.entries
}
```

### File Handle Tracking

```go
type HandleTracker struct {
    mu          sync.RWMutex
    handles     map[uint64]*handleEntry
    nextHandle  uint64
}

type handleEntry struct {
    file     absfs.File
    refCount int32
    flags    int
}

func (ht *HandleTracker) Add(file absfs.File, flags int) uint64 {
    ht.mu.Lock()
    defer ht.mu.Unlock()

    ht.nextHandle++
    fh := ht.nextHandle

    ht.handles[fh] = &handleEntry{
        file:     file,
        refCount: 1,
        flags:    flags,
    }

    return fh
}

func (ht *HandleTracker) Release(fh uint64) error {
    ht.mu.Lock()
    defer ht.mu.Unlock()

    entry := ht.handles[fh]
    if entry == nil {
        return syscall.EBADF
    }

    entry.refCount--
    if entry.refCount == 0 {
        err := entry.file.Close()
        delete(ht.handles, fh)
        return err
    }

    return nil
}
```

### Permission Handling

**Unix Permission Mapping**
```go
func fileInfoToMode(info absfs.FileInfo) uint32 {
    mode := uint32(info.Mode().Perm())

    if info.IsDir() {
        mode |= syscall.S_IFDIR
    } else {
        mode |= syscall.S_IFREG
    }

    // Handle symbolic links if supported
    if info.Mode()&os.ModeSymlink != 0 {
        mode |= syscall.S_IFLNK
    }

    return mode
}

func (fs *FuseFS) Access(path string, mask uint32) syscall.Errno {
    info, err := fs.absFS.Stat(path)
    if err != nil {
        return mapError(err)
    }

    // Check permissions based on UID/GID
    // Simplified: assumes current user
    perm := info.Mode().Perm()

    if mask&syscall.R_OK != 0 && perm&0400 == 0 {
        return syscall.EACCES
    }
    if mask&syscall.W_OK != 0 && perm&0200 == 0 {
        return syscall.EACCES
    }
    if mask&syscall.X_OK != 0 && perm&0100 == 0 {
        return syscall.EACCES
    }

    return 0
}
```

**UID/GID Handling**
```go
type MountOptions struct {
    UID uint32  // Override file owner
    GID uint32  // Override file group
    // ...
}

func (fs *FuseFS) populateAttr(attr *fuse.Attr, info absfs.FileInfo) {
    // Use mount options if provided, otherwise extract from FileInfo
    if fs.opts.UID != 0 {
        attr.Uid = fs.opts.UID
    } else if sysInfo := extractSysInfo(info); sysInfo != nil {
        attr.Uid = sysInfo.UID
    }

    if fs.opts.GID != 0 {
        attr.Gid = fs.opts.GID
    } else if sysInfo := extractSysInfo(info); sysInfo != nil {
        attr.Gid = sysInfo.GID
    }
}
```

### Extended Attributes

**Interface Definition**
```go
// XAttrFS extends absfs.FileSystem with extended attribute support
type XAttrFS interface {
    absfs.FileSystem
    GetXAttr(path string, name string) ([]byte, error)
    SetXAttr(path string, name string, value []byte, flags int) error
    ListXAttr(path string) ([]string, error)
    RemoveXAttr(path string, name string) error
}
```

**FUSE Integration**
```go
func (fs *FuseFS) GetXAttr(path string, attr string) ([]byte, syscall.Errno) {
    xattrFS, ok := fs.absFS.(XAttrFS)
    if !ok {
        return nil, syscall.ENOTSUP
    }

    value, err := xattrFS.GetXAttr(path, attr)
    if err != nil {
        return nil, mapError(err)
    }

    return value, 0
}
```

### Error Code Mapping

```go
func mapError(err error) syscall.Errno {
    if err == nil {
        return 0
    }

    // Handle standard absfs errors
    switch {
    case errors.Is(err, absfs.ErrNotExist):
        return syscall.ENOENT
    case errors.Is(err, absfs.ErrExist):
        return syscall.EEXIST
    case errors.Is(err, absfs.ErrPermission):
        return syscall.EACCES
    case errors.Is(err, absfs.ErrIsDir):
        return syscall.EISDIR
    case errors.Is(err, absfs.ErrNotDir):
        return syscall.ENOTDIR
    case errors.Is(err, absfs.ErrNotEmpty):
        return syscall.ENOTEMPTY
    case errors.Is(err, absfs.ErrClosed):
        return syscall.EBADF
    case errors.Is(err, absfs.ErrInvalid):
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
```

### Unmount Handling

```go
func (fs *FuseFS) Unmount() error {
    // Signal all operations to complete
    fs.unmounting.Store(true)

    // Close all open file handles
    fs.handleTracker.CloseAll()

    // Clear caches
    fs.inodeManager.Clear()

    // Unmount FUSE filesystem
    return fs.server.Unmount()
}

func (fs *FuseFS) checkUnmounting() syscall.Errno {
    if fs.unmounting.Load() {
        return syscall.ENOTCONN
    }
    return 0
}
```

## Configuration Options

### Mount Options

```go
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

    // CacheTimeout sets attribute/directory cache timeout
    AttrTimeout time.Duration
    EntryTimeout time.Duration

    // Name shown in mount table
    FSName string

    // Additional FUSE options
    Options []string
}
```

### Caching Configuration

```go
type CacheConfig struct {
    // Attribute cache duration
    AttrCacheTTL time.Duration

    // Directory listing cache duration
    DirCacheTTL time.Duration

    // Maximum cached directory entries
    MaxDirCacheEntries int

    // Inode cache size
    MaxInodes int

    // Enable kernel page cache
    EnablePageCache bool
}
```

## Implementation Phases

### Phase 1: Core FUSE Adapter
- Basic FUSE filesystem structure
- Inode manager implementation
- File handle tracker
- Error mapper
- Essential operations:
  - Lookup, GetAttr
  - Open, Read, Release
  - Readdir
- Mount/unmount handling
- Linux support

### Phase 2: Full Operation Support
- Write operations (Write, Create)
- Directory operations (Mkdir, Rmdir)
- File manipulation (Rename, Unlink, Link)
- Metadata operations (Chmod, Chown, Utimens)
- Truncate/Fallocate
- Flush/Fsync

### Phase 3: Performance Optimization
- Attribute caching
- Directory entry caching
- Readahead optimization
- Parallel operations
- Memory pooling for buffers
- Inode cache eviction strategies

### Phase 4: Advanced Features
- Extended attributes (if absfs supports)
- Symbolic link operations
- File locking (Flock, POSIX locks)
- Poll/Select support
- Direct I/O mode
- Ioctl passthrough (if applicable)

### Phase 5: Platform Extensions
- macOS optimization (macFUSE-specific)
- Windows support (WinFsp integration)
- Platform-specific mount options
- Cross-platform testing

### Phase 6: Production Readiness
- Comprehensive error handling
- Resource cleanup on errors
- Graceful shutdown
- Logging and diagnostics
- Performance monitoring
- Documentation and examples

## Usage Examples

### Basic: Mount memfs

```go
package main

import (
    "log"
    "os"
    "os/signal"
    "syscall"

    "github.com/absfs/absfs"
    "github.com/absfs/fusefs"
    "github.com/absfs/memfs"
)

func main() {
    // Create in-memory filesystem
    mfs := memfs.NewFS()

    // Create some initial content
    mfs.Mkdir("/documents", 0755)
    absfs.WriteFile(mfs, "/documents/hello.txt", []byte("Hello, FUSE!"), 0644)

    // Mount filesystem
    opts := &fusefs.MountOptions{
        Mountpoint: "/tmp/memfs-mount",
        AllowOther: true,
        FSName:     "memfs",
    }

    fuseFS, err := fusefs.Mount(mfs, opts)
    if err != nil {
        log.Fatal(err)
    }
    defer fuseFS.Unmount()

    log.Printf("Mounted at %s", opts.Mountpoint)

    // Wait for interrupt
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
    <-sigCh

    log.Println("Unmounting...")
}
```

### Mount S3 Bucket

```go
package main

import (
    "log"
    "os"
    "os/signal"
    "syscall"

    "github.com/absfs/fusefs"
    "github.com/absfs/s3fs"
)

func main() {
    // Create S3 filesystem
    s3FS, err := s3fs.NewFS(s3fs.Options{
        Bucket: "my-bucket",
        Region: "us-west-2",
        Prefix: "data/",
    })
    if err != nil {
        log.Fatal(err)
    }

    // Mount with caching for better performance
    opts := &fusefs.MountOptions{
        Mountpoint:   "/mnt/s3bucket",
        ReadOnly:     false,
        FSName:       "s3fs",
        AttrTimeout:  5 * time.Second,
        EntryTimeout: 5 * time.Second,
        MaxReadahead: 128 * 1024,
    }

    fuseFS, err := fusefs.Mount(s3FS, opts)
    if err != nil {
        log.Fatal(err)
    }
    defer fuseFS.Unmount()

    log.Printf("S3 bucket mounted at %s", opts.Mountpoint)

    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
    <-sigCh
}
```

### Mount Encrypted Filesystem

```go
package main

import (
    "log"
    "os"

    "github.com/absfs/encryptfs"
    "github.com/absfs/fusefs"
    "github.com/absfs/osfs"
)

func main() {
    // Create base filesystem
    baseFS := osfs.NewFS()

    // Wrap with encryption
    key := []byte("your-32-byte-encryption-key-")
    encFS, err := encryptfs.New(baseFS, encryptfs.Options{
        Key:       key,
        Algorithm: encryptfs.AES256GCM,
    })
    if err != nil {
        log.Fatal(err)
    }

    // Mount encrypted filesystem
    opts := &fusefs.MountOptions{
        Mountpoint:         "/mnt/encrypted",
        DefaultPermissions: true,
        FSName:             "encryptfs",
    }

    fuseFS, err := fusefs.Mount(encFS, opts)
    if err != nil {
        log.Fatal(err)
    }
    defer fuseFS.Unmount()

    log.Println("Encrypted filesystem mounted")

    // Files written through /mnt/encrypted will be encrypted on disk
    select {}
}
```

### Mount Composed Stack

```go
package main

import (
    "log"
    "time"

    "github.com/absfs/cachefs"
    "github.com/absfs/encryptfs"
    "github.com/absfs/fusefs"
    "github.com/absfs/metricsfs"
    "github.com/absfs/s3fs"
)

func main() {
    // Build filesystem stack: S3 → Encryption → Cache → Metrics

    // 1. Base: S3 storage
    s3FS, _ := s3fs.NewFS(s3fs.Options{
        Bucket: "my-bucket",
        Region: "us-west-2",
    })

    // 2. Add encryption
    key := []byte("your-32-byte-encryption-key-")
    encFS, _ := encryptfs.New(s3FS, encryptfs.Options{
        Key: key,
    })

    // 3. Add caching layer
    cacheFS, _ := cachefs.New(encFS, cachefs.Options{
        MaxSize: 1024 * 1024 * 1024, // 1GB cache
        TTL:     10 * time.Minute,
    })

    // 4. Add metrics
    metricsFS := metricsfs.New(cacheFS)

    // Mount the complete stack
    opts := &fusefs.MountOptions{
        Mountpoint: "/mnt/secure-s3",
        FSName:     "secure-s3",
    }

    fuseFS, err := fusefs.Mount(metricsFS, opts)
    if err != nil {
        log.Fatal(err)
    }
    defer fuseFS.Unmount()

    log.Println("Composed filesystem mounted")

    // Monitor metrics
    go func() {
        ticker := time.NewTicker(30 * time.Second)
        for range ticker.C {
            stats := metricsFS.Stats()
            log.Printf("Ops: %d, Bytes Read: %d, Bytes Written: %d",
                stats.Operations, stats.BytesRead, stats.BytesWritten)
        }
    }()

    select {}
}
```

### CLI Tool

```go
// cmd/fusefs/main.go
package main

import (
    "flag"
    "log"
    "os"
    "os/signal"
    "syscall"

    "github.com/absfs/fusefs"
    "github.com/absfs/memfs"
    "github.com/absfs/osfs"
    "github.com/absfs/s3fs"
)

func main() {
    fsType := flag.String("type", "os", "Filesystem type (os, mem, s3)")
    mountpoint := flag.String("mount", "", "Mount point (required)")
    source := flag.String("source", "", "Source path or configuration")
    readOnly := flag.Bool("ro", false, "Mount read-only")
    allowOther := flag.Bool("allow-other", false, "Allow other users")
    flag.Parse()

    if *mountpoint == "" {
        log.Fatal("Mount point required")
    }

    // Create filesystem based on type
    var fs absfs.FileSystem
    var err error

    switch *fsType {
    case "os":
        fs = osfs.NewFS()
        if *source != "" {
            fs = osfs.NewFS().WithRoot(*source)
        }
    case "mem":
        fs = memfs.NewFS()
    case "s3":
        // Parse S3 config from source
        fs, err = s3fs.NewFS(parseS3Config(*source))
    default:
        log.Fatalf("Unknown filesystem type: %s", *fsType)
    }

    if err != nil {
        log.Fatal(err)
    }

    // Mount options
    opts := &fusefs.MountOptions{
        Mountpoint: *mountpoint,
        ReadOnly:   *readOnly,
        AllowOther: *allowOther,
        FSName:     *fsType + "fs",
    }

    fuseFS, err := fusefs.Mount(fs, opts)
    if err != nil {
        log.Fatal(err)
    }
    defer fuseFS.Unmount()

    log.Printf("Mounted %s filesystem at %s", *fsType, *mountpoint)

    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
    <-sigCh

    log.Println("Unmounting...")
}
```

## Performance Considerations

### Attribute Caching

FUSE kernel performs attribute caching based on `AttrTimeout` and `EntryTimeout`:
- **AttrTimeout**: How long file attributes (size, mtime, permissions) are cached
- **EntryTimeout**: How long directory entries (name → inode) are cached
- Default: 1 second (conservative)
- Remote filesystems: 5-30 seconds (reduce network requests)
- Local filesystems: 100ms-1s (faster updates)

### Directory Caching

User-space directory listing cache:
- Cache `ReadDir()` results to avoid repeated calls
- Invalidate on write operations to the directory
- TTL-based expiration
- Memory-bounded (LRU eviction)

### Read-Ahead

Configure `MaxReadahead` for sequential read workloads:
- Default: 128KB
- Streaming: 1-4MB
- Random access: 0 (disable)

### Write Buffering

- Kernel buffers writes by default
- Use `DirectIO` to bypass (for consistency-critical applications)
- `Fsync()` ensures data persistence

### Parallel Operations

FUSE allows concurrent operations:
- Use `sync.Map` or sharded locks for scalability
- Avoid global locks in hot paths
- File handle tracker benefits from concurrency

### Memory Management

- Pool buffers for Read/Write operations
- Limit cache sizes to prevent unbounded growth
- Monitor and tune inode cache size

## Testing Strategy

### Unit Tests

Test individual components:
- Inode manager: allocation, caching, lookup
- Handle tracker: allocation, reference counting, cleanup
- Error mapper: all error code mappings
- Attribute conversion: FileInfo → FUSE attrs

### Integration Tests

Test with real FUSE mounts:
```go
func TestBasicOperations(t *testing.T) {
    // Create temporary mountpoint
    mountpoint := t.TempDir()

    // Create and mount filesystem
    mfs := memfs.NewFS()
    opts := &fusefs.MountOptions{Mountpoint: mountpoint}
    fuseFS, err := fusefs.Mount(mfs, opts)
    require.NoError(t, err)
    defer fuseFS.Unmount()

    // Test through standard Go file operations
    testFile := filepath.Join(mountpoint, "test.txt")

    // Write
    err = os.WriteFile(testFile, []byte("hello"), 0644)
    assert.NoError(t, err)

    // Read
    data, err := os.ReadFile(testFile)
    assert.NoError(t, err)
    assert.Equal(t, "hello", string(data))

    // Stat
    info, err := os.Stat(testFile)
    assert.NoError(t, err)
    assert.Equal(t, int64(5), info.Size())

    // Remove
    err = os.Remove(testFile)
    assert.NoError(t, err)
}
```

### System Command Tests

Test with real system tools:
```bash
#!/bin/bash
# integration_test.sh

MOUNT=/tmp/fusefs-test

# Mount filesystem
./fusefs -type mem -mount $MOUNT &
FUSE_PID=$!
sleep 1

# Test with standard Unix tools
echo "Testing with ls..."
ls -la $MOUNT

echo "Testing with find..."
find $MOUNT -type f

echo "Testing with dd..."
dd if=/dev/zero of=$MOUNT/testfile bs=1M count=10

echo "Testing with rsync..."
rsync -av /usr/share/dict/words $MOUNT/

# Cleanup
kill $FUSE_PID
```

### Performance Tests

Benchmark operations:
```go
func BenchmarkRead(b *testing.B) {
    mountpoint := setupMount(b)
    defer cleanupMount(mountpoint)

    // Create test file
    testFile := filepath.Join(mountpoint, "benchmark.dat")
    data := make([]byte, 1024*1024) // 1MB
    os.WriteFile(testFile, data, 0644)

    buf := make([]byte, 4096)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        f, _ := os.Open(testFile)
        io.ReadFull(f, buf)
        f.Close()
    }
}
```

### Stress Tests

Test under high load:
- Concurrent read/write operations
- Many open files
- Large directory listings
- Rapid create/delete cycles
- Unmount with pending operations

## Security Considerations

### Permission Checks

- Implement proper access control in `Access()` operation
- Respect absfs permission model
- Consider `DefaultPermissions` mount option (kernel checks)
- Handle UID/GID mapping carefully

### Path Traversal

- Sanitize paths to prevent escaping mountpoint
- Validate symbolic link targets
- Check for `..` components in paths

### Resource Limits

- Limit open file handles
- Bound cache sizes
- Timeout long-running operations
- Rate limit operations if needed

### Sensitive Data

- Clear buffers containing sensitive data
- Avoid logging file contents
- Secure handling of encryption keys
- Consider mlock for key material

### Mount Options

- `AllowOther` requires configuration (`/etc/fuse.conf`)
- `AllowRoot` has security implications
- Document security model clearly

## API Design

### Public Interface

```go
package fusefs

// Mount mounts an absfs.FileSystem at the specified mountpoint
func Mount(fs absfs.FileSystem, opts *MountOptions) (*FuseFS, error)

// MountOptions configures the FUSE mount
type MountOptions struct {
    Mountpoint         string
    ReadOnly           bool
    AllowOther         bool
    AllowRoot          bool
    DefaultPermissions bool
    UID                uint32
    GID                uint32
    DirectIO           bool
    MaxReadahead       uint32
    MaxWrite           uint32
    AsyncRead          bool
    AttrTimeout        time.Duration
    EntryTimeout       time.Duration
    FSName             string
    Options            []string
}

// FuseFS represents a mounted FUSE filesystem
type FuseFS struct {
    // Unexported fields
}

// Unmount unmounts the filesystem
func (f *FuseFS) Unmount() error

// Wait blocks until the filesystem is unmounted
func (f *FuseFS) Wait() error

// Stats returns filesystem statistics
func (f *FuseFS) Stats() Stats

type Stats struct {
    Mountpoint    string
    Operations    uint64
    BytesRead     uint64
    BytesWritten  uint64
    Errors        uint64
    OpenFiles     int
    CachedInodes  int
}
```

## Project Structure

```
fusefs/
├── README.md
├── LICENSE
├── go.mod
├── go.sum
├── .gitignore
├── fuse.go              # Main FUSE adapter
├── mount.go             # Mount/unmount logic
├── operations.go        # FUSE operation handlers
├── inode.go             # Inode manager
├── handles.go           # File handle tracker
├── errors.go            # Error mapping
├── cache.go             # Caching strategies
├── options.go           # Mount options
├── stats.go             # Statistics tracking
├── internal/
│   └── platform/
│       ├── linux.go     # Linux-specific code
│       ├── darwin.go    # macOS-specific code
│       └── windows.go   # Windows-specific code
├── cmd/
│   └── fusefs/
│       └── main.go      # CLI tool
├── examples/
│   ├── basic/
│   ├── s3mount/
│   ├── encrypted/
│   └── composed/
└── integration_test.go  # Integration tests
```

## Dependencies

```
github.com/hanwen/go-fuse/v2   # FUSE library
github.com/absfs/absfs         # Core filesystem interface
golang.org/x/sys               # System calls
```

## Comparison with Other FUSE Libraries

### go-fuse/v2 (Selected)

**Pros:**
- Low-level control and high performance
- Active development and maintenance
- Battle-tested in production systems
- Good macOS support with macFUSE

**Cons:**
- Lower-level API (more code required)
- Steeper learning curve

### bazil.org/fuse

**Pros:**
- Higher-level, more Go-idiomatic API
- Simpler for basic use cases

**Cons:**
- Less active maintenance
- Fewer advanced features
- Performance overhead

### cgofuse

**Pros:**
- True cross-platform (native WinFsp)
- Single codebase for all platforms

**Cons:**
- CGo dependency (complicates builds)
- Cross-compilation challenges
- Less idiomatic Go API

## Future Enhancements

- Support for WinFsp on Windows
- Advanced caching strategies (write-back cache)
- Hot-reload of filesystem configuration
- FUSE protocol negotiation
- Support for FUSE 3 protocol features
- Performance profiling integration
- Distributed filesystem coordination
- Snapshot/versioning support

## Contributing

Contributions welcome! Areas of interest:
- Platform-specific optimizations
- Performance improvements
- Extended attribute support
- Additional mount options
- Testing and benchmarking
- Documentation and examples

## License

MIT License - see LICENSE file

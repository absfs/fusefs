package fusefs

import (
	"context"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
)

// XAttrFS defines the interface for filesystems that support extended attributes.
//
// Extended attributes are name-value pairs associated with files and directories,
// providing a mechanism to attach arbitrary metadata beyond standard file attributes.
//
// absfs implementations can optionally implement this interface to support xattrs.
// If the underlying filesystem doesn't implement XAttrFS, xattr operations will
// return ENOTSUP (not supported).
//
// Common uses of extended attributes:
//   - Security labels (SELinux, AppArmor)
//   - File capabilities
//   - User-defined metadata
//   - MIME types
//   - Checksums
//
// Extended attribute namespaces:
//   - user.*     - User-defined attributes
//   - system.*   - System attributes (ACLs, capabilities)
//   - security.* - Security modules (SELinux, etc.)
//   - trusted.*  - Trusted attributes (root only)
type XAttrFS interface {
	// GetXAttr retrieves the value of an extended attribute.
	//
	// Parameters:
	//   - path: The file path
	//   - name: The attribute name (including namespace, e.g., "user.myattr")
	//
	// Returns:
	//   - []byte: The attribute value
	//   - error: os.ErrNotExist if attribute doesn't exist, os.ErrPermission if denied
	GetXAttr(path string, name string) ([]byte, error)

	// SetXAttr sets the value of an extended attribute.
	//
	// Parameters:
	//   - path: The file path
	//   - name: The attribute name
	//   - value: The attribute value
	//   - flags: XATTR_CREATE (fail if exists) or XATTR_REPLACE (fail if not exists), or 0
	//
	// Returns:
	//   - error: os.ErrExist if XATTR_CREATE and exists, os.ErrNotExist if XATTR_REPLACE and doesn't exist
	SetXAttr(path string, name string, value []byte, flags int) error

	// ListXAttr lists all extended attribute names for a file.
	//
	// Parameters:
	//   - path: The file path
	//
	// Returns:
	//   - []string: List of attribute names
	//   - error: Error if any
	ListXAttr(path string) ([]string, error)

	// RemoveXAttr removes an extended attribute.
	//
	// Parameters:
	//   - path: The file path
	//   - name: The attribute name
	//
	// Returns:
	//   - error: os.ErrNotExist if attribute doesn't exist
	RemoveXAttr(path string, name string) error
}

// Extended attribute flags (from <sys/xattr.h>)
const (
	XATTR_CREATE  = 1 // Set value, fail if exists
	XATTR_REPLACE = 2 // Set value, fail if doesn't exist
)

// Getxattr retrieves an extended attribute value
func (n *fuseNode) Getxattr(ctx context.Context, attr string, dest []byte) (uint32, syscall.Errno) {
	n.fusefs.stats.recordOperation()

	if n.fusefs.checkUnmounting() {
		return 0, syscall.ENOTCONN
	}

	// Check if filesystem supports xattrs
	xattrFS, ok := n.fusefs.absFS.(XAttrFS)
	if !ok {
		return 0, syscall.ENOTSUP
	}

	// Get attribute value
	value, err := xattrFS.GetXAttr(n.path, attr)
	if err != nil {
		n.fusefs.stats.recordError()
		return 0, mapError(err)
	}

	// If dest is nil, return size needed
	if dest == nil {
		return uint32(len(value)), 0
	}

	// Copy to destination
	copied := copy(dest, value)
	return uint32(copied), 0
}

// Setxattr sets an extended attribute value
func (n *fuseNode) Setxattr(ctx context.Context, attr string, data []byte, flags uint32) syscall.Errno {
	n.fusefs.stats.recordOperation()

	if n.fusefs.checkUnmounting() {
		return syscall.ENOTCONN
	}

	// Check if filesystem supports xattrs
	xattrFS, ok := n.fusefs.absFS.(XAttrFS)
	if !ok {
		return syscall.ENOTSUP
	}

	// Set attribute
	err := xattrFS.SetXAttr(n.path, attr, data, int(flags))
	if err != nil {
		n.fusefs.stats.recordError()
		return mapError(err)
	}

	return 0
}

// Listxattr lists all extended attribute names
func (n *fuseNode) Listxattr(ctx context.Context, dest []byte) (uint32, syscall.Errno) {
	n.fusefs.stats.recordOperation()

	if n.fusefs.checkUnmounting() {
		return 0, syscall.ENOTCONN
	}

	// Check if filesystem supports xattrs
	xattrFS, ok := n.fusefs.absFS.(XAttrFS)
	if !ok {
		return 0, syscall.ENOTSUP
	}

	// List attributes
	attrs, err := xattrFS.ListXAttr(n.path)
	if err != nil {
		n.fusefs.stats.recordError()
		return 0, mapError(err)
	}

	// Build null-terminated list
	var totalSize int
	for _, attr := range attrs {
		totalSize += len(attr) + 1 // +1 for null terminator
	}

	// If dest is nil, return size needed
	if dest == nil {
		return uint32(totalSize), 0
	}

	// Copy attributes to destination
	offset := 0
	for _, attr := range attrs {
		copy(dest[offset:], attr)
		offset += len(attr)
		dest[offset] = 0 // null terminator
		offset++
	}

	return uint32(offset), 0
}

// Removexattr removes an extended attribute
func (n *fuseNode) Removexattr(ctx context.Context, attr string) syscall.Errno {
	n.fusefs.stats.recordOperation()

	if n.fusefs.checkUnmounting() {
		return syscall.ENOTCONN
	}

	// Check if filesystem supports xattrs
	xattrFS, ok := n.fusefs.absFS.(XAttrFS)
	if !ok {
		return syscall.ENOTSUP
	}

	// Remove attribute
	err := xattrFS.RemoveXAttr(n.path, attr)
	if err != nil {
		n.fusefs.stats.recordError()
		return mapError(err)
	}

	return 0
}

// Ensure fuseNode implements xattr interfaces
var _ fs.NodeGetxattrer = (*fuseNode)(nil)
var _ fs.NodeSetxattrer = (*fuseNode)(nil)
var _ fs.NodeListxattrer = (*fuseNode)(nil)
var _ fs.NodeRemovexattrer = (*fuseNode)(nil)

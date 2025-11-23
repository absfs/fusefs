package main

import (
	"os"
	"time"

	"github.com/absfs/absfs"
)

// osFS is a simple filesystem wrapper around os functions
type osFS struct {
	root string
}

// NewOSFS creates a new OS filesystem rooted at the given path
func NewOSFS(root string) absfs.FileSystem {
	return &osFS{root: root}
}

func (fs *osFS) OpenFile(name string, flag int, perm os.FileMode) (absfs.File, error) {
	return os.OpenFile(fs.path(name), flag, perm)
}

func (fs *osFS) Mkdir(name string, perm os.FileMode) error {
	return os.Mkdir(fs.path(name), perm)
}

func (fs *osFS) Remove(name string) error {
	return os.Remove(fs.path(name))
}

func (fs *osFS) Rename(oldpath, newpath string) error {
	return os.Rename(fs.path(oldpath), fs.path(newpath))
}

func (fs *osFS) Stat(name string) (os.FileInfo, error) {
	return os.Stat(fs.path(name))
}

func (fs *osFS) Chmod(name string, mode os.FileMode) error {
	return os.Chmod(fs.path(name), mode)
}

func (fs *osFS) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return os.Chtimes(fs.path(name), atime, mtime)
}

func (fs *osFS) Chown(name string, uid, gid int) error {
	return os.Chown(fs.path(name), uid, gid)
}

func (fs *osFS) Separator() uint8 {
	return os.PathSeparator
}

func (fs *osFS) ListSeparator() uint8 {
	return os.PathListSeparator
}

func (fs *osFS) Chdir(dir string) error {
	return os.Chdir(fs.path(dir))
}

func (fs *osFS) Getwd() (dir string, err error) {
	return os.Getwd()
}

func (fs *osFS) TempDir() string {
	return os.TempDir()
}

func (fs *osFS) Open(name string) (absfs.File, error) {
	return os.Open(fs.path(name))
}

func (fs *osFS) Create(name string) (absfs.File, error) {
	return os.Create(fs.path(name))
}

func (fs *osFS) MkdirAll(name string, perm os.FileMode) error {
	return os.MkdirAll(fs.path(name), perm)
}

func (fs *osFS) RemoveAll(path string) error {
	return os.RemoveAll(fs.path(path))
}

func (fs *osFS) Truncate(name string, size int64) error {
	return os.Truncate(fs.path(name), size)
}

func (fs *osFS) path(name string) string {
	if fs.root == "" || fs.root == "/" {
		return name
	}
	if name == "/" {
		return fs.root
	}
	return fs.root + name
}

package ghostscript

import (
	"io/fs"
	"os"

	"github.com/spf13/afero"
	"github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/experimental/sysfs"
	system "github.com/tetratelabs/wazero/sys"
)

type stashedOpenParams struct {
	openFileFlags sys.Oflag
	openFilePerm  fs.FileMode
}

type memAdaptFS struct {
	sys.UnimplementedFS

	wrapped  sys.FS
	original afero.Fs
	stashed  *stashedOpenParams
}

// A horrible hack of a way to expose a working file system (wrt OpenFile) to wazero
func NewInMemoryWasmFS() *memAdaptFS {
	original := afero.NewMemMapFs()
	adapter := &memAdaptFS{original: original}
	adapter.wrapped = &sysfs.AdaptFS{FS: adapter}
	return adapter
}

func (fs *memAdaptFS) OpenFile(path string, flag sys.Oflag, perm fs.FileMode) (sys.File, sys.Errno) {
	fs.stashed = &stashedOpenParams{flag, perm}
	// This just calls Open(path) and drops the flag/perm which breaks semantics. So we
	// stash the flags above and properly call OpenFile in our Open implementation.
	//
	// We still reuse this OpenFile because we want the nice adapter from os.File to sys.File
	f, errno := fs.wrapped.OpenFile(path, flag, perm)
	fs.stashed = nil
	return f, errno
}

func (fs *memAdaptFS) Lstat(path string) (system.Stat_t, sys.Errno) {
	return fs.wrapped.Lstat(path)
}

func (fs *memAdaptFS) Stat(path string) (system.Stat_t, sys.Errno) {
	return fs.wrapped.Stat(path)
}

func (fs *memAdaptFS) Readlink(path string) (string, sys.Errno) {
	return fs.wrapped.Readlink(path)
}

func (fs *memAdaptFS) Mkdir(path string, perm fs.FileMode) sys.Errno {
	return fs.wrapped.Mkdir(path, perm)
}

func (fs *memAdaptFS) Chmod(path string, perm fs.FileMode) sys.Errno {
	return fs.wrapped.Chmod(path, perm)
}

func (fs *memAdaptFS) Rename(from, to string) sys.Errno {
	return fs.wrapped.Rename(from, to)
}

func (fs *memAdaptFS) Rmdir(path string) sys.Errno {
	return fs.wrapped.Rmdir(path)
}

func (fs *memAdaptFS) Open(name string) (fs.File, error) {
	if fs.stashed == nil {
		return fs.original.Open(name)
	}
	return fs.original.OpenFile(name, translateFlags(fs.stashed.openFileFlags), fs.stashed.openFilePerm)
}

func translateFlags(f sys.Oflag) int {
	r := 0
	mapping := map[sys.Oflag]int{
		sys.O_CREAT:  os.O_CREATE,
		sys.O_APPEND: os.O_APPEND,
		sys.O_EXCL:   os.O_EXCL,
		sys.O_RDWR:   os.O_RDWR,
		sys.O_SYNC:   os.O_SYNC,
		sys.O_TRUNC:  os.O_TRUNC,
		sys.O_RDONLY: os.O_RDONLY,
		sys.O_WRONLY: os.O_WRONLY,
	}
	for wasiFlag, osFlag := range mapping {
		if f&wasiFlag != 0 {
			r = r | osFlag
		}
	}
	return r
}

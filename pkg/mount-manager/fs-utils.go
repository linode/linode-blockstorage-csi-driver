package mountmanager

import (
	"io/fs"
	"os"
)

type FileInterface interface {
	Close() error
}

// FileSystem defines the methods for file system operations.
type FileSystem interface {
	IsNotExist(err error) bool
	MkdirAll(path string, perm os.FileMode) error
	Stat(name string) (fs.FileInfo, error)
	Remove(path string) error
	OpenFile(name string, flag int, perm os.FileMode) (FileInterface, error)
}

// OSFileSystem implements FileSystemInterface using the os package.
type OSFileSystem struct{}

func NewFileSystem() FileSystem {
	return OSFileSystem{}
}

func (OSFileSystem) IsNotExist(err error) bool {
	return os.IsNotExist(err)
}

func (OSFileSystem) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (OSFileSystem) Stat(name string) (fs.FileInfo, error) {
	return os.Stat(name)
}

func (OSFileSystem) Remove(path string) error {
	return os.Remove(path)
}

func (OSFileSystem) OpenFile(name string, flag int, perm os.FileMode) (FileInterface, error) {
	return os.OpenFile(name, flag, perm)
}
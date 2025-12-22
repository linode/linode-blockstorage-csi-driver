package filesystemstats

import "golang.org/x/sys/unix"

// FilesystemStatter provides an interface for getting filesystem statistics.
// This interface allows for easier mocking in tests.
type FilesystemStatter interface {
	// Statfs returns filesystem statistics for the given path.
	Statfs(path string, stat *unix.Statfs_t) error
}

// UnixFilesystemStatter implements FilesystemStatter using the real unix.Statfs system call.
type UnixFilesystemStatter struct{}

// Statfs calls the unix.Statfs system call.
func (u *UnixFilesystemStatter) Statfs(path string, stat *unix.Statfs_t) error {
	return unix.Statfs(path, stat)
}

// NewFilesystemStatter creates a new FilesystemStatter using the real unix.Statfs.
func NewFilesystemStatter() FilesystemStatter {
	return &UnixFilesystemStatter{}
}

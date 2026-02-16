package scheduler

import (
	"os"
	"syscall"
)

// FileLock provides a non-blocking file lock using flock(2).
type FileLock struct {
	path string
	file *os.File
}

// NewFileLock creates a FileLock for the given path.
func NewFileLock(path string) *FileLock {
	return &FileLock{path: path}
}

// TryLock attempts to acquire the lock without blocking.
// Returns true if the lock was acquired, false if another process holds it.
func (l *FileLock) TryLock() (bool, error) {
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return false, err
	}

	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		f.Close()
		if err == syscall.EWOULDBLOCK {
			return false, nil
		}
		return false, err
	}

	l.file = f
	return true, nil
}

// Unlock releases the lock and removes the lock file.
func (l *FileLock) Unlock() error {
	if l.file == nil {
		return nil
	}
	if err := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN); err != nil {
		l.file.Close()
		return err
	}
	name := l.file.Name()
	l.file.Close()
	l.file = nil
	os.Remove(name)
	return nil
}

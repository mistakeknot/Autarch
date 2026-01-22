//go:build windows

package file

import "os"

type FileLock struct{ f *os.File }

func LockFile(path string) (*FileLock, error) {
	lockPath := path + ".lock"
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	return &FileLock{f: f}, nil
}

func (l *FileLock) Unlock() error {
	if l == nil || l.f == nil {
		return nil
	}
	return l.f.Close()
}

package lockfile

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type syncedFile interface {
	Name() string
	Chmod(os.FileMode) error
	Write([]byte) (int, error)
	Sync() error
	Close() error
}

type atomicOperations interface {
	CreateTemp(string, string) (syncedFile, error)
	Rename(string, string) error
	ReadFile(string) ([]byte, error)
	Remove(string) error
}

type osAtomicOperations struct{}

func (osAtomicOperations) CreateTemp(directory, pattern string) (syncedFile, error) {
	return os.CreateTemp(directory, pattern)
}
func (osAtomicOperations) Rename(oldPath, newPath string) error { return os.Rename(oldPath, newPath) }
func (osAtomicOperations) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }
func (osAtomicOperations) Remove(path string) error             { return os.Remove(path) }

// WriteAtomic replaces a lock through a synced same-directory temporary file.
func WriteAtomic(path string, data []byte) error {
	return writeAtomic(path, data, osAtomicOperations{})
}

func writeAtomic(path string, data []byte, operations atomicOperations) error {
	old, oldErr := operations.ReadFile(path)
	if oldErr != nil && !errors.Is(oldErr, os.ErrNotExist) {
		return fmt.Errorf("read old lock: %w", oldErr)
	}
	directory := filepath.Dir(path)
	temporary, err := operations.CreateTemp(directory, ".gunte.lock.*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	renamed := false
	defer func() {
		if !renamed {
			_ = operations.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return err
	}
	if count, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	} else if count != len(data) {
		_ = temporary.Close()
		return io.ErrShortWrite
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := operations.Rename(temporaryPath, path); err != nil {
		observed, readErr := operations.ReadFile(path)
		if readErr == nil && bytes.Equal(observed, data) {
			return nil
		}
		if readErr == nil && oldErr == nil && bytes.Equal(observed, old) {
			return fmt.Errorf("rename left the old lock unchanged: %w", err)
		}
		return fmt.Errorf("rename result could not be confirmed: %w", err)
	}
	renamed = true
	observed, err := operations.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read after rename: %w", err)
	}
	if !bytes.Equal(observed, data) {
		return fmt.Errorf("lock bytes differ after rename")
	}
	return nil
}

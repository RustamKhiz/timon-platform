package bot

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type OffsetStore interface {
	Load() (int64, error)
	Save(int64) error
}

type memoryOffsetStore struct {
	offset int64
}

func (s memoryOffsetStore) Load() (int64, error) { return s.offset, nil }
func (s memoryOffsetStore) Save(int64) error     { return nil }

type FileOffsetStore struct {
	path string
}

func NewFileOffsetStore(path string) *FileOffsetStore {
	return &FileOffsetStore{path: path}
}

func (s *FileOffsetStore) Load() (int64, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read offset file: %w", err)
	}
	value, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil || value < 0 {
		return 0, errors.New("offset file contains an invalid value")
	}
	return value, nil
}

func (s *FileOffsetStore) Save(offset int64) error {
	if offset < 0 {
		return errors.New("offset cannot be negative")
	}
	directory := filepath.Dir(s.path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create offset directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".telegram-offset-*")
	if err != nil {
		return fmt.Errorf("create temporary offset file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath) //nolint:errcheck
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close() //nolint:errcheck
		return fmt.Errorf("protect temporary offset file: %w", err)
	}
	if _, err := temporary.WriteString(strconv.FormatInt(offset, 10) + "\n"); err != nil {
		temporary.Close() //nolint:errcheck
		return fmt.Errorf("write temporary offset file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close() //nolint:errcheck
		return fmt.Errorf("sync temporary offset file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary offset file: %w", err)
	}
	if err := os.Rename(temporaryPath, s.path); err != nil {
		if removeErr := os.Remove(s.path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return fmt.Errorf("remove previous offset file: %w", removeErr)
		}
		if retryErr := os.Rename(temporaryPath, s.path); retryErr != nil {
			return fmt.Errorf("replace offset file: %w", retryErr)
		}
	}
	return nil
}

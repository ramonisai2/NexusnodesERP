package blob

import (
	"fmt"
	"os"
	"path/filepath"
)

// LocalStore persists processed image bytes under a root directory.
type LocalStore struct {
	Root string
}

func NewLocalStore(root string) (*LocalStore, error) {
	if root == "" {
		root = "./data/image-reports"
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &LocalStore{Root: root}, nil
}

func (s *LocalStore) Put(key string, data []byte) error {
	path := filepath.Join(s.Root, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *LocalStore) Get(key string) ([]byte, error) {
	path := filepath.Join(s.Root, filepath.FromSlash(key))
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("blob not found: %w", err)
	}
	return data, nil
}

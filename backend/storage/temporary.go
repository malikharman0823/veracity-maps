package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type StorageManager struct {
	BaseSessionDir string
	UploadDir      string
	ProcessedDir   string
}

func NewStorageManager(sessionDir, uploadDir, processedDir string) (*StorageManager, error) {
	sm := &StorageManager{
		BaseSessionDir: sessionDir,
		UploadDir:      uploadDir,
		ProcessedDir:   processedDir,
	}
	if err := os.MkdirAll(sm.BaseSessionDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create session directory: %w", err)
	}
	if err := os.MkdirAll(sm.UploadDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create upload directory: %w", err)
	}
	if err := os.MkdirAll(sm.ProcessedDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create processed directory: %w", err)
	}
	return sm, nil
}

func (sm *StorageManager) GetSessionDir(sessionID string) string {
	return filepath.Join(sm.BaseSessionDir, sessionID)
}

func (sm *StorageManager) GetDatasetDir(sessionID, datasetID string) string {
	return filepath.Join(sm.GetSessionDir(sessionID), "datasets", datasetID)
}

func (sm *StorageManager) SafePath(baseDir, relativePath string) (string, error) {
	cleanBase := filepath.Clean(baseDir)
	target := filepath.Join(cleanBase, relativePath)
	cleanTarget := filepath.Clean(target)

	// Ensure target is within baseDir (guards against zip-slip and path traversal)
	if !strings.HasPrefix(cleanTarget, cleanBase+string(filepath.Separator)) && cleanTarget != cleanBase {
		return "", fmt.Errorf("unsafe path traversal detected: %s outside %s", relativePath, baseDir)
	}
	return cleanTarget, nil
}

// SafeRemoveAll removes path, clearing Windows read-only attributes and retrying on lock
func SafeRemoveAll(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		// Walk and clear read-only flag
		_ = filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
			if err == nil && info != nil {
				_ = os.Chmod(p, 0666)
			}
			return nil
		})

		err := os.RemoveAll(path)
		if err == nil {
			return nil
		}
		lastErr = err
		time.Sleep(time.Duration(50*(attempt+1)) * time.Millisecond)
	}
	return lastErr
}

// CopyFile copies a file from src to dst safely
func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

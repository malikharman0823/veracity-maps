package storage

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrUnauthorizedStorageAccess = errors.New("storage access denied: resource does not belong to authenticated user")
	ErrStorageQuotaExceeded      = errors.New("user storage quota exceeded")
	ErrInvalidStorageNamespace   = errors.New("invalid user or project storage identifier")
)

const (
	// DefaultUserQuotaBytes caps private storage per user at 2 GB by default.
	DefaultUserQuotaBytes int64 = 2 * 1024 * 1024 * 1024
)

// PrivateStorage enforces strict per-user filesystem isolation:
//
//	<root>/<owner_id>/projects/<project_id>/{uploads,processed,exports,assets}/
//	<root>/<owner_id>/temp/
type PrivateStorage struct {
	RootDir        string
	UserQuotaBytes int64
}

func NewPrivateStorage(rootDir string, quotaBytes int64) (*PrivateStorage, error) {
	if strings.TrimSpace(rootDir) == "" {
		rootDir = filepath.Join("storage", "private")
	}
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(absRoot, 0755); err != nil {
		return nil, err
	}
	if quotaBytes <= 0 {
		quotaBytes = DefaultUserQuotaBytes
	}
	return &PrivateStorage{
		RootDir:        absRoot,
		UserQuotaBytes: quotaBytes,
	}, nil
}

func sanitizeSegment(seg string) (string, error) {
	clean := strings.TrimSpace(seg)
	if clean == "" || strings.Contains(clean, "..") || strings.ContainsAny(clean, `/\:*?"<>|`) {
		return "", ErrInvalidStorageNamespace
	}
	return clean, nil
}

// UserRootDir returns the canonical private root directory for ownerID.
func (ps *PrivateStorage) UserRootDir(ownerID string) (string, error) {
	uid, err := sanitizeSegment(ownerID)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(ps.RootDir, uid)
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(absDir, ps.RootDir+string(filepath.Separator)) {
		return "", ErrUnauthorizedStorageAccess
	}
	return absDir, nil
}

// ProjectBucketDir returns <root>/<owner_id>/projects/<project_id>/<bucket>
// where bucket is one of: "uploads", "processed", "exports", "assets", "maps".
func (ps *PrivateStorage) ProjectBucketDir(ownerID, projectID, bucket string) (string, error) {
	userRoot, err := ps.UserRootDir(ownerID)
	if err != nil {
		return "", err
	}
	pid, err := sanitizeSegment(projectID)
	if err != nil {
		return "", err
	}
	switch bucket {
	case "uploads", "processed", "exports", "assets", "maps":
	default:
		return "", fmt.Errorf("%w: unsupported storage bucket %q", ErrInvalidStorageNamespace, bucket)
	}
	dir := filepath.Join(userRoot, "projects", pid, bucket)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

// UserTempDir returns <root>/<owner_id>/temp for isolated temporary processing files.
func (ps *PrivateStorage) UserTempDir(ownerID string) (string, error) {
	userRoot, err := ps.UserRootDir(ownerID)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(userRoot, "temp")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

// GenerateInternalFilename creates a unique, safe internal filename so user-supplied names
// are never trusted as filesystem paths and original uploads cannot be overwritten.
func GenerateInternalFilename(prefix, originalFilename string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	ext := strings.ToLower(filepath.Ext(filepath.Base(originalFilename)))
	if len(ext) > 12 || strings.ContainsAny(ext, `/\:*?"<>| `) {
		ext = ""
	}
	return fmt.Sprintf("%s_%s%s", prefix, hex.EncodeToString(b), ext)
}

// VerifyOwnerPath guarantees that candidatePath resides strictly inside ownerID's private namespace.
func (ps *PrivateStorage) VerifyOwnerPath(ownerID, candidatePath string) (string, error) {
	userRoot, err := ps.UserRootDir(ownerID)
	if err != nil {
		return "", err
	}
	absPath, err := filepath.Abs(candidatePath)
	if err != nil {
		return "", ErrUnauthorizedStorageAccess
	}
	if absPath != userRoot && !strings.HasPrefix(absPath, userRoot+string(filepath.Separator)) {
		return "", ErrUnauthorizedStorageAccess
	}
	return absPath, nil
}

// CheckQuota verifies that adding incomingBytes does not exceed ownerID's private storage quota.
func (ps *PrivateStorage) CheckQuota(ownerID string, incomingBytes int64) error {
	userRoot, err := ps.UserRootDir(ownerID)
	if err != nil {
		return err
	}
	var used int64
	_ = filepath.Walk(userRoot, func(_ string, info os.FileInfo, wErr error) error {
		if wErr == nil && info != nil && !info.IsDir() {
			used += info.Size()
		}
		return nil
	})
	if used+incomingBytes > ps.UserQuotaBytes {
		return fmt.Errorf("%w (used=%d, incoming=%d, limit=%d)", ErrStorageQuotaExceeded, used, incomingBytes, ps.UserQuotaBytes)
	}
	return nil
}

// WritePrivateFile validates owner namespace + quota and writes data atomically inside the user's bucket.
func (ps *PrivateStorage) WritePrivateFile(ownerID, projectID, bucket, originalFilename string, data []byte) (string, error) {
	if err := ps.CheckQuota(ownerID, int64(len(data))); err != nil {
		return "", err
	}
	bucketDir, err := ps.ProjectBucketDir(ownerID, projectID, bucket)
	if err != nil {
		return "", err
	}
	internalName := GenerateInternalFilename(bucket, originalFilename)
	fullPath := filepath.Join(bucketDir, internalName)
	verified, err := ps.VerifyOwnerPath(ownerID, fullPath)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(verified, data, 0600); err != nil {
		return "", err
	}
	return verified, nil
}

// ReadPrivateFile verifies ownership before reading any file from private storage.
func (ps *PrivateStorage) ReadPrivateFile(ownerID, filePath string) ([]byte, error) {
	verified, err := ps.VerifyOwnerPath(ownerID, filePath)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(verified)
}

// DeletePrivateFile verifies ownership before deleting a file from private storage.
func (ps *PrivateStorage) DeletePrivateFile(ownerID, filePath string) error {
	verified, err := ps.VerifyOwnerPath(ownerID, filePath)
	if err != nil {
		return err
	}
	return os.Remove(verified)
}

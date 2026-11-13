package services

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
	"github.com/malikharman0823/veracity-maps/storage"
)

// MapConfigurationService coordinates durable map configuration commits.
type MapConfigurationService struct {
	maps        repository.MapRepo
	uploadRoot  string
	sessionRoot string
}

func NewMapConfigurationService(maps repository.MapRepo, uploadRoot, sessionRoot string) *MapConfigurationService {
	return &MapConfigurationService{
		maps:        maps,
		uploadRoot:  uploadRoot,
		sessionRoot: sessionRoot,
	}
}

func (s *MapConfigurationService) CreatePersonalMapFromConfiguration(
	ctx context.Context,
	user *models.User,
	commit models.MapConfigurationCommit,
) (*models.MapConfigurationResult, error) {
	if s == nil || s.maps == nil {
		return nil, models.ErrPersistenceUnavailable
	}
	if user == nil || user.ID == "" {
		return nil, models.ErrMapAccessDenied
	}
	if strings.TrimSpace(commit.Name) == "" || strings.TrimSpace(commit.PrimaryDatasetID) == "" || strings.TrimSpace(commit.ConfigurationJSON) == "" {
		return nil, models.ErrInvalidMapConfiguration
	}

	mapID := newPersistentMapID(user.ID, commit.Name)
	commit.MapID = mapID
	durableMapRoot := filepath.Join(s.uploadRoot, "maps", user.ID, mapID)
	assetIDs := uniqueAssetIDs(commit.PrimaryDatasetID, commit.MemberAssetIDs)
	commit.MemberAssetIDs = assetIDs
	commit.AssetLocations = make([]models.MapAssetLocation, 0, len(assetIDs))
	createdDirectories := make([]string, 0, len(assetIDs))
	cleanupCreated := func() {
		for i := len(createdDirectories) - 1; i >= 0; i-- {
			_ = storage.SafeRemoveAll(createdDirectories[i])
		}
		_ = storage.SafeRemoveAll(durableMapRoot)
	}

	for _, assetID := range assetIDs {
		if !safePathSegment(assetID) {
			cleanupCreated()
			return nil, models.ErrInvalidMapConfiguration
		}
		asset, err := s.maps.GetAsset(ctx, assetID, user.ID)
		if err != nil {
			cleanupCreated()
			return nil, err
		}
		if asset.OwnerUserID != user.ID {
			cleanupCreated()
			return nil, models.ErrMapAccessDenied
		}
		location, createdDir, err := s.stageAsset(user.ID, mapID, asset)
		if err != nil {
			cleanupCreated()
			return nil, err
		}
		if createdDir != "" {
			createdDirectories = append(createdDirectories, createdDir)
		}
		commit.AssetLocations = append(commit.AssetLocations, location)
	}

	result, err := s.maps.CreateMapFromConfiguration(ctx, user.ID, user.Name, commit)
	if err != nil {
		cleanupCreated()
		return nil, err
	}
	return result, nil
}

func (s *MapConfigurationService) SaveExistingMapConfiguration(
	ctx context.Context,
	user *models.User,
	mapID string,
	commit models.MapConfigurationCommit,
) (*models.MapConfigurationResult, error) {
	if s == nil || s.maps == nil {
		return nil, models.ErrPersistenceUnavailable
	}
	if user == nil || user.ID == "" {
		return nil, models.ErrMapAccessDenied
	}
	if strings.TrimSpace(mapID) == "" || strings.TrimSpace(commit.ConfigurationJSON) == "" {
		return nil, models.ErrInvalidMapConfiguration
	}
	mapRecord, err := s.maps.GetMap(ctx, mapID, user.ID)
	if err != nil {
		return nil, err
	}
	if mapRecord.Permission != models.PermissionEditor {
		return nil, models.ErrMapAccessDenied
	}

	assetIDs := uniqueAssetIDs(commit.PrimaryDatasetID, commit.MemberAssetIDs)
	commit.MemberAssetIDs = assetIDs
	commit.AssetLocations = make([]models.MapAssetLocation, 0, len(assetIDs))
	createdDirectories := make([]string, 0, len(assetIDs))
	cleanupCreated := func() {
		for i := len(createdDirectories) - 1; i >= 0; i-- {
			_ = storage.SafeRemoveAll(createdDirectories[i])
		}
	}
	for _, assetID := range assetIDs {
		if !safePathSegment(assetID) {
			cleanupCreated()
			return nil, models.ErrInvalidMapConfiguration
		}
		asset, err := s.maps.GetAsset(ctx, assetID, user.ID)
		if err != nil && mapRecord.OwnerUserID != "" && mapRecord.OwnerUserID != user.ID {
			asset, err = s.maps.GetAsset(ctx, assetID, mapRecord.OwnerUserID)
		}
		if err != nil || asset == nil {
			if discovered := s.discoverAssetFromSession(assetID, mapID, mapRecord.OwnerUserID); discovered != nil {
				if regErr := s.maps.RegisterAsset(ctx, discovered); regErr == nil {
					asset = discovered
					err = nil
				}
			}
		}
		if err != nil || asset == nil {
			cleanupCreated()
			if err != nil {
				return nil, err
			}
			return nil, models.ErrMapAssetNotFound
		}
		if asset.OwnerUserID != mapRecord.OwnerUserID && asset.OwnerUserID != user.ID {
			cleanupCreated()
			return nil, models.ErrMapAccessDenied
		}

		durableAssetRoot := filepath.Join(s.uploadRoot, "maps", mapRecord.OwnerUserID, mapID, "assets", asset.ID)
		location := models.MapAssetLocation{
			AssetID:     asset.ID,
			OwnerUserID: mapRecord.OwnerUserID,
			StoragePath: asset.StoragePath,
		}
		assetToStage := *asset
		if livePath := s.findLatestSessionDatasetFile(asset.ID, filepath.Base(asset.StoragePath)); livePath != "" && !pathWithin(durableAssetRoot, livePath) {
			assetToStage.StoragePath = livePath
		}
		if !pathWithin(durableAssetRoot, assetToStage.StoragePath) {
			var createdDir string
			location, createdDir, err = s.stageAsset(mapRecord.OwnerUserID, mapID, &assetToStage)
			if err != nil {
				cleanupCreated()
				return nil, err
			}
			if createdDir != "" {
				createdDirectories = append(createdDirectories, createdDir)
			}
		}
		commit.AssetLocations = append(commit.AssetLocations, location)
	}

	result, err := s.maps.SaveMapConfiguration(ctx, mapID, user.ID, user.Name, commit)
	if err != nil {
		cleanupCreated()
		return nil, err
	}
	return result, nil
}

func (s *MapConfigurationService) stageAsset(ownerUserID, mapID string, asset *models.MapAsset) (models.MapAssetLocation, string, error) {
	if asset == nil || !safePathSegment(ownerUserID) || !safePathSegment(mapID) || !safePathSegment(asset.ID) {
		return models.MapAssetLocation{}, "", models.ErrInvalidMapConfiguration
	}
	sourcePath, err := filepath.Abs(asset.StoragePath)
	if err != nil {
		return models.MapAssetLocation{}, "", fmt.Errorf("resolve asset source: %w", err)
	}
	sourceInfo, err := os.Stat(sourcePath)
	if err != nil || sourceInfo.IsDir() {
		return models.MapAssetLocation{}, "", models.ErrMapAssetNotFound
	}

	durableRoot := filepath.Join(s.uploadRoot, "maps", ownerUserID, mapID, "assets", asset.ID)
	stagingRoot := durableRoot + fmt.Sprintf(".staging-%d", time.Now().UTC().UnixNano())
	if err := storage.CopyTree(filepath.Dir(sourcePath), stagingRoot); err != nil {
		_ = storage.SafeRemoveAll(stagingRoot)
		return models.MapAssetLocation{}, "", fmt.Errorf("stage asset %s: %w", asset.ID, err)
	}
	stagedPrimary := filepath.Join(stagingRoot, filepath.Base(sourcePath))
	stagedInfo, err := os.Stat(stagedPrimary)
	if err != nil || stagedInfo.Size() != sourceInfo.Size() {
		_ = storage.SafeRemoveAll(stagingRoot)
		return models.MapAssetLocation{}, "", fmt.Errorf("verify staged asset %s: size mismatch", asset.ID)
	}
	rewriteStagedDatasetPaths(stagingRoot, filepath.Base(sourcePath))

	existedBefore := false
	if _, err := os.Stat(durableRoot); err == nil {
		existedBefore = true
		if rmErr := storage.SafeRemoveAll(durableRoot); rmErr != nil {
			_ = storage.SafeRemoveAll(stagingRoot)
			return models.MapAssetLocation{}, "", fmt.Errorf("replace durable asset directory: %w", rmErr)
		}
	} else if !os.IsNotExist(err) {
		_ = storage.SafeRemoveAll(stagingRoot)
		return models.MapAssetLocation{}, "", fmt.Errorf("inspect durable asset directory: %w", err)
	}
	if err := os.Rename(stagingRoot, durableRoot); err != nil {
		_ = storage.SafeRemoveAll(stagingRoot)
		return models.MapAssetLocation{}, "", fmt.Errorf("publish staged asset %s: %w", asset.ID, err)
	}
	rewriteStagedDatasetPaths(durableRoot, filepath.Base(sourcePath))

	createdDir := durableRoot
	if existedBefore {
		createdDir = ""
	}
	return models.MapAssetLocation{
		AssetID:     asset.ID,
		OwnerUserID: ownerUserID,
		StoragePath: filepath.Join(durableRoot, filepath.Base(sourcePath)),
	}, createdDir, nil
}

func (s *MapConfigurationService) findLatestSessionDatasetFile(assetID, filename string) string {
	if s == nil || s.sessionRoot == "" || !safePathSegment(assetID) {
		return ""
	}
	entries, err := os.ReadDir(s.sessionRoot)
	if err != nil {
		return ""
	}
	var bestPath string
	var bestMod time.Time
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dsDir := filepath.Join(s.sessionRoot, entry.Name(), "datasets", assetID)
		info, err := os.Stat(dsDir)
		if err != nil || !info.IsDir() {
			continue
		}
		var candidate string
		if filename != "" && safePathSegment(filename) {
			p := filepath.Join(dsDir, filename)
			if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
				candidate = p
			}
		}
		if candidate == "" {
			metaPath := filepath.Join(dsDir, "dataset.json")
			if metaBytes, err := os.ReadFile(metaPath); err == nil {
				var ds models.Dataset
				if json.Unmarshal(metaBytes, &ds) == nil && ds.Filename != "" && safePathSegment(ds.Filename) {
					p := filepath.Join(dsDir, ds.Filename)
					if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
						candidate = p
					}
				}
			}
		}
		if candidate == "" {
			continue
		}
		modTime := info.ModTime()
		if metaInfo, err := os.Stat(filepath.Join(dsDir, "dataset.json")); err == nil && metaInfo.ModTime().After(modTime) {
			modTime = metaInfo.ModTime()
		}
		if bestPath == "" || modTime.After(bestMod) {
			bestPath = candidate
			bestMod = modTime
		}
	}
	return bestPath
}

func (s *MapConfigurationService) discoverAssetFromSession(assetID, mapID, ownerUserID string) *models.MapAsset {
	if s == nil || s.sessionRoot == "" || !safePathSegment(assetID) {
		return nil
	}
	entries, err := os.ReadDir(s.sessionRoot)
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dsDir := filepath.Join(s.sessionRoot, entry.Name(), "datasets", assetID)
		metaPath := filepath.Join(dsDir, "dataset.json")
		metaBytes, err := os.ReadFile(metaPath)
		if err != nil {
			continue
		}
		var ds models.Dataset
		if err := json.Unmarshal(metaBytes, &ds); err != nil || ds.ID == "" {
			continue
		}
		storagePath := ds.StoragePath
		if storagePath == "" || !mapAssetFileExists(storagePath) {
			if ds.Filename != "" {
				candidate := filepath.Join(dsDir, filepath.Base(ds.Filename))
				if mapAssetFileExists(candidate) {
					storagePath = candidate
				}
			}
		}
		if storagePath == "" || !mapAssetFileExists(storagePath) {
			continue
		}
		return &models.MapAsset{
			ID:               ds.ID,
			MapID:            mapID,
			OwnerUserID:      ownerUserID,
			OriginalFilename: ds.Filename,
			StoragePath:      storagePath,
			FileType:         ds.ContentType,
			FileSize:         ds.FileSize,
			MimeType:         ds.ContentType,
			CreatedAt:        time.Now().UTC(),
		}
	}
	return nil
}

func rewriteStagedDatasetPaths(dir, primaryFilename string) {
	metaPath := filepath.Join(dir, "dataset.json")
	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		return
	}
	var ds models.Dataset
	if err := json.Unmarshal(metaBytes, &ds); err != nil {
		return
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		absDir = dir
	}
	if primaryFilename != "" {
		candidate := filepath.Join(absDir, primaryFilename)
		if mapAssetFileExists(candidate) {
			ds.StoragePath = candidate
		}
	}
	if ds.GeoreferencedPath != "" {
		vrtCandidate := filepath.Join(absDir, "processed", filepath.Base(ds.GeoreferencedPath))
		if mapAssetFileExists(vrtCandidate) {
			ds.GeoreferencedPath = vrtCandidate
		} else if rootCandidate := filepath.Join(absDir, filepath.Base(ds.GeoreferencedPath)); mapAssetFileExists(rootCandidate) {
			ds.GeoreferencedPath = rootCandidate
		}
	}
	if ds.ProcessedPath != "" {
		procCandidate := filepath.Join(absDir, "processed", filepath.Base(ds.ProcessedPath))
		if mapAssetFileExists(procCandidate) {
			ds.ProcessedPath = procCandidate
		} else if rootCandidate := filepath.Join(absDir, filepath.Base(ds.ProcessedPath)); mapAssetFileExists(rootCandidate) {
			ds.ProcessedPath = rootCandidate
		}
	}
	if updated, err := json.MarshalIndent(&ds, "", "  "); err == nil {
		_ = os.WriteFile(metaPath, updated, 0644)
	}
}

func mapAssetFileExists(p string) bool {
	if p == "" {
		return false
	}
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

func uniqueAssetIDs(primaryID string, memberIDs []string) []string {
	seen := make(map[string]struct{}, len(memberIDs)+1)
	result := make([]string, 0, len(memberIDs)+1)
	for _, id := range append([]string{primaryID}, memberIDs...) {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func safePathSegment(value string) bool {
	return value != "" && value != "." && value != ".." && filepath.Base(value) == value && !strings.ContainsAny(value, `/\\`)
}

func pathWithin(root, path string) bool {
	root, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && relative != "."
}

func newPersistentMapID(ownerUserID, name string) string {
	now := time.Now().UTC().UnixNano()
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s-%s-%d", ownerUserID, name, now)))
	return fmt.Sprintf("map_%x", sum)[:20]
}

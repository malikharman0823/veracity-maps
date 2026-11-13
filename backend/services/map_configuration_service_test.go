package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
)

type recordingMapConfigurationRepo struct {
	repository.MapRepo
	ownerUserID string
	authorName  string
	commit      models.MapConfigurationCommit
	result      *models.MapConfigurationResult
	assets      map[string]*models.MapAsset
	createErr   error
	mapsByID    map[string]*models.Map
	saveMapID   string
	saveUserID  string
	saveAuthor  string
	saveCommit  models.MapConfigurationCommit
	saveResult  *models.MapConfigurationResult
}

func (r *recordingMapConfigurationRepo) CreateMapFromConfiguration(
	_ context.Context,
	ownerUserID string,
	authorName string,
	commit models.MapConfigurationCommit,
) (*models.MapConfigurationResult, error) {
	r.ownerUserID = ownerUserID
	r.authorName = authorName
	r.commit = commit
	if r.createErr != nil {
		return nil, r.createErr
	}
	if r.result == nil {
		r.result = &models.MapConfigurationResult{
			Map:      &models.Map{ID: commit.MapID, OwnerUserID: ownerUserID, RevisionNumber: 1},
			Revision: &models.MapRevision{ID: "rev_1", MapID: commit.MapID, RevisionNumber: 1, CreatedBy: ownerUserID},
		}
	}
	return r.result, nil
}

func (r *recordingMapConfigurationRepo) GetAsset(_ context.Context, assetID, userID string) (*models.MapAsset, error) {
	asset, ok := r.assets[assetID]
	if !ok {
		return nil, models.ErrMapAssetNotFound
	}
	if asset.OwnerUserID != userID {
		sharedMap, shared := r.mapsByID[asset.MapID]
		if !shared || sharedMap.Permission != models.PermissionEditor {
			return nil, models.ErrMapAccessDenied
		}
	}
	clone := *asset
	return &clone, nil
}

func (r *recordingMapConfigurationRepo) GetMap(_ context.Context, mapID, userID string) (*models.Map, error) {
	item, ok := r.mapsByID[mapID]
	if !ok {
		return nil, models.ErrMapNotFound
	}
	clone := *item
	return &clone, nil
}

func (r *recordingMapConfigurationRepo) SaveMapConfiguration(
	_ context.Context,
	mapID, userID, authorName string,
	commit models.MapConfigurationCommit,
) (*models.MapConfigurationResult, error) {
	r.saveMapID = mapID
	r.saveUserID = userID
	r.saveAuthor = authorName
	r.saveCommit = commit
	return r.saveResult, nil
}

func TestMapConfigurationService_CreateRequiresAtomicRepositoryCommit(t *testing.T) {
	sessionRoot := t.TempDir()
	uploadRoot := t.TempDir()
	primaryPath := writeMapAssetFixture(t, sessionRoot, "ds_primary")
	overlayPath := writeMapAssetFixture(t, sessionRoot, "ds_overlay")
	wantResult := &models.MapConfigurationResult{
		Map: &models.Map{
			ID:             "map_ground_floor",
			OwnerUserID:    "user_a",
			RevisionNumber: 1,
		},
		Revision: &models.MapRevision{
			ID:             "rev_ground_floor_1",
			MapID:          "map_ground_floor",
			RevisionNumber: 1,
			CreatedBy:      "user_a",
		},
	}
	repo := &recordingMapConfigurationRepo{
		result: wantResult,
		assets: map[string]*models.MapAsset{
			"ds_primary": {ID: "ds_primary", OwnerUserID: "user_a", StoragePath: primaryPath},
			"ds_overlay": {ID: "ds_overlay", OwnerUserID: "user_a", StoragePath: overlayPath},
		},
	}
	service := NewMapConfigurationService(repo, uploadRoot, sessionRoot)

	got, err := service.CreatePersonalMapFromConfiguration(
		context.Background(),
		&models.User{ID: "user_a", Name: "Alice Architect"},
		models.MapConfigurationCommit{
			Name:              "Ground Floor",
			PrimaryDatasetID:  "ds_primary",
			MemberAssetIDs:    []string{"ds_primary", "ds_overlay"},
			ConfigurationJSON: `{"active_view":"map"}`,
		},
	)
	if err != nil {
		t.Fatalf("CreatePersonalMapFromConfiguration returned error: %v", err)
	}
	if got != wantResult {
		t.Fatalf("result = %#v, want repository result %#v", got, wantResult)
	}
	if repo.ownerUserID != "user_a" || repo.authorName != "Alice Architect" {
		t.Fatalf("authenticated author = (%q, %q), want (%q, %q)", repo.ownerUserID, repo.authorName, "user_a", "Alice Architect")
	}
	if repo.commit.Name != "Ground Floor" || repo.commit.PrimaryDatasetID != "ds_primary" {
		t.Fatalf("commit identity = (%q, %q), want (%q, %q)", repo.commit.Name, repo.commit.PrimaryDatasetID, "Ground Floor", "ds_primary")
	}
	if len(repo.commit.MemberAssetIDs) != 2 || repo.commit.MemberAssetIDs[0] != "ds_primary" || repo.commit.MemberAssetIDs[1] != "ds_overlay" {
		t.Fatalf("commit members = %#v, want primary and overlay", repo.commit.MemberAssetIDs)
	}
	if repo.commit.ConfigurationJSON != `{"active_view":"map"}` {
		t.Fatalf("configuration = %q, want map view JSON", repo.commit.ConfigurationJSON)
	}
}

func writeMapAssetFixture(t *testing.T, root, assetID string) string {
	t.Helper()
	path := filepath.Join(root, "sess_1", "datasets", assetID, assetID+".png")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create asset fixture directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(assetID+"-bytes"), 0o644); err != nil {
		t.Fatalf("write asset fixture: %v", err)
	}
	return path
}

func TestMapConfigurationService_StagesDatasetTreeOutsideSessionStorage(t *testing.T) {
	sessionRoot := t.TempDir()
	uploadRoot := t.TempDir()
	sourceDir := filepath.Join(sessionRoot, "sess_1", "datasets", "ds_primary")
	sourceFile := filepath.Join(sourceDir, "floor.png")
	processedFile := filepath.Join(sourceDir, "processed", "tiles", "0", "0", "0.png")
	if err := os.MkdirAll(filepath.Dir(processedFile), 0o755); err != nil {
		t.Fatalf("create source tree: %v", err)
	}
	if err := os.WriteFile(sourceFile, []byte("source-bytes"), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}
	if err := os.WriteFile(processedFile, []byte("tile-bytes"), 0o644); err != nil {
		t.Fatalf("write processed file: %v", err)
	}

	repo := &recordingMapConfigurationRepo{assets: map[string]*models.MapAsset{
		"ds_primary": {
			ID:          "ds_primary",
			OwnerUserID: "user_a",
			StoragePath: sourceFile,
		},
	}}
	service := NewMapConfigurationService(repo, uploadRoot, sessionRoot)

	result, err := service.CreatePersonalMapFromConfiguration(
		context.Background(),
		&models.User{ID: "user_a", Name: "Alice Architect"},
		models.MapConfigurationCommit{
			Name:              "Ground Floor",
			PrimaryDatasetID:  "ds_primary",
			MemberAssetIDs:    []string{"ds_primary"},
			ConfigurationJSON: `{"active_view":"map"}`,
		},
	)
	if err != nil {
		t.Fatalf("CreatePersonalMapFromConfiguration returned error: %v", err)
	}
	if result.Map.ID == "" || repo.commit.MapID != result.Map.ID {
		t.Fatalf("committed map ID = %q, result map ID = %q", repo.commit.MapID, result.Map.ID)
	}
	if len(repo.commit.AssetLocations) != 1 {
		t.Fatalf("asset locations = %#v, want one durable location", repo.commit.AssetLocations)
	}
	durablePath := repo.commit.AssetLocations[0].StoragePath
	wantPrefix := filepath.Join(uploadRoot, "maps", "user_a", result.Map.ID, "assets", "ds_primary") + string(filepath.Separator)
	if !strings.HasPrefix(filepath.Clean(durablePath), wantPrefix) {
		t.Fatalf("durable path = %q, want prefix %q", durablePath, wantPrefix)
	}
	if strings.HasPrefix(filepath.Clean(durablePath), filepath.Clean(sessionRoot)+string(filepath.Separator)) {
		t.Fatalf("durable path remained inside session storage: %q", durablePath)
	}
	if got, readErr := os.ReadFile(durablePath); readErr != nil || string(got) != "source-bytes" {
		t.Fatalf("durable primary bytes = %q, err = %v", got, readErr)
	}
	durableProcessed := filepath.Join(filepath.Dir(durablePath), "processed", "tiles", "0", "0", "0.png")
	if got, readErr := os.ReadFile(durableProcessed); readErr != nil || string(got) != "tile-bytes" {
		t.Fatalf("durable processed bytes = %q, err = %v", got, readErr)
	}
	if _, statErr := os.Stat(sourceFile); statErr != nil {
		t.Fatalf("session source was removed: %v", statErr)
	}
}

func TestMapConfigurationService_RemovesOnlyNewStageOnRepositoryFailure(t *testing.T) {
	sessionRoot := t.TempDir()
	uploadRoot := t.TempDir()
	sourcePath := writeMapAssetFixture(t, sessionRoot, "ds_primary")
	repo := &recordingMapConfigurationRepo{
		assets: map[string]*models.MapAsset{
			"ds_primary": {ID: "ds_primary", OwnerUserID: "user_a", StoragePath: sourcePath},
		},
		createErr: errors.New("database commit failed"),
	}
	service := NewMapConfigurationService(repo, uploadRoot, sessionRoot)

	_, err := service.CreatePersonalMapFromConfiguration(
		context.Background(),
		&models.User{ID: "user_a", Name: "Alice Architect"},
		models.MapConfigurationCommit{
			Name:              "Ground Floor",
			PrimaryDatasetID:  "ds_primary",
			MemberAssetIDs:    []string{"ds_primary"},
			ConfigurationJSON: `{"active_view":"map"}`,
		},
	)
	if err == nil || err.Error() != "database commit failed" {
		t.Fatalf("error = %v, want repository commit failure", err)
	}
	if repo.commit.MapID == "" {
		t.Fatal("repository did not receive a generated map ID")
	}
	durableRoot := filepath.Join(uploadRoot, "maps", "user_a", repo.commit.MapID)
	if _, statErr := os.Stat(durableRoot); !os.IsNotExist(statErr) {
		t.Fatalf("failed save left durable map directory %q: %v", durableRoot, statErr)
	}
	if got, readErr := os.ReadFile(sourcePath); readErr != nil || string(got) != "ds_primary-bytes" {
		t.Fatalf("session source changed after failed commit: bytes=%q err=%v", got, readErr)
	}
}

func TestMapConfigurationService_SharedEditorSavesOwnersDurableAssetOnSameMap(t *testing.T) {
	uploadRoot := t.TempDir()
	sessionRoot := t.TempDir()
	mapID := "map_shared"
	durablePath := filepath.Join(uploadRoot, "maps", "user_a", mapID, "assets", "ds_primary", "floor.png")
	if err := os.MkdirAll(filepath.Dir(durablePath), 0o755); err != nil {
		t.Fatalf("create durable asset directory: %v", err)
	}
	if err := os.WriteFile(durablePath, []byte("owner-bytes"), 0o644); err != nil {
		t.Fatalf("write durable asset: %v", err)
	}
	wantResult := &models.MapConfigurationResult{
		Map:      &models.Map{ID: mapID, OwnerUserID: "user_a", RevisionNumber: 2},
		Revision: &models.MapRevision{ID: "rev_2", MapID: mapID, RevisionNumber: 2, CreatedBy: "user_b"},
	}
	repo := &recordingMapConfigurationRepo{
		mapsByID: map[string]*models.Map{
			mapID: {ID: mapID, OwnerUserID: "user_a", Permission: models.PermissionEditor, RevisionNumber: 1},
		},
		assets: map[string]*models.MapAsset{
			"ds_primary": {ID: "ds_primary", MapID: mapID, OwnerUserID: "user_a", StoragePath: durablePath},
		},
		saveResult: wantResult,
	}
	service := NewMapConfigurationService(repo, uploadRoot, sessionRoot)

	result, err := service.SaveExistingMapConfiguration(
		context.Background(),
		&models.User{ID: "user_b", Name: "Bob Builder"},
		mapID,
		models.MapConfigurationCommit{
			PrimaryDatasetID:  "ds_primary",
			MemberAssetIDs:    []string{"ds_primary"},
			ConfigurationJSON: `{"active_view":"map"}`,
			ExpectedRevision:  1,
		},
	)
	if err != nil {
		t.Fatalf("SaveExistingMapConfiguration returned error: %v", err)
	}
	if result != wantResult {
		t.Fatalf("result = %#v, want repository result %#v", result, wantResult)
	}
	if repo.saveMapID != mapID || repo.saveUserID != "user_b" || repo.saveAuthor != "Bob Builder" {
		t.Fatalf("save identity = (%q, %q, %q), want shared map and User B", repo.saveMapID, repo.saveUserID, repo.saveAuthor)
	}
	if len(repo.saveCommit.AssetLocations) != 1 {
		t.Fatalf("asset locations = %#v, want one owner asset", repo.saveCommit.AssetLocations)
	}
	location := repo.saveCommit.AssetLocations[0]
	if location.OwnerUserID != "user_a" || location.StoragePath != durablePath {
		t.Fatalf("asset location = %#v, want User A's durable path", location)
	}
	if got, readErr := os.ReadFile(durablePath); readErr != nil || string(got) != "owner-bytes" {
		t.Fatalf("owner asset changed: bytes=%q err=%v", got, readErr)
	}
}

package repository

import (
	"context"
	"testing"

	"github.com/malikharman0823/veracity-maps/models"
)

func TestMemoryMapRepository_SaveMapConfigurationCommitsAssetAndRevisionTogether(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryMapRepository()
	createdMap, err := repo.CreateMap(ctx, "user_a", "Ground Floor", "Initial")
	if err != nil {
		t.Fatalf("CreateMap returned error: %v", err)
	}
	if err := repo.RegisterAsset(ctx, &models.MapAsset{
		ID:          "ds_primary",
		MapID:       createdMap.ID,
		OwnerUserID: "user_a",
		StoragePath: `C:\temp\sessions\sess_1\datasets\ds_primary\floor.png`,
	}); err != nil {
		t.Fatalf("RegisterAsset returned error: %v", err)
	}

	result, err := repo.SaveMapConfiguration(ctx, createdMap.ID, "user_a", "Alice Architect", models.MapConfigurationCommit{
		Name:              "Ground Floor",
		Description:       "Durable baseline",
		PrimaryDatasetID:  "ds_primary",
		MemberAssetIDs:    []string{"ds_primary"},
		ConfigurationJSON: `{"active_view":"map"}`,
		ExpectedRevision:  1,
		AssetLocations: []models.MapAssetLocation{
			{AssetID: "ds_primary", OwnerUserID: "user_a", StoragePath: `C:\storage\maps\user_a\map_1\assets\ds_primary\floor.png`},
		},
	})
	if err != nil {
		t.Fatalf("SaveMapConfiguration returned error: %v", err)
	}
	if result.Map.RevisionNumber != 2 || result.Revision.RevisionNumber != 2 {
		t.Fatalf("revision numbers = map %d, revision %d; want 2", result.Map.RevisionNumber, result.Revision.RevisionNumber)
	}
	if result.Revision.CreatedBy != "user_a" || result.Revision.CreatedByName != "Alice Architect" {
		t.Fatalf("revision author = (%q, %q), want authenticated owner", result.Revision.CreatedBy, result.Revision.CreatedByName)
	}
	asset, err := repo.GetAsset(ctx, "ds_primary", "user_a")
	if err != nil {
		t.Fatalf("GetAsset returned error: %v", err)
	}
	wantPath := `C:\storage\maps\user_a\map_1\assets\ds_primary\floor.png`
	if asset.StoragePath != wantPath {
		t.Fatalf("asset storage path = %q, want %q", asset.StoragePath, wantPath)
	}

	_, err = repo.SaveMapConfiguration(ctx, createdMap.ID, "user_a", "Alice Architect", models.MapConfigurationCommit{
		ConfigurationJSON: `{"active_view":"database"}`,
		ExpectedRevision:  1,
	})
	if err != models.ErrOutdatedRevision {
		t.Fatalf("stale save error = %v, want ErrOutdatedRevision", err)
	}
	revisions, err := repo.ListMapRevisions(ctx, createdMap.ID, "user_a")
	if err != nil {
		t.Fatalf("ListMapRevisions returned error: %v", err)
	}
	if len(revisions) != 2 {
		t.Fatalf("revision count = %d, want 2 after rejected stale save", len(revisions))
	}
}

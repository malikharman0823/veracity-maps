package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/malikharman0823/veracity-maps/config"
	"github.com/malikharman0823/veracity-maps/db"
	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
	"github.com/malikharman0823/veracity-maps/services"
)

type legacyCandidate struct {
	SessionID  string
	SourcePath string
	Config     models.SavedConfig
}

type legacySkip struct {
	SessionID string
	Reason    string
}

type promotionCounts struct {
	Candidates int
	Promoted   int
	Skipped    int
	Existing   int
	Failed     int
}

func main() {
	source := flag.String("source", "", "session storage directory or saved_configs.json file")
	apply := flag.Bool("apply", false, "commit eligible promotions; omitted means dry-run")
	flag.Parse()
	if strings.TrimSpace(*source) == "" {
		log.Fatal("-source is required")
	}

	candidates, scanSkips, err := scanLegacyConfigs(*source)
	if err != nil {
		log.Fatalf("scan legacy configurations: %v", err)
	}
	counts := promotionCounts{Candidates: len(candidates), Skipped: len(scanSkips)}

	cfg := config.LoadConfig()
	if strings.TrimSpace(cfg.MSSQLConnStr) == "" {
		log.Fatal("MSSQL_CONNECTION_STRING is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	sqlDB, err := db.InitDB(ctx, cfg.MSSQLConnStr)
	if err != nil {
		log.Fatal("connect to MSSQL: durable database unavailable")
	}
	defer sqlDB.Close()
	if err := db.RunMigrations(ctx, sqlDB, "migrations"); err != nil {
		log.Fatalf("run migrations: %v", err)
	}

	mapRepo := repository.NewMSSQLMapRepository(sqlDB)
	service := services.NewMapConfigurationService(mapRepo, cfg.UploadDir, cfg.SessionStorageDir)
	for _, candidate := range candidates {
		outcome := promoteCandidate(ctx, sqlDB, service, candidate, *apply)
		switch outcome {
		case "promoted":
			counts.Promoted++
		case "existing":
			counts.Existing++
		case "failed":
			counts.Failed++
		default:
			counts.Skipped++
		}
	}
	mode := "dry-run"
	if *apply {
		mode = "apply"
	}
	log.Printf("legacy promotion %s complete: candidates=%d promoted=%d existing=%d skipped=%d failed=%d", mode, counts.Candidates, counts.Promoted, counts.Existing, counts.Skipped, counts.Failed)
}

func scanLegacyConfigs(source string) ([]legacyCandidate, []legacySkip, error) {
	absSource, err := filepath.Abs(source)
	if err != nil {
		return nil, nil, err
	}
	info, err := os.Stat(absSource)
	if err != nil {
		return nil, nil, err
	}
	paths := make([]string, 0)
	if !info.IsDir() {
		if filepath.Base(absSource) != "saved_configs.json" {
			return nil, nil, fmt.Errorf("source file must be named saved_configs.json")
		}
		paths = append(paths, absSource)
	} else {
		err = filepath.WalkDir(absSource, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if !entry.IsDir() && entry.Name() == "saved_configs.json" {
				paths = append(paths, path)
			}
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
	}
	sort.Strings(paths)
	candidates := make([]legacyCandidate, 0)
	skips := make([]legacySkip, 0)
	for _, path := range paths {
		sessionID := filepath.Base(filepath.Dir(path))
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, err
		}
		var configs []models.SavedConfig
		if err := json.Unmarshal(raw, &configs); err != nil {
			skips = append(skips, legacySkip{SessionID: sessionID, Reason: "malformed_json"})
			continue
		}
		for _, saved := range configs {
			candidates = append(candidates, legacyCandidate{SessionID: sessionID, SourcePath: path, Config: saved})
		}
	}
	return candidates, skips, nil
}

func resolveLegacyOwner(assetIDs []string, owners map[string]string) (string, string) {
	owner := ""
	for _, assetID := range assetIDs {
		assetOwner, ok := owners[assetID]
		if !ok {
			return "", "missing_asset"
		}
		if strings.TrimSpace(assetOwner) == "" {
			return "", "anonymous_owner"
		}
		if owner == "" {
			owner = assetOwner
			continue
		}
		if owner != assetOwner {
			return "", "mixed_owners"
		}
	}
	if owner == "" {
		return "", "missing_asset"
	}
	return owner, ""
}

func promoteCandidate(ctx context.Context, sqlDB *sql.DB, service *services.MapConfigurationService, candidate legacyCandidate, apply bool) string {
	if candidate.Config.ID == "" || strings.TrimSpace(candidate.Config.Name) == "" || strings.TrimSpace(candidate.Config.PrimaryDatasetID) == "" {
		return "skipped"
	}
	var markerStatus string
	err := sqlDB.QueryRowContext(ctx, `
		SELECT status FROM dbo.Legacy_Config_Promotions
		WHERE legacy_session_id = @p1 AND legacy_config_id = @p2;
	`, candidate.SessionID, candidate.Config.ID).Scan(&markerStatus)
	if err == nil {
		return "existing"
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "failed"
	}

	assetIDs := uniqueLegacyAssetIDs(candidate.Config.PrimaryDatasetID, candidate.Config.Members)
	owners := make(map[string]string, len(assetIDs))
	for _, assetID := range assetIDs {
		var owner string
		err := sqlDB.QueryRowContext(ctx, `SELECT owner_user_id FROM dbo.Map_Assets WHERE id = @p1;`, assetID).Scan(&owner)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return "failed"
		}
		owners[assetID] = owner
	}
	ownerID, reason := resolveLegacyOwner(assetIDs, owners)
	if reason != "" {
		if apply {
			_, _ = sqlDB.ExecContext(ctx, `
				INSERT INTO dbo.Legacy_Config_Promotions
				(legacy_session_id, legacy_config_id, status, reason_code, created_at, updated_at)
				VALUES (@p1, @p2, 'skipped', @p3, SYSDATETIMEOFFSET(), SYSDATETIMEOFFSET());
			`, candidate.SessionID, candidate.Config.ID, reason)
		}
		return "skipped"
	}
	if !apply {
		return "skipped"
	}
	if _, err := sqlDB.ExecContext(ctx, `
		INSERT INTO dbo.Legacy_Config_Promotions
		(legacy_session_id, legacy_config_id, status, created_at, updated_at)
		VALUES (@p1, @p2, 'processing', SYSDATETIMEOFFSET(), SYSDATETIMEOFFSET());
	`, candidate.SessionID, candidate.Config.ID); err != nil {
		return "existing"
	}

	var ownerName string
	if err := sqlDB.QueryRowContext(ctx, `SELECT ISNULL(name, '') FROM dbo.Users WHERE id = @p1;`, ownerID).Scan(&ownerName); err != nil {
		markPromotionFailed(ctx, sqlDB, candidate, "owner_not_found")
		return "failed"
	}
	snapshot, err := json.Marshal(candidate.Config)
	if err != nil {
		markPromotionFailed(ctx, sqlDB, candidate, "malformed_config")
		return "failed"
	}
	result, err := service.CreatePersonalMapFromConfiguration(ctx, &models.User{ID: ownerID, Name: ownerName}, models.MapConfigurationCommit{
		Name:              candidate.Config.Name,
		Description:       "Promoted from legacy session configuration",
		PrimaryDatasetID:  candidate.Config.PrimaryDatasetID,
		MemberAssetIDs:    candidate.Config.Members,
		ConfigurationJSON: string(snapshot),
	})
	if err != nil {
		markPromotionFailed(ctx, sqlDB, candidate, "promotion_failed")
		return "failed"
	}
	if _, err := sqlDB.ExecContext(ctx, `
		UPDATE dbo.Legacy_Config_Promotions
		SET map_id = @p1, status = 'completed', reason_code = NULL, updated_at = SYSDATETIMEOFFSET()
		WHERE legacy_session_id = @p2 AND legacy_config_id = @p3;
	`, result.Map.ID, candidate.SessionID, candidate.Config.ID); err != nil {
		return "failed"
	}
	return "promoted"
}

func markPromotionFailed(ctx context.Context, sqlDB *sql.DB, candidate legacyCandidate, reason string) {
	_, _ = sqlDB.ExecContext(ctx, `
		UPDATE dbo.Legacy_Config_Promotions
		SET status = 'failed', reason_code = @p1, updated_at = SYSDATETIMEOFFSET()
		WHERE legacy_session_id = @p2 AND legacy_config_id = @p3;
	`, reason, candidate.SessionID, candidate.Config.ID)
}

func uniqueLegacyAssetIDs(primary string, members []string) []string {
	seen := make(map[string]struct{}, len(members)+1)
	result := make([]string, 0, len(members)+1)
	for _, id := range append([]string{primary}, members...) {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

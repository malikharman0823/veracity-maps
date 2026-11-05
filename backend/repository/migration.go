package repository

import (
	"database/sql"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/malikharman0823/veracity-maps/models"
)

// MigrateJSONToMSSQL inspects storage/users and storage/projects
// and safely copies records into MSSQL Server 2022 if they are not already present.
// It also creates a timestamped backup directory of the JSON files.
// Deprecated: legacy offline tooling only. Server startup never imports JSON;
// user-owned ownership and token cutover require the separately verified migration path.
func MigrateJSONToMSSQL(storageDir string, db *sql.DB) error {
	if db == nil {
		return nil
	}

	backupDir := filepath.Join(storageDir, "backups", time.Now().Format("20060102_150405"))
	_ = os.MkdirAll(backupDir, 0755)

	// 1. Migrate Users
	usersFile := filepath.Join(storageDir, "users", "users.json")
	if data, err := os.ReadFile(usersFile); err == nil && len(data) > 0 {
		var userList []*models.User
		if err := json.Unmarshal(data, &userList); err == nil {
			_ = copyFile(usersFile, filepath.Join(backupDir, "users.json"))
			for _, u := range userList {
				var count int
				_ = db.QueryRow("SELECT COUNT(*) FROM Users WHERE id = @p1", u.ID).Scan(&count)
				if count == 0 {
					_, err := db.Exec(`
						INSERT INTO Users (id, email, name, password_hash, salt, created_at, updated_at)
						VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7);
					`, u.ID, strings.ToLower(u.Email), u.Name, u.PasswordHash, u.Salt, u.CreatedAt, u.UpdatedAt)
					if err != nil {
						log.Printf("[MIGRATION WARNING] Failed migrating user %s: %v", u.Email, err)
					} else {
						log.Printf("[MIGRATION] Migrated user: %s (%s)", u.Name, u.Email)
					}
				}
			}
		}
	}

	// 2. Migrate Projects
	projBaseDir := filepath.Join(storageDir, "projects")
	if entries, err := os.ReadDir(projBaseDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() || entry.Name() == "backups" {
				continue
			}
			projID := entry.Name()
			projFile := filepath.Join(projBaseDir, projID, "project.json")
			if pData, pErr := os.ReadFile(projFile); pErr == nil && len(pData) > 0 {
				var p models.UserProject
				if jErr := json.Unmarshal(pData, &p); jErr == nil && p.ID != "" {
					var count int
					_ = db.QueryRow("SELECT COUNT(*) FROM Projects WHERE id = @p1", p.ID).Scan(&count)
					if count == 0 {
						datasetBytes, _ := json.Marshal(p.DatasetIDs)
						_, err := db.Exec(`
							INSERT INTO Projects (id, user_id, org_id, owner_type, name, description, primary_dataset_id, dataset_ids, created_at, updated_at)
							VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7, @p8, @p9, @p10);
						`, p.ID, p.UserID, nil, "personal", p.Name, p.Description, p.PrimaryDatasetID, string(datasetBytes), p.CreatedAt, p.UpdatedAt)
						if err != nil {
							log.Printf("[MIGRATION WARNING] Failed migrating project %s: %v", p.ID, err)
						} else {
							log.Printf("[MIGRATION] Migrated project: %s (%s)", p.Name, p.ID)
						}
					}

					// Members
					membersFile := filepath.Join(projBaseDir, projID, "members.json")
					if mData, mErr := os.ReadFile(membersFile); mErr == nil && len(mData) > 0 {
						var mList []*models.ProjectMember
						if jErr := json.Unmarshal(mData, &mList); jErr == nil {
							for _, m := range mList {
								var mCount int
								_ = db.QueryRow("SELECT COUNT(*) FROM ProjectMembers WHERE id = @p1", m.ID).Scan(&mCount)
								if mCount == 0 {
									_, _ = db.Exec(`
										INSERT INTO ProjectMembers (id, project_id, user_id, user_email, user_name, role, invitation_status, invited_by, created_at, updated_at)
										VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7, @p8, @p9, @p10);
									`, m.ID, m.ProjectID, m.UserID, m.UserEmail, m.UserName, string(m.Role), string(m.InvitationStatus), m.InvitedBy, m.CreatedAt, m.UpdatedAt)
								}
							}
						}
					}
				}
			}
		}
	}

	return nil
}

func copyFile(src, dst string) error {
	in, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, in, 0644)
}

package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	_ "github.com/microsoft/go-mssqldb"
	_ "github.com/microsoft/go-mssqldb/sharedmemory"
)

// InitDB connects to Microsoft SQL Server 2022 using the provided connection string.
func InitDB(ctx context.Context, connStr string) (*sql.DB, error) {
	if strings.TrimSpace(connStr) == "" {
		return nil, fmt.Errorf("MSSQL connection string is empty")
	}

	db, err := sql.Open("sqlserver", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open mssql connection: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(15 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping mssql database: %w", err)
	}

	return db, nil
}

// GO is a client batch separator, not T-SQL. Match whole lines, including CRLF.
var migrationBatchSeparator = regexp.MustCompile(`(?im)^[\t ]*GO[\t ]*(?:--[^\r\n]*)?\r?$`)

// RunMigrations applies sorted SQL files once, atomically recording each filename.
func RunMigrations(ctx context.Context, db *sql.DB, migrationsDir string) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}

	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("failed to read migrations dir: %w", err)
	}
	if _, err := db.ExecContext(ctx, `
		IF OBJECT_ID(N'dbo.SchemaMigrations', N'U') IS NULL
		CREATE TABLE dbo.SchemaMigrations (
			version NVARCHAR(255) NOT NULL PRIMARY KEY,
			applied_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET()
		);`); err != nil {
		return fmt.Errorf("failed to initialize migration tracking: %w", err)
	}

	// os.ReadDir returns entries sorted by filename.
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("failed to begin migration %s: %w", entry.Name(), err)
		}
		if err := applyMigration(ctx, tx, migrationsDir, entry.Name()); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration %s: %w", entry.Name(), err)
		}
	}
	return nil
}

func applyMigration(ctx context.Context, tx *sql.Tx, migrationsDir, version string) error {
	var applied int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM dbo.SchemaMigrations WITH (UPDLOCK, HOLDLOCK) WHERE version = @p1", version).Scan(&applied); err != nil {
		return fmt.Errorf("failed to query migration %s: %w", version, err)
	}
	if applied != 0 {
		return nil
	}
	filePath := filepath.Join(migrationsDir, version)
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read migration %s: %w", version, err)
	}

	// Split on GO statements if present, otherwise execute whole batch
	batches := migrationBatchSeparator.Split(string(content), -1)
	for _, batch := range batches {
		sqlText := strings.TrimSpace(batch)
		if sqlText == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, sqlText); err != nil {
			return fmt.Errorf("failed executing migration %s: %w", version, err)
		}
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO dbo.SchemaMigrations (version) VALUES (@p1)", version)
	if err != nil {
		return fmt.Errorf("failed recording migration %s: %w", version, err)
	}
	return nil
}

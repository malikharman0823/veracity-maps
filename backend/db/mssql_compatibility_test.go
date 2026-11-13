package db_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/malikharman0823/veracity-maps/db"
)

func TestMigrationsAcceptCanonicalMapsCreatedBeforeLedgerCatchup(t *testing.T) {
	masterConnection := os.Getenv("MSSQL_TEST_CONNECTION_STRING")
	if masterConnection == "" {
		t.Skip("MSSQL_TEST_CONNECTION_STRING is not set; a dedicated SQL Server test database is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	master, err := db.InitDB(ctx, masterConnection)
	if err != nil {
		t.Fatal("connect to MSSQL test server: connection details withheld")
	}

	databaseName := fmt.Sprintf("veracity_migration_compat_%d", time.Now().UnixNano())
	if _, err := master.ExecContext(ctx, "CREATE DATABASE ["+databaseName+"]"); err != nil {
		t.Fatalf("create disposable database: %v", err)
	}
	_ = master.Close()
	t.Cleanup(func() {
		cleanupDB, cleanupErr := db.InitDB(context.Background(), masterConnection)
		if cleanupErr == nil {
			_, _ = cleanupDB.Exec("ALTER DATABASE [" + databaseName + "] SET SINGLE_USER WITH ROLLBACK IMMEDIATE; DROP DATABASE [" + databaseName + "]")
			_ = cleanupDB.Close()
		}
	})

	databaseConnection := regexp.MustCompile(`(?i)(database\s*=\s*)[^;]+`).ReplaceAllString(masterConnection, "${1}"+databaseName)
	database, err := db.InitDB(ctx, databaseConnection)
	if err != nil {
		t.Fatal("connect to disposable MSSQL database: connection details withheld")
	}
	defer database.Close()

	initialDir := t.TempDir()
	initialMigration, err := os.ReadFile(filepath.Join("..", "migrations", "001_initial_schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(initialDir, "001_initial_schema.sql"), initialMigration, 0600); err != nil {
		t.Fatal(err)
	}
	if err := db.RunMigrations(ctx, database, initialDir); err != nil {
		t.Fatalf("apply initial migration: %v", err)
	}

	if _, err := database.ExecContext(ctx, `
		CREATE TABLE dbo.Maps (
			id NVARCHAR(64) NOT NULL PRIMARY KEY,
			owner_user_id NVARCHAR(64) NOT NULL,
			name NVARCHAR(255) NOT NULL,
			description NVARCHAR(2048) NULL,
			current_revision_id NVARCHAR(64) NULL,
			revision_number INT NOT NULL DEFAULT 1,
			created_by NVARCHAR(64) NOT NULL,
			created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
			updated_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET()
		);
	`); err != nil {
		t.Fatalf("seed canonical Maps table: %v", err)
	}

	if err := db.RunMigrations(ctx, database, filepath.Join("..", "migrations")); err != nil {
		t.Fatalf("migration catch-up with canonical Maps table: %v", err)
	}

	var applied int
	if err := database.QueryRowContext(ctx, "SELECT COUNT(*) FROM dbo.SchemaMigrations WHERE version = '002_user_owned_persistence.sql'").Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if applied != 1 {
		t.Fatalf("migration 002 ledger count = %d, want 1", applied)
	}
}

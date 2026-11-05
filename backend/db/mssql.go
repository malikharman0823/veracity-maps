package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/microsoft/go-mssqldb"
)

// InitDB connects to Microsoft SQL Server 2022 using the provided connection string.
func InitDB(connStr string) (*sql.DB, error) {
	if connStr == "" {
		return nil, fmt.Errorf("MSSQL connection string is empty")
	}

	db, err := sql.Open("sqlserver", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open mssql connection: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(15 * time.Minute)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping mssql database: %w", err)
	}

	return db, nil
}

// RunMigrations runs all migration scripts in the migrations folder idempotently.
func RunMigrations(db *sql.DB, migrationsDir string) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}

	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("failed to read migrations dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		filePath := filepath.Join(migrationsDir, entry.Name())
		content, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read migration %s: %w", entry.Name(), err)
		}

		// Split on GO statements if present, otherwise execute whole batch
		batches := strings.Split(string(content), "\nGO")
		for _, batch := range batches {
			sqlText := strings.TrimSpace(batch)
			if sqlText == "" {
				continue
			}
			if _, err := db.Exec(sqlText); err != nil {
				return fmt.Errorf("failed executing migration %s: %w", entry.Name(), err)
			}
		}
		log.Printf("[MSSQL] Applied migration: %s", entry.Name())
	}

	return nil
}

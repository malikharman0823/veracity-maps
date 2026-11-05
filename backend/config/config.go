package config

import (
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type Config struct {
	Port               string
	Host               string
	SessionStorageDir  string
	UploadDir          string
	ProcessedDir       string
	MaxUploadSizeBytes int64
	SessionIdleTimeout time.Duration
	SessionMaxLifetime time.Duration
	MSSQLConnStr       string
}

func LoadConfig() *Config {
	port := getEnv("PORT", "8000")
	host := getEnv("HOST", "0.0.0.0")

	baseDir, err := os.Getwd()
	if err != nil || os.Getenv("VERCEL") != "" || os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != "" {
		baseDir = os.TempDir()
	}

	sessionStorageDir := getEnv("SESSION_STORAGE_DIR", filepath.Join(baseDir, "temp", "sessions"))
	uploadDir := getEnv("UPLOAD_DIR", filepath.Join(baseDir, "storage", "uploads"))
	processedDir := getEnv("PROCESSED_DIR", filepath.Join(baseDir, "storage", "processed"))

	maxUploadBytes := int64(500 * 1024 * 1024) // 500 MB default
	if envMax := os.Getenv("MAX_UPLOAD_SIZE_BYTES"); envMax != "" {
		if val, err := strconv.ParseInt(envMax, 10, 64); err == nil && val > 0 {
			maxUploadBytes = val
		}
	}

	mssqlConn := os.Getenv("MSSQL_CONNECTION_STRING")
	if mssqlConn == "" {
		mssqlHost := getEnv("MSSQL_HOST", "")
		if mssqlHost != "" {
			mssqlPort := getEnv("MSSQL_PORT", "1433")
			mssqlUser := getEnv("MSSQL_USER", "")
			mssqlPassword := getEnv("MSSQL_PASSWORD", "")
			mssqlDatabase := getEnv("MSSQL_DATABASE", "veracity_maps")

			if mssqlUser != "" {
				mssqlConn = "sqlserver://" + mssqlUser + ":" + mssqlPassword + "@" + mssqlHost + ":" + mssqlPort + "?database=" + mssqlDatabase
			} else {
				mssqlConn = "server=" + mssqlHost + ";database=" + mssqlDatabase + ";trusted_connection=yes"
			}
		}
	}

	return &Config{
		Port:               port,
		Host:               host,
		SessionStorageDir:  sessionStorageDir,
		UploadDir:          uploadDir,
		ProcessedDir:       processedDir,
		MaxUploadSizeBytes: maxUploadBytes,
		SessionIdleTimeout: 24 * time.Hour,
		SessionMaxLifetime: 7 * 24 * time.Hour,
		MSSQLConnStr:       mssqlConn,
	}
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

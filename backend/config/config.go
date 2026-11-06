package config

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	Environment        string
	ResendAPIKey       string
	ResendFrom         string
	AppPublicURL       string
	WorkspaceTTL       time.Duration
}

func (c *Config) Validate() error {
	if strings.TrimSpace(c.MSSQLConnStr) == "" && c.Environment != "test" {
		return errors.New("MSSQL_CONNECTION_STRING is required outside tests")
	}
	if c.WorkspaceTTL <= 0 {
		return errors.New("WORKSPACE_TTL must be positive")
	}
	if c.Environment != "test" {
		for _, setting := range []struct{ name, value string }{
			{"RESEND_API_KEY", c.ResendAPIKey}, {"RESEND_FROM_EMAIL", c.ResendFrom}, {"APP_PUBLIC_URL", c.AppPublicURL},
		} {
			if strings.TrimSpace(setting.value) == "" {
				return errors.New(setting.name + " is required outside tests")
			}
		}
	}
	return nil
}

func LoadConfig() *Config {
	workspaceTTL, err := time.ParseDuration(getEnv("WORKSPACE_TTL", "24h"))
	if err != nil {
		workspaceTTL = 0 // Validate rejects malformed durations before startup.
	}
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
		Environment:        getEnv("APP_ENV", "development"),
		ResendAPIKey:       os.Getenv("RESEND_API_KEY"),
		ResendFrom:         os.Getenv("RESEND_FROM_EMAIL"),
		AppPublicURL:       os.Getenv("APP_PUBLIC_URL"),
		WorkspaceTTL:       workspaceTTL,
	}
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

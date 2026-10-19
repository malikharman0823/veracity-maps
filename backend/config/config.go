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
}

func LoadConfig() *Config {
	port := getEnv("PORT", "8000")
	host := getEnv("HOST", "0.0.0.0")

	baseDir, err := os.Getwd()
	if err != nil {
		baseDir = "."
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

	return &Config{
		Port:               port,
		Host:               host,
		SessionStorageDir:  sessionStorageDir,
		UploadDir:          uploadDir,
		ProcessedDir:       processedDir,
		MaxUploadSizeBytes: maxUploadBytes,
		SessionIdleTimeout: 24 * time.Hour,
		SessionMaxLifetime: 7 * 24 * time.Hour,
	}
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

package config

import (
	"bufio"
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
	SMTPHost           string
	SMTPPort           int
	SMTPUsername       string
	SMTPPassword       string
	SMTPFrom           string
	SMTPUseTLS         bool
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
		if strings.TrimSpace(c.AppPublicURL) == "" {
			return errors.New("APP_PUBLIC_URL is required outside tests")
		}
		if strings.TrimSpace(c.SMTPHost) == "" && strings.TrimSpace(c.ResendAPIKey) == "" {
			return errors.New("SMTP_HOST (or RESEND_API_KEY) is required outside tests")
		}
		if strings.TrimSpace(c.SMTPFrom) == "" && strings.TrimSpace(c.ResendFrom) == "" {
			return errors.New("SMTP_FROM_EMAIL (or RESEND_FROM_EMAIL) is required outside tests")
		}
	}
	return nil
}

func loadEnvFiles() {
	if os.Getenv("APP_ENV") == "test" {
		return
	}
	for _, p := range []string{".env", "../.env"} {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				k := strings.TrimSpace(parts[0])
				v := strings.TrimSpace(parts[1])
				if len(v) >= 2 && ((v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'')) {
					v = v[1 : len(v)-1]
				}
				if os.Getenv(k) == "" {
					_ = os.Setenv(k, v)
				}
			}
		}
		if err := scanner.Err(); err != nil {
			// Ignore read error from truncated env file
		}
		_ = f.Close()
	}
}

func LoadConfig() *Config {
	loadEnvFiles()
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

	smtpPort := 587
	if pStr := getEnv("SMTP_PORT", ""); pStr != "" {
		if pVal, err := strconv.Atoi(pStr); err == nil && pVal > 0 {
			smtpPort = pVal
		}
	}
	smtpUseTLS := false
	if tlsStr := getEnv("SMTP_USE_TLS", ""); tlsStr != "" {
		smtpUseTLS = (tlsStr == "true" || tlsStr == "1" || tlsStr == "yes")
	} else if smtpPort == 465 {
		smtpUseTLS = true
	}

	smtpHost := getEnv("SMTP_HOST", "")
	smtpUser := getEnv("SMTP_USERNAME", getEnv("SMTP_USER", ""))
	smtpPass := getEnv("SMTP_PASSWORD", getEnv("SMTP_PASS", ""))
	smtpFrom := getEnv("SMTP_FROM_EMAIL", getEnv("SMTP_FROM", ""))
	resendFrom := os.Getenv("RESEND_FROM_EMAIL")
	if smtpFrom == "" {
		smtpFrom = resendFrom
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
		SMTPHost:           smtpHost,
		SMTPPort:           smtpPort,
		SMTPUsername:       smtpUser,
		SMTPPassword:       smtpPass,
		SMTPFrom:           smtpFrom,
		SMTPUseTLS:         smtpUseTLS,
		ResendAPIKey:       os.Getenv("RESEND_API_KEY"),
		ResendFrom:         resendFrom,
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

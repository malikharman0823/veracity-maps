# Package: `config`

Located in [`backend/config/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/config)

The `config` package defines the configuration parameters and environment variable defaults for the Veracity Maps backend.

---

## File: `config.go`

### Purpose
Reads configuration settings from the process environment (with fallback defaults) to configure network binding, storage roots, file size limits, and session retention policies.

### Public Types & Functions
- **`type Config struct`**:
  - `Port string`: TCP port to listen on (Default: `"8000"`).
  - `Host string`: Interface to bind to (Default: `"0.0.0.0"`).
  - `SessionStorageDir string`: Root directory for ephemeral session workspaces (Default: `<cwd>/temp/sessions`).
  - `UploadDir string`: Default upload path (Default: `<cwd>/storage/uploads`).
  - `ProcessedDir string`: Default processed path (Default: `<cwd>/storage/processed`).
  - `MaxUploadSizeBytes int64`: Maximum multipart payload size (Default: `524288000` / 500 MB).
  - `SessionIdleTimeout time.Duration`: Inactivity duration before a session expires (Default: `24 * time.Hour`).
  - `SessionMaxLifetime time.Duration`: Maximum absolute session lifespan (Default: `7 * 24 * time.Hour`).
- **`func LoadConfig() *Config`**:
  - Inspects environment variables and returns a `Config`. Call `Validate()` before using it at startup.
- **`func (c *Config) Validate() error`**:
  - Requires MSSQL outside `APP_ENV=test` and a positive `WorkspaceTTL`. Malformed duration values fail validation.
- **`func getEnv(key, fallback string) string`**:
  - Private helper that checks `os.Getenv(key)` and returns the fallback string if empty.

---

## Environment Variables Summary

| Variable | Default Value | Description |
| :--- | :--- | :--- |
| `PORT` | `8000` | Port for the HTTP listener. |
| `HOST` | `0.0.0.0` | Host IP binding. |
| `SESSION_STORAGE_DIR` | `./temp/sessions` | Filesystem root for temporary session workspaces. |
| `MAX_UPLOAD_SIZE_BYTES` | `524288000` | Max multipart body upload limit (500 MB). |
| `APP_ENV` | `development` | Only isolated `test` configuration may omit MSSQL. |
| `MSSQL_CONNECTION_STRING` | empty | Required database connection; alternatively supplied by the existing discrete `MSSQL_*` variables. |
| `WORKSPACE_TTL` | `24h` | Positive Go duration for the temporary workspace lifecycle. |
| `RESEND_API_KEY` | empty | Mail provider key, loaded for the later verification/invitation cutover. |
| `RESEND_FROM_EMAIL` | empty | Verification/invitation sender, loaded as `ResendFrom`. |
| `APP_PUBLIC_URL` | empty | Public frontend URL used by verification/invitation links. |

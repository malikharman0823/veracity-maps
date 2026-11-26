package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	// ErrMissingTenantContext is returned when a tenant-scoped DB operation is attempted without a verified user ID.
	ErrMissingTenantContext = errors.New("authenticated user_id is required for RLS-scoped database execution")
)

const (
	setUserContextSQL   = `EXEC sys.sp_set_session_context @key = N'user_id', @value = @p1, @read_only = 0;`
	clearUserContextSQL = `EXEC sys.sp_set_session_context @key = N'user_id', @value = NULL, @read_only = 0;`
	setAdminContextSQL  = `EXEC sys.sp_set_session_context @key = N'is_admin', @value = @p1, @read_only = 0;`
)

// WithUserContext acquires a single connection from the shared MSSQL pool, binds the verified
// authenticated userID into SQL Server SESSION_CONTEXT(N'user_id') so Row-Level Security
// predicates enforce per-user isolation, runs fn, and unconditionally clears SESSION_CONTEXT
// before returning the connection to the pool (preventing cross-user context leakage).
func WithUserContext(ctx context.Context, sqlDB *sql.DB, userID string, fn func(ctx context.Context, conn *sql.Conn) error) error {
	cleanUserID := strings.TrimSpace(userID)
	if cleanUserID == "" {
		return ErrMissingTenantContext
	}
	if sqlDB == nil {
		return errors.New("database handle is nil")
	}

	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire pooled sql connection: %w", err)
	}

	// Always clear SESSION_CONTEXT with a fresh short-lived context even if the caller's ctx was canceled.
	defer func() {
		resetCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = conn.ExecContext(resetCtx, clearUserContextSQL)
		_, _ = conn.ExecContext(resetCtx, setAdminContextSQL, 0)
		_ = conn.Close()
	}()

	if _, err := conn.ExecContext(ctx, setAdminContextSQL, 0); err != nil {
		return fmt.Errorf("reset admin session context: %w", err)
	}
	if _, err := conn.ExecContext(ctx, setUserContextSQL, cleanUserID); err != nil {
		return fmt.Errorf("set RLS user_id session context: %w", err)
	}

	return fn(ctx, conn)
}

// WithUserTx runs fn inside a database transaction on a single pooled connection with
// SESSION_CONTEXT(N'user_id') bound to the verified userID and guaranteed cleanup on exit.
func WithUserTx(ctx context.Context, sqlDB *sql.DB, userID string, fn func(ctx context.Context, tx *sql.Tx) error) error {
	return WithUserContext(ctx, sqlDB, userID, func(ctx context.Context, conn *sql.Conn) error {
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin RLS transaction: %w", err)
		}
		if err := fn(ctx, tx); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit RLS transaction: %w", err)
		}
		return nil
	})
}

// WithSystemAdminContext executes a privileged system operation (e.g. startup migrations or
// background cleanup) with is_admin=1 and unconditionally clears the context before releasing
// the connection back to the shared pool. Never call this for user-triggered API requests.
func WithSystemAdminContext(ctx context.Context, sqlDB *sql.DB, fn func(ctx context.Context, conn *sql.Conn) error) error {
	if sqlDB == nil {
		return errors.New("database handle is nil")
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire admin sql connection: %w", err)
	}
	defer func() {
		resetCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = conn.ExecContext(resetCtx, clearUserContextSQL)
		_, _ = conn.ExecContext(resetCtx, setAdminContextSQL, 0)
		_ = conn.Close()
	}()

	if _, err := conn.ExecContext(ctx, setAdminContextSQL, 1); err != nil {
		return fmt.Errorf("set admin session context: %w", err)
	}
	return fn(ctx, conn)
}

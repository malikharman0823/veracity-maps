package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/malikharman0823/veracity-maps/models"
)

type MSSQLAuthRepository struct {
	db *sql.DB
}

func NewMSSQLAuthRepository(db *sql.DB) *MSSQLAuthRepository {
	return &MSSQLAuthRepository{db: db}
}

func (r *MSSQLAuthRepository) Register(ctx context.Context, name, email, password string) (*models.User, error) {
	cleanEmail := strings.ToLower(strings.TrimSpace(email))
	cleanName := strings.TrimSpace(name)

	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM Users WHERE email = @email", sql.Named("email", cleanEmail)).Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("failed checking existing user: %w", err)
	}
	if count > 0 {
		return nil, ErrUserAlreadyExists
	}

	uid, err := models.GenerateRandomHex(16)
	if err != nil {
		return nil, err
	}
	userID := "usr_" + uid

	hash, err := models.HashPassword(password)
	if err != nil {
		return nil, err
	}
	now := time.Now()

	query := `
		INSERT INTO Users (id, email, name, password_hash, salt, created_at, updated_at)
		VALUES (@id, @email, @name, @hash, @salt, @created, @updated);
	`
	_, err = r.db.ExecContext(ctx, query, sql.Named("id", userID), sql.Named("email", cleanEmail), sql.Named("name", cleanName), sql.Named("hash", hash), sql.Named("salt", ""), sql.Named("created", now), sql.Named("updated", now))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") || strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return nil, ErrUserAlreadyExists
		}
		return nil, fmt.Errorf("failed inserting user: %w", err)
	}

	return &models.User{
		ID:           userID,
		Email:        cleanEmail,
		Name:         cleanName,
		PasswordHash: hash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

func (r *MSSQLAuthRepository) Authenticate(ctx context.Context, email, password string) (*models.User, error) {
	cleanEmail := strings.ToLower(strings.TrimSpace(email))

	var u models.User
	query := "SELECT id, email, name, password_hash, salt, created_at, updated_at FROM Users WHERE email = @email"
	err := r.db.QueryRowContext(ctx, query, sql.Named("email", cleanEmail)).Scan(
		&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.Salt, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvalidPassword
		}
		return nil, fmt.Errorf("failed querying user: %w", err)
	}

	if strings.HasPrefix(u.PasswordHash, "$2") {
		ok, err := models.VerifyPassword(password, u.PasswordHash)
		if err != nil {
			return nil, fmt.Errorf("failed verifying password: %w", err)
		}
		if !ok {
			return nil, ErrInvalidPassword
		}
	} else {
		if !models.VerifyLegacyPassword(password, u.Salt, u.PasswordHash) {
			return nil, ErrInvalidPassword
		}
		hash, err := models.HashPassword(password)
		if err != nil {
			return nil, err
		}
		now := time.Now()
		_, err = r.db.ExecContext(ctx, "UPDATE Users SET password_hash = @hash, salt = '', updated_at = @updated WHERE id = @id", sql.Named("hash", hash), sql.Named("updated", now), sql.Named("id", u.ID))
		if err != nil {
			return nil, fmt.Errorf("failed upgrading password: %w", err)
		}
		u.PasswordHash, u.Salt, u.UpdatedAt = hash, "", now
	}

	return &u, nil
}

func (r *MSSQLAuthRepository) CreateToken(ctx context.Context, userID string, duration time.Duration) (*models.AuthToken, error) {
	tokHex, err := models.GenerateRandomHex(32)
	if err != nil {
		return nil, err
	}

	id, err := models.GenerateRandomHex(16)
	if err != nil {
		return nil, err
	}
	token := "vmap_" + tokHex
	now := time.Now()
	expiresAt := now.Add(duration)

	query := `
		INSERT INTO AuthTokens (id, token_hash, user_id, expires_at, created_at)
		VALUES (@id, @hash, @user, @expires, @created);
	`
	_, err = r.db.ExecContext(ctx, query, sql.Named("id", "tok_"+id), sql.Named("hash", models.HashToken(token)), sql.Named("user", userID), sql.Named("expires", expiresAt), sql.Named("created", now))
	if err != nil {
		return nil, fmt.Errorf("failed inserting auth token: %w", err)
	}

	return &models.AuthToken{
		ID:        "tok_" + id,
		Token:     token,
		UserID:    userID,
		ExpiresAt: expiresAt,
		CreatedAt: now,
	}, nil
}

func (r *MSSQLAuthRepository) ValidateToken(ctx context.Context, tokenStr string) (*models.User, error) {
	now := time.Now()
	query := `
		SELECT u.id, u.email, u.name, u.password_hash, u.salt, u.created_at, u.updated_at
		FROM AuthTokens t
		INNER JOIN Users u ON t.user_id = u.id
		WHERE t.token_hash = @hash AND t.expires_at > @now AND t.revoked_at IS NULL;
	`
	var u models.User
	err := r.db.QueryRowContext(ctx, query, sql.Named("hash", models.HashToken(tokenStr)), sql.Named("now", now)).Scan(
		&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.Salt, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvalidToken
		}
		return nil, fmt.Errorf("failed validating token: %w", err)
	}

	return &u, nil
}

func (r *MSSQLAuthRepository) RevokeToken(ctx context.Context, tokenStr string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE AuthTokens SET revoked_at = @now WHERE token_hash = @hash AND revoked_at IS NULL", sql.Named("now", time.Now()), sql.Named("hash", models.HashToken(tokenStr)))
	if err != nil {
		return fmt.Errorf("failed revoking token: %w", err)
	}
	return nil
}

func (r *MSSQLAuthRepository) GetUserByID(ctx context.Context, userID string) (*models.User, error) {
	var u models.User
	query := "SELECT id, email, name, password_hash, salt, created_at, updated_at FROM Users WHERE id = @id"
	err := r.db.QueryRowContext(ctx, query, sql.Named("id", userID)).Scan(
		&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.Salt, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed fetching user by id: %w", err)
	}
	return &u, nil
}

func (r *MSSQLAuthRepository) FindUserByEmail(ctx context.Context, email string) (*models.User, error) {
	cleanEmail := strings.ToLower(strings.TrimSpace(email))
	var u models.User
	query := "SELECT id, email, name, password_hash, salt, created_at, updated_at FROM Users WHERE email = @email"
	err := r.db.QueryRowContext(ctx, query, sql.Named("email", cleanEmail)).Scan(
		&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.Salt, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed finding user by email: %w", err)
	}
	return &u, nil
}

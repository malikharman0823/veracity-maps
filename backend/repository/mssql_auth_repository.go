package repository

import (
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

func (r *MSSQLAuthRepository) Register(name, email, password string) (*models.User, error) {
	cleanEmail := strings.ToLower(strings.TrimSpace(email))
	cleanName := strings.TrimSpace(name)

	var count int
	err := r.db.QueryRow("SELECT COUNT(*) FROM Users WHERE email = @p1", cleanEmail).Scan(&count)
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

	salt, err := models.GenerateRandomHex(16)
	if err != nil {
		return nil, err
	}

	hash := models.HashPassword(password, salt)
	now := time.Now()

	query := `
		INSERT INTO Users (id, email, name, password_hash, salt, created_at, updated_at)
		VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7);
	`
	_, err = r.db.Exec(query, userID, cleanEmail, cleanName, hash, salt, now, now)
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
		Salt:         salt,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

func (r *MSSQLAuthRepository) Authenticate(email, password string) (*models.User, error) {
	cleanEmail := strings.ToLower(strings.TrimSpace(email))

	var u models.User
	query := "SELECT id, email, name, password_hash, salt, created_at, updated_at FROM Users WHERE email = @p1"
	err := r.db.QueryRow(query, cleanEmail).Scan(
		&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.Salt, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvalidPassword
		}
		return nil, fmt.Errorf("failed querying user: %w", err)
	}

	if !models.VerifyPassword(password, u.Salt, u.PasswordHash) {
		return nil, ErrInvalidPassword
	}

	return &u, nil
}

func (r *MSSQLAuthRepository) CreateToken(userID string, duration time.Duration) (*models.AuthToken, error) {
	tokHex, err := models.GenerateRandomHex(32)
	if err != nil {
		return nil, err
	}

	token := "vmap_" + tokHex
	now := time.Now()
	expiresAt := now.Add(duration)

	query := `
		INSERT INTO AuthTokens (token, user_id, expires_at, created_at)
		VALUES (@p1, @p2, @p3, @p4);
	`
	_, err = r.db.Exec(query, token, userID, expiresAt, now)
	if err != nil {
		return nil, fmt.Errorf("failed inserting auth token: %w", err)
	}

	return &models.AuthToken{
		Token:     token,
		UserID:    userID,
		ExpiresAt: expiresAt,
		CreatedAt: now,
	}, nil
}

func (r *MSSQLAuthRepository) ValidateToken(tokenStr string) (*models.User, error) {
	now := time.Now()
	query := `
		SELECT u.id, u.email, u.name, u.password_hash, u.salt, u.created_at, u.updated_at
		FROM AuthTokens t
		INNER JOIN Users u ON t.user_id = u.id
		WHERE t.token = @p1 AND t.expires_at > @p2;
	`
	var u models.User
	err := r.db.QueryRow(query, tokenStr, now).Scan(
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

func (r *MSSQLAuthRepository) RevokeToken(tokenStr string) {
	_, _ = r.db.Exec("DELETE FROM AuthTokens WHERE token = @p1", tokenStr)
}

func (r *MSSQLAuthRepository) GetUserByID(userID string) (*models.User, error) {
	var u models.User
	query := "SELECT id, email, name, password_hash, salt, created_at, updated_at FROM Users WHERE id = @p1"
	err := r.db.QueryRow(query, userID).Scan(
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

func (r *MSSQLAuthRepository) FindUserByEmail(email string) (*models.User, error) {
	cleanEmail := strings.ToLower(strings.TrimSpace(email))
	var u models.User
	query := "SELECT id, email, name, password_hash, salt, created_at, updated_at FROM Users WHERE email = @p1"
	err := r.db.QueryRow(query, cleanEmail).Scan(
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

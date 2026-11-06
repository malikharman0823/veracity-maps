package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/malikharman0823/veracity-maps/models"
)

type MSSQLProfileRepository struct {
	db *sql.DB
}

var (
	ErrVerificationRateLimited = errors.New("verification requested recently")
	ErrEmailAlreadyVerified    = errors.New("email already verified")
)

func NewMSSQLProfileRepository(db *sql.DB) *MSSQLProfileRepository {
	return &MSSQLProfileRepository{db: db}
}

func (r *MSSQLProfileRepository) Get(ctx context.Context, userID string) (*models.UserPublicProfile, error) {
	var p models.UserPublicProfile
	var avatar sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT id, name, email, profile_picture_path, email_verified,
		account_status, created_at, updated_at FROM Users WHERE id = @id`, sql.Named("id", userID)).Scan(
		&p.ID, &p.FullName, &p.Email, &avatar, &p.EmailVerified, &p.AccountStatus, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	if avatar.Valid && avatar.String != "" {
		p.ProfilePicture = "/api/profile/avatar"
	}
	return &p, nil
}

func (r *MSSQLProfileRepository) UpdateName(ctx context.Context, userID, name string) (*models.UserPublicProfile, error) {
	result, err := r.db.ExecContext(ctx, "UPDATE Users SET name = @name, updated_at = @now WHERE id = @id", sql.Named("name", name), sql.Named("now", time.Now().UTC()), sql.Named("id", userID))
	if err := profileUpdated(result, err); err != nil {
		return nil, err
	}
	return r.Get(ctx, userID)
}

func (r *MSSQLProfileRepository) SetAvatar(ctx context.Context, userID, path string) (*models.UserPublicProfile, error) {
	p, err := r.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, "UPDATE Users SET profile_picture_path = @path, updated_at = @now WHERE id = @id", sql.Named("path", path), sql.Named("now", now), sql.Named("id", userID))
	if err := profileUpdated(result, err); err != nil {
		return nil, err
	}
	p.ProfilePicture = "/api/profile/avatar"
	p.UpdatedAt = now
	return p, nil
}

// AvatarPath is internal metadata; Get exposes an authenticated URL instead.
func (r *MSSQLProfileRepository) AvatarPath(ctx context.Context, userID string) (string, error) {
	var path sql.NullString
	err := r.db.QueryRowContext(ctx, "SELECT profile_picture_path FROM Users WHERE id = @id", sql.Named("id", userID)).Scan(&path)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && (!path.Valid || path.String == "")) {
		return "", ErrUserNotFound
	}
	return path.String, err
}

func profileUpdated(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count == 0 {
		return ErrUserNotFound
	}
	return err
}

func (r *MSSQLProfileRepository) ChangePassword(ctx context.Context, userID, currentPassword, newPassword, currentToken string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var hash, salt string
	err = tx.QueryRowContext(ctx, "SELECT password_hash, salt FROM Users WITH (UPDLOCK, HOLDLOCK) WHERE id = @id", sql.Named("id", userID)).Scan(&hash, &salt)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrUserNotFound
	}
	if err != nil {
		return err
	}
	var valid bool
	if strings.HasPrefix(hash, "$2") {
		valid, err = models.VerifyPassword(currentPassword, hash)
	} else {
		valid = len([]byte(currentPassword)) <= 72 && models.VerifyLegacyPassword(currentPassword, salt, hash)
	}
	if err != nil {
		return err
	}
	if !valid {
		return ErrInvalidPassword
	}
	now := time.Now().UTC()
	var active int
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM AuthTokens WHERE user_id = @user AND token_hash = @token AND revoked_at IS NULL AND expires_at > @now`,
		sql.Named("user", userID), sql.Named("token", models.HashToken(currentToken)), sql.Named("now", now)).Scan(&active)
	if err != nil {
		return err
	}
	if active != 1 {
		return ErrInvalidToken
	}
	newHash, err := models.HashPassword(newPassword)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "UPDATE Users SET password_hash = @hash, salt = '', updated_at = @now WHERE id = @id", sql.Named("hash", newHash), sql.Named("now", now), sql.Named("id", userID))
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "UPDATE AuthTokens SET revoked_at = @now WHERE user_id = @user AND token_hash <> @current AND revoked_at IS NULL", sql.Named("now", now), sql.Named("user", userID), sql.Named("current", models.HashToken(currentToken)))
	if err != nil {
		return err
	}
	return tx.Commit()
}

// CreateEmailVerification rotates pending requests and returns the raw token only
// to the caller building the mail link. SQL stores its SHA-256 hash.
func (r *MSSQLProfileRepository) CreateEmailVerification(ctx context.Context, userID, email, purpose string) (string, error) {
	if purpose != "email_change" && purpose != "initial_verification" {
		return "", ErrInvalidToken
	}
	raw, err := models.GenerateRandomHex(32)
	if err != nil {
		return "", err
	}
	id, err := models.GenerateRandomHex(16)
	if err != nil {
		return "", err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var currentEmail string
	var verified bool
	err = tx.QueryRowContext(ctx, "SELECT email, email_verified FROM Users WITH (UPDLOCK, HOLDLOCK) WHERE id = @id", sql.Named("id", userID)).Scan(&currentEmail, &verified)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrUserNotFound
	}
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	if purpose == "initial_verification" {
		if verified {
			return "", ErrEmailAlreadyVerified
		}
		if email != currentEmail {
			return "", ErrInvalidToken
		}
	}
	var count int
	err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM Users WHERE email = @email AND id <> @id", sql.Named("email", email), sql.Named("id", userID)).Scan(&count)
	if err != nil {
		return "", err
	}
	if count != 0 {
		return "", ErrUserAlreadyExists
	}
	// The user lock serializes requests across both verification purposes, so
	// email-change cannot bypass the initial-verification resend window.
	var recent int
	err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM UserVerificationTokens WHERE user_id = @id AND created_at > @window", sql.Named("id", userID), sql.Named("window", now.Add(-time.Minute))).Scan(&recent)
	if err != nil {
		return "", err
	}
	if recent > 0 {
		return "", ErrVerificationRateLimited
	}
	_, err = tx.ExecContext(ctx, "UPDATE UserVerificationTokens SET consumed_at = @now WHERE user_id = @id AND purpose = @purpose AND consumed_at IS NULL", sql.Named("now", now), sql.Named("id", userID), sql.Named("purpose", purpose))
	if err != nil {
		return "", err
	}
	if purpose == "email_change" {
		_, err = tx.ExecContext(ctx, "UPDATE Users SET pending_email = @email, updated_at = @now WHERE id = @id", sql.Named("email", email), sql.Named("now", now), sql.Named("id", userID))
		if err != nil {
			return "", err
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO UserVerificationTokens (id, user_id, purpose, requested_email, token_hash, expires_at, created_at)
		VALUES (@id, @user, @purpose, @email, @hash, @expires, @now)`, sql.Named("id", "verify_"+id), sql.Named("user", userID), sql.Named("purpose", purpose), sql.Named("email", email), sql.Named("hash", models.HashToken(raw)), sql.Named("expires", now.Add(30*time.Minute)), sql.Named("now", now))
	if err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return raw, nil
}

func (r *MSSQLProfileRepository) ConsumeEmailVerification(ctx context.Context, raw string) error {
	var userID string
	err := r.db.QueryRowContext(ctx, "SELECT user_id FROM UserVerificationTokens WHERE token_hash = @hash", sql.Named("hash", models.HashToken(raw))).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrInvalidToken
	}
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Use the same user-then-token lock order as request rotation.
	var pending sql.NullString
	var currentEmail string
	var verified bool
	err = tx.QueryRowContext(ctx, "SELECT pending_email, email, email_verified FROM Users WITH (UPDLOCK, HOLDLOCK) WHERE id = @id", sql.Named("id", userID)).Scan(&pending, &currentEmail, &verified)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrInvalidToken
	}
	if err != nil {
		return err
	}
	var email, purpose string
	now := time.Now().UTC()
	err = tx.QueryRowContext(ctx, `SELECT requested_email, purpose FROM UserVerificationTokens WITH (UPDLOCK, HOLDLOCK)
		WHERE token_hash = @hash AND user_id = @id AND purpose IN ('email_change', 'initial_verification') AND consumed_at IS NULL AND expires_at > @now`, sql.Named("hash", models.HashToken(raw)), sql.Named("id", userID), sql.Named("now", now)).Scan(&email, &purpose)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrInvalidToken
	}
	if err != nil {
		return err
	}
	if purpose == "initial_verification" {
		if verified || email != currentEmail {
			return ErrInvalidToken
		}
		_, err = tx.ExecContext(ctx, "UPDATE Users SET email_verified = 1, updated_at = @now WHERE id = @id", sql.Named("now", now), sql.Named("id", userID))
	} else {
		if !pending.Valid || pending.String != email {
			return ErrInvalidToken
		}
		_, err = tx.ExecContext(ctx, "UPDATE Users SET email = @email, pending_email = NULL, email_verified = 1, updated_at = @now WHERE id = @id", sql.Named("email", email), sql.Named("now", now), sql.Named("id", userID))
	}
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") || strings.Contains(strings.ToLower(err.Error()), "unique") {
			return ErrUserAlreadyExists
		}
		return err
	}
	_, err = tx.ExecContext(ctx, "UPDATE UserVerificationTokens SET consumed_at = @now WHERE token_hash = @hash", sql.Named("now", now), sql.Named("hash", models.HashToken(raw)))
	if err != nil {
		return err
	}
	return tx.Commit()
}

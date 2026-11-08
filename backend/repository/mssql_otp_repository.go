package repository

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/malikharman0823/veracity-maps/models"
)

// MSSQLOTPRepository handles OTP storage in Microsoft SQL Server.
type MSSQLOTPRepository struct {
	db *sql.DB
}

func NewMSSQLOTPRepository(db *sql.DB) *MSSQLOTPRepository {
	return &MSSQLOTPRepository{db: db}
}

func (r *MSSQLOTPRepository) CreateOTP(
	ctx context.Context,
	email, name, purpose, code string,
	expiresDuration, resendCooldown time.Duration,
) (*OTPRecord, error) {
	cleanEmail := strings.ToLower(strings.TrimSpace(email))
	cleanPurpose := strings.ToLower(strings.TrimSpace(purpose))
	cleanName := strings.TrimSpace(name)
	now := time.Now()

	// 1. Rate-limiting check: check for recent active OTP
	var count int
	rateCheckQuery := `
		SELECT COUNT(*)
		FROM EmailOTPs
		WHERE email = @email AND purpose = @purpose
		  AND resend_available_at > @now
		  AND expires_at > @now
		  AND consumed_at IS NULL;
	`
	err := r.db.QueryRowContext(ctx, rateCheckQuery,
		sql.Named("email", cleanEmail),
		sql.Named("purpose", cleanPurpose),
		sql.Named("now", now),
	).Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("failed checking OTP rate limit: %w", err)
	}
	if count > 0 {
		return nil, ErrOTPRateLimited
	}

	// 2. Invalidate any older unconsumed codes for this email + purpose
	_, _ = r.db.ExecContext(ctx, `
		UPDATE EmailOTPs
		SET consumed_at = @now
		WHERE email = @email AND purpose = @purpose AND consumed_at IS NULL;
	`, sql.Named("now", now), sql.Named("email", cleanEmail), sql.Named("purpose", cleanPurpose))

	// 3. Generate new OTP record
	uid, err := models.GenerateRandomHex(16)
	if err != nil {
		return nil, err
	}
	otpID := "otp_" + uid
	otpHash := models.HashToken(code)
	expiresAt := now.Add(expiresDuration)
	resendAt := now.Add(resendCooldown)

	insertQuery := `
		INSERT INTO EmailOTPs (id, email, name, otp_hash, purpose, attempts, max_attempts, expires_at, resend_available_at, created_at)
		VALUES (@id, @email, @name, @hash, @purpose, 0, 5, @expires, @resend, @created);
	`
	_, err = r.db.ExecContext(ctx, insertQuery,
		sql.Named("id", otpID),
		sql.Named("email", cleanEmail),
		sql.Named("name", cleanName),
		sql.Named("hash", otpHash),
		sql.Named("purpose", cleanPurpose),
		sql.Named("expires", expiresAt),
		sql.Named("resend", resendAt),
		sql.Named("created", now),
	)
	if err != nil {
		return nil, fmt.Errorf("failed inserting OTP record: %w", err)
	}

	return &OTPRecord{
		ID:                otpID,
		Email:             cleanEmail,
		Name:              cleanName,
		OTPHash:           otpHash,
		Purpose:           cleanPurpose,
		Attempts:          0,
		MaxAttempts:       5,
		ExpiresAt:         expiresAt,
		ResendAvailableAt: resendAt,
		CreatedAt:         now,
	}, nil
}

func (r *MSSQLOTPRepository) VerifyAndConsumeOTP(ctx context.Context, email, code, purpose string) (*OTPRecord, error) {
	cleanEmail := strings.ToLower(strings.TrimSpace(email))
	cleanPurpose := strings.ToLower(strings.TrimSpace(purpose))
	now := time.Now()

	selectQuery := `
		SELECT TOP 1 id, email, COALESCE(name, ''), otp_hash, purpose, attempts, max_attempts, expires_at, resend_available_at, created_at
		FROM EmailOTPs
		WHERE email = @email AND purpose = @purpose
		  AND expires_at > @now
		  AND consumed_at IS NULL
		ORDER BY created_at DESC;
	`

	var rec OTPRecord
	err := r.db.QueryRowContext(ctx, selectQuery,
		sql.Named("email", cleanEmail),
		sql.Named("purpose", cleanPurpose),
		sql.Named("now", now),
	).Scan(
		&rec.ID,
		&rec.Email,
		&rec.Name,
		&rec.OTPHash,
		&rec.Purpose,
		&rec.Attempts,
		&rec.MaxAttempts,
		&rec.ExpiresAt,
		&rec.ResendAvailableAt,
		&rec.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrOTPExpiredOrNotFound
		}
		return nil, fmt.Errorf("failed querying OTP record: %w", err)
	}

	rec.Attempts++
	// Increment attempts counter in database
	_, _ = r.db.ExecContext(ctx, "UPDATE EmailOTPs SET attempts = attempts + 1 WHERE id = @id", sql.Named("id", rec.ID))

	if rec.Attempts > rec.MaxAttempts {
		// Invalidate
		_, _ = r.db.ExecContext(ctx, "UPDATE EmailOTPs SET consumed_at = @now WHERE id = @id", sql.Named("now", now), sql.Named("id", rec.ID))
		return nil, ErrOTPMaxAttempts
	}

	codeHash := models.HashToken(code)
	if subtle.ConstantTimeCompare([]byte(rec.OTPHash), []byte(codeHash)) != 1 {
		return nil, ErrOTPInvalid
	}

	// Mark consumed
	_, err = r.db.ExecContext(ctx, "UPDATE EmailOTPs SET consumed_at = @now WHERE id = @id", sql.Named("now", now), sql.Named("id", rec.ID))
	if err != nil {
		return nil, fmt.Errorf("failed consuming OTP: %w", err)
	}

	rec.ConsumedAt = &now
	return &rec, nil
}

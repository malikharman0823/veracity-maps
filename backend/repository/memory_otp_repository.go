package repository

import (
	"context"
	"crypto/subtle"
	"strings"
	"sync"
	"time"

	"github.com/malikharman0823/veracity-maps/models"
)

// MemoryOTPRepository provides an in-memory thread-safe OTP store.
type MemoryOTPRepository struct {
	mu      sync.RWMutex
	records map[string]*OTPRecord // key: email + ":" + purpose
}

func NewMemoryOTPRepository() *MemoryOTPRepository {
	return &MemoryOTPRepository{
		records: make(map[string]*OTPRecord),
	}
}

func otpKey(email, purpose string) string {
	return strings.ToLower(strings.TrimSpace(email)) + ":" + strings.ToLower(strings.TrimSpace(purpose))
}

func (r *MemoryOTPRepository) CreateOTP(
	ctx context.Context,
	email, name, purpose, code string,
	expiresDuration, resendCooldown time.Duration,
) (*OTPRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	key := otpKey(email, purpose)
	now := time.Now()

	// Check rate limit: existing unconsumed OTP with resend_available_at > now
	if existing, exists := r.records[key]; exists {
		if existing.ConsumedAt == nil && existing.ExpiresAt.After(now) && existing.ResendAvailableAt.After(now) {
			return nil, ErrOTPRateLimited
		}
	}

	id, err := models.GenerateRandomHex(16)
	if err != nil {
		return nil, err
	}

	rec := &OTPRecord{
		ID:                "otp_" + id,
		Email:             strings.ToLower(strings.TrimSpace(email)),
		Name:              strings.TrimSpace(name),
		OTPHash:           models.HashToken(code),
		Purpose:           strings.ToLower(strings.TrimSpace(purpose)),
		Attempts:          0,
		MaxAttempts:       5,
		ExpiresAt:         now.Add(expiresDuration),
		ResendAvailableAt: now.Add(resendCooldown),
		CreatedAt:         now,
	}

	r.records[key] = rec
	return rec, nil
}

func (r *MemoryOTPRepository) VerifyAndConsumeOTP(ctx context.Context, email, code, purpose string) (*OTPRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	key := otpKey(email, purpose)
	rec, exists := r.records[key]
	if !exists {
		return nil, ErrOTPExpiredOrNotFound
	}

	now := time.Now()
	if rec.ConsumedAt != nil || now.After(rec.ExpiresAt) {
		return nil, ErrOTPExpiredOrNotFound
	}

	rec.Attempts++
	if rec.Attempts > rec.MaxAttempts {
		rec.ConsumedAt = &now // Invalidate after too many attempts
		return nil, ErrOTPMaxAttempts
	}

	codeHash := models.HashToken(code)
	if subtle.ConstantTimeCompare([]byte(rec.OTPHash), []byte(codeHash)) != 1 {
		return nil, ErrOTPInvalid
	}

	// Mark consumed
	rec.ConsumedAt = &now
	return rec, nil
}

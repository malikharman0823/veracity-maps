package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/malikharman0823/veracity-maps/models"
)

var (
	ErrUserNotFound         = errors.New("user not found")
	ErrUserAlreadyExists    = errors.New("user with this email already exists")
	ErrInvalidPassword      = errors.New("invalid email or password")
	ErrInvalidToken         = errors.New("invalid or expired token")
	ErrOTPExpiredOrNotFound = errors.New("verification code expired or not found")
	ErrOTPRateLimited       = errors.New("please wait before requesting another code")
	ErrOTPMaxAttempts       = errors.New("maximum verification attempts exceeded")
	ErrOTPInvalid           = errors.New("invalid verification code")
)

type AuthRepository struct {
	mu         sync.RWMutex
	db         *sql.DB
	storageDir string
	usersFile  string
	tokensFile string
	users      map[string]*models.User      // Key: user ID
	emailToID  map[string]string            // Key: lowercased email -> user ID
	tokens     map[string]*models.AuthToken // Key: SHA-256 token hash
}

func (r *AuthRepository) SetDB(db *sql.DB) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.db = db
}

func (r *AuthRepository) AuthenticateAndCreateToken(ctx context.Context, email, password string, duration time.Duration) (*models.User, *models.AuthToken, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	u, err := r.authenticateLocked(email, password)
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if err != nil {
		return nil, nil, err
	}
	token, err := r.createTokenLocked(ctx, u.ID, duration)
	if err != nil {
		return nil, nil, err
	}
	return u, token, nil
}

func NewAuthRepository(storageDir string) (*AuthRepository, error) {
	usersDir := filepath.Join(storageDir, "users")
	if err := os.MkdirAll(usersDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create users storage directory: %w", err)
	}

	repo := &AuthRepository{
		storageDir: usersDir,
		usersFile:  filepath.Join(usersDir, "users.json"),
		tokensFile: filepath.Join(usersDir, "tokens.json"),
		users:      make(map[string]*models.User),
		emailToID:  make(map[string]string),
		tokens:     make(map[string]*models.AuthToken),
	}

	if err := repo.load(); err != nil {
		return nil, fmt.Errorf("failed to load auth data: %w", err)
	}

	return repo, nil
}

func (r *AuthRepository) load() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Load users
	if _, err := os.Stat(r.usersFile); err == nil {
		data, err := os.ReadFile(r.usersFile)
		if err == nil && len(data) > 0 {
			var userList []*models.User
			if err := json.Unmarshal(data, &userList); err == nil {
				for _, u := range userList {
					r.users[u.ID] = u
					r.emailToID[strings.ToLower(u.Email)] = u.ID
				}
			}
		}
	}

	// Load tokens
	if _, err := os.Stat(r.tokensFile); err == nil {
		data, err := os.ReadFile(r.tokensFile)
		if err == nil && len(data) > 0 {
			var tokenList []*models.AuthToken
			now := time.Now()
			if err := json.Unmarshal(data, &tokenList); err == nil {
				for _, t := range tokenList {
					if t.TokenHash != "" && t.Token == "" && t.RevokedAt == nil && t.ExpiresAt.After(now) {
						r.tokens[t.TokenHash] = t
					}
				}
			}
		}
	}

	return nil
}

func (r *AuthRepository) saveUsersLocked() error {
	var userList []*models.User
	for _, u := range r.users {
		userList = append(userList, u)
	}

	data, err := json.MarshalIndent(userList, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := r.usersFile + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpFile, r.usersFile)
}

func (r *AuthRepository) saveTokensLocked() error {
	var tokenList []*models.AuthToken
	now := time.Now()
	for _, t := range r.tokens {
		if t.ExpiresAt.After(now) {
			tokenList = append(tokenList, t)
		}
	}

	data, err := json.MarshalIndent(tokenList, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := r.tokensFile + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpFile, r.tokensFile)
}

// Register creates a new user and hashes their password.
func (r *AuthRepository) Register(ctx context.Context, name, email, password string) (*models.User, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	cleanEmail := strings.ToLower(strings.TrimSpace(email))
	if _, exists := r.emailToID[cleanEmail]; exists {
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

	user := &models.User{
		ID:           userID,
		Email:        cleanEmail,
		Name:         strings.TrimSpace(name),
		PasswordHash: hash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	r.users[user.ID] = user
	r.emailToID[cleanEmail] = user.ID

	if err := r.saveUsersLocked(); err != nil {
		delete(r.users, user.ID)
		delete(r.emailToID, cleanEmail)
		return nil, fmt.Errorf("failed to persist user: %w", err)
	}

	if r.db != nil {
		if _, err := r.db.ExecContext(ctx, `
			INSERT INTO Users (id, email, name, password_hash, salt, created_at, updated_at)
			VALUES (@id, @email, @name, @hash, @salt, @created, @updated);
		`, sql.Named("id", user.ID), sql.Named("email", user.Email), sql.Named("name", user.Name), sql.Named("hash", user.PasswordHash), sql.Named("salt", user.Salt), sql.Named("created", user.CreatedAt), sql.Named("updated", user.UpdatedAt)); err != nil {
			log.Printf("[MSSQL ERROR] Users INSERT failed: %v", err)
		}
	}

	return user, nil
}

// RegisterWithoutPassword creates a new user verified via OTP without requiring a password.
func (r *AuthRepository) RegisterWithoutPassword(ctx context.Context, name, email string) (*models.User, error) {
	randomPassword, err := models.GenerateRandomHex(32)
	if err != nil {
		return nil, err
	}
	u, err := r.Register(ctx, name, email, randomPassword)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	if user, ok := r.users[u.ID]; ok {
		user.EmailVerified = true
	}
	_ = r.saveUsersLocked()
	r.mu.Unlock()
	u.EmailVerified = true
	return u, nil
}

// Authenticate verifies email and password, returning the User if valid.
func (r *AuthRepository) Authenticate(ctx context.Context, email, password string) (*models.User, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.authenticateLocked(email, password)
}

func (r *AuthRepository) authenticateLocked(email, password string) (*models.User, error) {
	if len([]byte(password)) > 72 {
		return nil, ErrInvalidPassword
	}

	cleanEmail := strings.ToLower(strings.TrimSpace(email))
	uid, exists := r.emailToID[cleanEmail]
	if !exists {
		return nil, ErrInvalidPassword
	}

	user, ok := r.users[uid]
	if !ok {
		return nil, ErrInvalidPassword
	}

	if strings.HasPrefix(user.PasswordHash, "$2") {
		ok, err := models.VerifyPassword(password, user.PasswordHash)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrInvalidPassword
		}
	} else {
		if !models.VerifyLegacyPassword(password, user.Salt, user.PasswordHash) {
			return nil, ErrInvalidPassword
		}
		hash, err := models.HashPassword(password)
		if err != nil {
			return nil, err
		}
		previous := *user
		user.PasswordHash, user.Salt, user.UpdatedAt = hash, "", time.Now()
		if err := r.saveUsersLocked(); err != nil {
			*user = previous
			return nil, err
		}
	}

	return user, nil
}

// CreateToken issues a new session bearer token for a user.
func (r *AuthRepository) CreateToken(ctx context.Context, userID string, duration time.Duration) (*models.AuthToken, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.createTokenLocked(ctx, userID, duration)
}

func (r *AuthRepository) createTokenLocked(ctx context.Context, userID string, duration time.Duration) (*models.AuthToken, error) {

	tokHex, err := models.GenerateRandomHex(32)
	if err != nil {
		return nil, err
	}

	id, err := models.GenerateRandomHex(16)
	if err != nil {
		return nil, err
	}
	token := &models.AuthToken{
		ID:        "tok_" + id,
		Token:     "vmap_" + tokHex,
		UserID:    userID,
		ExpiresAt: time.Now().Add(duration),
		CreatedAt: time.Now(),
	}

	stored := *token
	stored.TokenHash, stored.Token = models.HashToken(token.Token), ""
	r.tokens[stored.TokenHash] = &stored
	if err := r.saveTokensLocked(); err != nil {
		delete(r.tokens, stored.TokenHash)
		return nil, err
	}

	if r.db != nil {
		_, _ = r.db.ExecContext(ctx, `
			INSERT INTO AuthTokens (id, token_hash, user_id, expires_at, created_at)
			VALUES (@id, @hash, @user, @expires, @created);
		`, sql.Named("id", token.ID), sql.Named("hash", stored.TokenHash), sql.Named("user", token.UserID), sql.Named("expires", token.ExpiresAt), sql.Named("created", token.CreatedAt))
	}

	return token, nil
}

// ValidateToken returns the User associated with a valid token.
func (r *AuthRepository) ValidateToken(ctx context.Context, tokenStr string) (*models.User, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	token, ok := r.tokens[models.HashToken(tokenStr)]
	if !ok || token.RevokedAt != nil || !token.ExpiresAt.After(time.Now()) {
		return nil, ErrInvalidToken
	}

	user, ok := r.users[token.UserID]
	if !ok {
		return nil, ErrUserNotFound
	}

	return user, nil
}

// RevokeToken deletes an auth token (logout).
func (r *AuthRepository) RevokeToken(ctx context.Context, tokenStr string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	hash := models.HashToken(tokenStr)
	token := r.tokens[hash]
	delete(r.tokens, hash)
	if err := r.saveTokensLocked(); err != nil {
		if token != nil {
			r.tokens[hash] = token
		}
		return err
	}

	if r.db != nil {
		_, err := r.db.ExecContext(ctx, "UPDATE AuthTokens SET revoked_at = @now WHERE token_hash = @hash AND revoked_at IS NULL", sql.Named("now", time.Now()), sql.Named("hash", hash))
		if err != nil {
			return err
		}
	}
	return nil
}

// GetUserByID returns user info by ID.
func (r *AuthRepository) GetUserByID(ctx context.Context, userID string) (*models.User, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	user, ok := r.users[userID]
	if !ok {
		return nil, ErrUserNotFound
	}
	return user, nil
}

// FindUserByEmail returns user info by lowercased email address.
func (r *AuthRepository) FindUserByEmail(ctx context.Context, email string) (*models.User, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	cleanEmail := strings.ToLower(strings.TrimSpace(email))
	uid, ok := r.emailToID[cleanEmail]
	if !ok {
		return nil, ErrUserNotFound
	}
	user, ok := r.users[uid]
	if !ok {
		return nil, ErrUserNotFound
	}
	return user, nil
}

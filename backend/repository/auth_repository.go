package repository

import (
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
	ErrUserNotFound      = errors.New("user not found")
	ErrUserAlreadyExists = errors.New("user with this email already exists")
	ErrInvalidPassword   = errors.New("invalid email or password")
	ErrInvalidToken      = errors.New("invalid or expired token")
)

type AuthRepository struct {
	mu         sync.RWMutex
	db         *sql.DB
	storageDir string
	usersFile  string
	tokensFile string
	users      map[string]*models.User      // Key: user ID
	emailToID  map[string]string            // Key: lowercased email -> user ID
	tokens     map[string]*models.AuthToken // Key: token string
}

func (r *AuthRepository) SetDB(db *sql.DB) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.db = db
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
					if t.ExpiresAt.After(now) {
						r.tokens[t.Token] = t
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
func (r *AuthRepository) Register(name, email, password string) (*models.User, error) {
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

	salt, err := models.GenerateRandomHex(16)
	if err != nil {
		return nil, err
	}

	hash := models.HashPassword(password, salt)
	now := time.Now()

	user := &models.User{
		ID:           userID,
		Email:        cleanEmail,
		Name:         strings.TrimSpace(name),
		PasswordHash: hash,
		Salt:         salt,
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
		if _, err := r.db.Exec(`
			INSERT INTO Users (id, email, name, password_hash, salt, created_at, updated_at)
			VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7);
		`, user.ID, user.Email, user.Name, user.PasswordHash, user.Salt, user.CreatedAt, user.UpdatedAt); err != nil {
			log.Printf("[MSSQL ERROR] Users INSERT failed: %v", err)
		}
	}

	return user, nil
}

// Authenticate verifies email and password, returning the User if valid.
func (r *AuthRepository) Authenticate(email, password string) (*models.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cleanEmail := strings.ToLower(strings.TrimSpace(email))
	uid, exists := r.emailToID[cleanEmail]
	if !exists {
		return nil, ErrInvalidPassword
	}

	user, ok := r.users[uid]
	if !ok {
		return nil, ErrInvalidPassword
	}

	if !models.VerifyPassword(password, user.Salt, user.PasswordHash) {
		return nil, ErrInvalidPassword
	}

	return user, nil
}

// CreateToken issues a new session bearer token for a user.
func (r *AuthRepository) CreateToken(userID string, duration time.Duration) (*models.AuthToken, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	tokHex, err := models.GenerateRandomHex(32)
	if err != nil {
		return nil, err
	}

	token := &models.AuthToken{
		Token:     "vmap_" + tokHex,
		UserID:    userID,
		ExpiresAt: time.Now().Add(duration),
		CreatedAt: time.Now(),
	}

	r.tokens[token.Token] = token
	_ = r.saveTokensLocked()

	if r.db != nil {
		_, _ = r.db.Exec(`
			INSERT INTO AuthTokens (token, user_id, expires_at, created_at)
			VALUES (@p1, @p2, @p3, @p4);
		`, token.Token, token.UserID, token.ExpiresAt, token.CreatedAt)
	}

	return token, nil
}

// ValidateToken returns the User associated with a valid token.
func (r *AuthRepository) ValidateToken(tokenStr string) (*models.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	token, ok := r.tokens[tokenStr]
	if !ok || token.ExpiresAt.Before(time.Now()) {
		return nil, ErrInvalidToken
	}

	user, ok := r.users[token.UserID]
	if !ok {
		return nil, ErrUserNotFound
	}

	return user, nil
}

// RevokeToken deletes an auth token (logout).
func (r *AuthRepository) RevokeToken(tokenStr string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.tokens, tokenStr)
	_ = r.saveTokensLocked()

	if r.db != nil {
		_, _ = r.db.Exec("DELETE FROM AuthTokens WHERE token = @p1", tokenStr)
	}
}

// GetUserByID returns user info by ID.
func (r *AuthRepository) GetUserByID(userID string) (*models.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	user, ok := r.users[userID]
	if !ok {
		return nil, ErrUserNotFound
	}
	return user, nil
}

// FindUserByEmail returns user info by lowercased email address.
func (r *AuthRepository) FindUserByEmail(email string) (*models.User, error) {
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

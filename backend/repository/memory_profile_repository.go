package repository

import (
	"context"
	"errors"
	"sync"

	"github.com/malikharman0823/veracity-maps/models"
)

// MemoryProfileRepository is a thread-safe in-memory/storage-backed implementation of ProfileRepo for development.
type MemoryProfileRepository struct {
	mu          sync.RWMutex
	authRepo    AuthRepo
	avatars     map[string]string // userID -> relative path
	verifTokens map[string]string // token -> userID:email:purpose
}

// NewMemoryProfileRepository constructs a development profile repository.
func NewMemoryProfileRepository(authRepo AuthRepo) *MemoryProfileRepository {
	return &MemoryProfileRepository{
		authRepo:    authRepo,
		avatars:     make(map[string]string),
		verifTokens: make(map[string]string),
	}
}

func (r *MemoryProfileRepository) Get(ctx context.Context, userID string) (*models.UserPublicProfile, error) {
	user, err := r.authRepo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	r.mu.RLock()
	avatarPath := r.avatars[userID]
	r.mu.RUnlock()

	return &models.UserPublicProfile{
		ID:             user.ID,
		FullName:       user.Name,
		Email:          user.Email,
		ProfilePicture: avatarPath,
		EmailVerified:  user.EmailVerified,
		AccountStatus:  "active",
		CreatedAt:      user.CreatedAt,
		UpdatedAt:      user.UpdatedAt,
	}, nil
}

func (r *MemoryProfileRepository) UpdateName(ctx context.Context, userID, name string) (*models.UserPublicProfile, error) {
	user, err := r.authRepo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	user.Name = name
	return r.Get(ctx, userID)
}

func (r *MemoryProfileRepository) SetAvatar(ctx context.Context, userID, path string) (*models.UserPublicProfile, error) {
	r.mu.Lock()
	r.avatars[userID] = path
	r.mu.Unlock()
	return r.Get(ctx, userID)
}

func (r *MemoryProfileRepository) AvatarPath(ctx context.Context, userID string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	path, ok := r.avatars[userID]
	if !ok || path == "" {
		return "", errors.New("avatar not found")
	}
	return path, nil
}

func (r *MemoryProfileRepository) ChangePassword(ctx context.Context, userID, currentPassword, newPassword, currentToken string) error {
	return nil
}

func (r *MemoryProfileRepository) CreateEmailVerification(ctx context.Context, userID, email, purpose string) (string, error) {
	token := "dev_verif_" + userID
	r.mu.Lock()
	r.verifTokens[token] = userID + ":" + email + ":" + purpose
	r.mu.Unlock()
	return token, nil
}

func (r *MemoryProfileRepository) ConsumeEmailVerification(ctx context.Context, raw string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.verifTokens[raw]; !ok {
		return errors.New("invalid or expired verification token")
	}
	delete(r.verifTokens, raw)
	return nil
}

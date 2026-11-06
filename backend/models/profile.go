package models

import (
	"encoding/json"
	"time"
)

// UserPublicProfile contains only account information safe for API responses.
type UserPublicProfile struct {
	ID             string    `json:"id"`
	FullName       string    `json:"full_name"`
	Email          string    `json:"email"`
	ProfilePicture string    `json:"profile_picture,omitempty"`
	EmailVerified  bool      `json:"email_verified"`
	AccountStatus  string    `json:"account_status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// MarshalJSON keeps the legacy frontend name key derived from the same value.
func (p UserPublicProfile) MarshalJSON() ([]byte, error) {
	type publicProfile UserPublicProfile
	return json.Marshal(struct {
		publicProfile
		Name string `json:"name"`
	}{publicProfile: publicProfile(p), Name: p.FullName})
}

type UpdateProfileRequest struct {
	FullName string `json:"full_name"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type ChangeEmailRequest struct {
	Email string `json:"email"`
}

type VerifyEmailRequest struct {
	Token string `json:"token"`
}

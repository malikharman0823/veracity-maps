package services

import (
	"context"
	"errors"
)

var ErrEmailDeliveryFailed = errors.New("email delivery failed")

type VerificationEmail struct {
	To   string
	Link string
}

type InvitationEmail struct {
	To          string
	Link        string
	ProjectName string
	InviterName string
	Role        string
}

type OTPEmail struct {
	To      string
	Code    string
	Purpose string // "login" | "register"
}

type Mailer interface {
	SendVerification(context.Context, VerificationEmail) error
	SendInvitation(context.Context, InvitationEmail) error
	SendOTP(context.Context, OTPEmail) error
}

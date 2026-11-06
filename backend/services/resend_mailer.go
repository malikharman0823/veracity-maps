package services

import (
	"context"
	"net/http"
	"time"

	"github.com/resend/resend-go/v4"
)

type ResendMailer struct {
	client *resend.Client
	from   string
}

func NewResendMailer(apiKey, from string) *ResendMailer {
	return &ResendMailer{
		client: resend.NewCustomClient(&http.Client{Timeout: 15 * time.Second}, apiKey),
		from:   from,
	}
}

func (m *ResendMailer) SendVerification(ctx context.Context, message VerificationEmail) error {
	return m.send(ctx, message.To, "Verify your Veracity Maps email", "Verify your email using this link (expires in 30 minutes):\n"+message.Link)
}

func (m *ResendMailer) SendInvitation(ctx context.Context, message InvitationEmail) error {
	return m.send(ctx, message.To, "Veracity Maps project invitation", "You have been invited to a project. Open this link:\n"+message.Link)
}

func (m *ResendMailer) send(ctx context.Context, to, subject, body string) error {
	_, err := m.client.Emails.SendWithContext(ctx, &resend.SendEmailRequest{
		From: m.from, To: []string{to}, Subject: subject, Text: body,
	})
	if err != nil {
		// Provider errors may contain request details; keep links and keys private.
		return ErrEmailDeliveryFailed
	}
	return nil
}

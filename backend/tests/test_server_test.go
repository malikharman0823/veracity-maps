package tests

import (
	"context"
	"sync"

	"github.com/malikharman0823/veracity-maps/services"
)

type fakeMailer struct {
	mu                     sync.Mutex
	verificationRecipients []string
	invitationRecipients   []string
	verifications          []services.VerificationEmail
	err                    error
}

func (m *fakeMailer) SendVerification(_ context.Context, message services.VerificationEmail) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.verificationRecipients = append(m.verificationRecipients, message.To)
	m.verifications = append(m.verifications, message)
	return m.err
}

func (m *fakeMailer) SendInvitation(_ context.Context, message services.InvitationEmail) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.invitationRecipients = append(m.invitationRecipients, message.To)
	return m.err
}

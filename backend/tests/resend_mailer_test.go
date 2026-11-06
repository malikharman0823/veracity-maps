package tests

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/malikharman0823/veracity-maps/services"
)

type mailTransport func(*http.Request) (*http.Response, error)

func (f mailTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestResendMailerSendsVerificationAndInvitation(t *testing.T) {
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	var messages []map[string]any
	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "request-context")
	http.DefaultTransport = mailTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" || r.URL.Host != "api.resend.com" || r.URL.Path != "/emails" {
			t.Fatalf("unexpected provider request: %s %s", r.Method, r.URL)
		}
		if r.Context().Value(contextKey{}) != "request-context" {
			t.Fatal("mail request context was not propagated")
		}
		var message map[string]any
		if err := json.NewDecoder(r.Body).Decode(&message); err != nil {
			t.Fatal(err)
		}
		messages = append(messages, message)
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"test-message"}`))}, nil
	})
	mailer := services.NewResendMailer("test-key", "Veracity Maps <maps@example.com>")
	if err := mailer.SendVerification(ctx, services.VerificationEmail{To: "verify@example.com", Link: "https://maps.example.com/verify-email?token=test-only"}); err != nil {
		t.Fatal(err)
	}
	if err := mailer.SendInvitation(ctx, services.InvitationEmail{To: "invite@example.com", Link: "https://maps.example.com/invitations/test-only"}); err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 {
		t.Fatalf("provider request count: %d", len(messages))
	}
	for i, message := range messages {
		to := []string{"verify@example.com", "invite@example.com"}[i]
		if message["from"] != "Veracity Maps <maps@example.com>" || message["to"].([]any)[0] != to || !strings.Contains(message["text"].(string), "test-only") {
			t.Fatalf("wrong provider message %d", i)
		}
	}
}

func TestResendMailerDoesNotExposeProviderErrors(t *testing.T) {
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	http.DefaultTransport = mailTransport(func(r *http.Request) (*http.Response, error) { return nil, errors.New("private-key-and-link") })
	mailer := services.NewResendMailer("test-key", "maps@example.com")
	err := mailer.SendVerification(context.Background(), services.VerificationEmail{To: "test@example.com", Link: "https://maps.example.com"})
	if err == nil || strings.Contains(err.Error(), "private-key-and-link") {
		t.Fatalf("provider error was not sanitized: %v", err)
	}
}

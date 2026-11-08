package services

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
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
	return m.send(ctx, message.To, "Verify your Veracity Maps email", "Verify your email using this link (expires in 30 minutes):\n"+message.Link, "")
}

func (m *ResendMailer) SendInvitation(ctx context.Context, message InvitationEmail) error {
	inviter := message.InviterName
	if strings.TrimSpace(inviter) == "" {
		inviter = "A team member"
	}
	projName := message.ProjectName
	if strings.TrimSpace(projName) == "" {
		projName = "a project"
	}
	roleDesc := message.Role
	if strings.TrimSpace(roleDesc) == "" {
		roleDesc = "collaborator"
	}

	subject := fmt.Sprintf("%s invited you to collaborate on %s", inviter, projName)
	body := fmt.Sprintf("%s has invited you to collaborate on \"%s\" as an %s.\n\nOpen this link to view and accept your invitation:\n%s\n\nIf you do not have an account yet, simply use your email to sign in.",
		inviter, projName, roleDesc, message.Link)

	html := fmt.Sprintf(`<div style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; max-width: 520px; margin: 0 auto; padding: 32px 24px; background: #ffffff; border: 1px solid #e2e8f0; border-radius: 12px; color: #171717;">
		<div style="margin-bottom: 24px;">
			<span style="font-size: 18px; font-weight: 700; color: #10b981; letter-spacing: -0.5px;">Veracity Maps</span>
		</div>
		<h2 style="font-size: 20px; font-weight: 600; margin: 0 0 12px; color: #171717;">Project Invitation</h2>
		<p style="font-size: 14px; color: #4b5563; margin: 0 0 20px; line-height: 1.5;">
			<strong style="color: #171717;">%s</strong> has invited you to collaborate on <strong style="color: #171717;">%s</strong> with <span style="display: inline-block; background: #ecfdf5; color: #059669; font-weight: 600; padding: 2px 8px; border-radius: 4px; font-size: 12px; text-transform: capitalize;">%s</span> permissions.
		</p>
		<div style="text-align: center; margin: 28px 0;">
			<a href="%s" style="background-color: #10b981; color: #ffffff; padding: 12px 28px; font-size: 14px; font-weight: 600; text-decoration: none; border-radius: 6px; display: inline-block; box-shadow: 0 1px 2px rgba(0,0,0,0.05);">
				View &amp; Accept Invitation
			</a>
		</div>
		<p style="font-size: 13px; color: #64748b; margin: 0 0 8px; line-height: 1.5;">Or copy and paste this link into your browser:</p>
		<p style="font-size: 12px; color: #0284c7; word-break: break-all; margin: 0 0 24px;"><a href="%s" style="color: #0284c7;">%s</a></p>
		<hr style="border: none; border-top: 1px solid #f1f5f9; margin: 24px 0 16px;" />
		<p style="font-size: 12px; color: #94a3b8; margin: 0; line-height: 1.5;">If you were not expecting this invitation, you can safely ignore this email.</p>
	</div>`, inviter, projName, roleDesc, message.Link, message.Link, message.Link)

	return m.send(ctx, message.To, subject, body, html)
}

func (m *ResendMailer) SendOTP(ctx context.Context, message OTPEmail) error {
	purposeTitle := "sign in"
	if message.Purpose == "register" {
		purposeTitle = "registration"
	}
	subject := fmt.Sprintf("%s is your Veracity Maps verification code", message.Code)
	body := fmt.Sprintf("Your Veracity Maps verification code is: %s\n\nThis code will expire in 10 minutes.\nIf you did not request this code, you can safely ignore this email.", message.Code)
	html := fmt.Sprintf(`<div style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; max-width: 480px; margin: 0 auto; padding: 32px 24px; background: #ffffff; border: 1px solid #e2e8f0; border-radius: 12px; color: #171717;">
		<div style="margin-bottom: 24px;">
			<span style="font-size: 18px; font-weight: 700; color: #10b981; letter-spacing: -0.5px;">Veracity Maps</span>
		</div>
		<h2 style="font-size: 20px; font-weight: 600; margin: 0 0 12px; color: #171717;">Verification Code</h2>
		<p style="font-size: 14px; color: #4b5563; margin: 0 0 24px; line-height: 1.5;">Use the following one-time code to complete your %s:</p>
		<div style="background-color: #f8fafc; border: 1px solid #e2e8f0; padding: 20px; text-align: center; border-radius: 8px; margin: 0 0 24px;">
			<span style="font-size: 32px; font-weight: 700; letter-spacing: 8px; color: #10b981; font-family: monospace;">%s</span>
		</div>
		<p style="font-size: 12px; color: #94a3b8; margin: 0; line-height: 1.5;">This code will expire in 10 minutes. If you did not request this email, no action is required.</p>
	</div>`, purposeTitle, message.Code)
	return m.send(ctx, message.To, subject, body, html)
}

func (m *ResendMailer) send(ctx context.Context, to, subject, body, html string) error {
	req := &resend.SendEmailRequest{
		From: m.from, To: []string{to}, Subject: subject, Text: body,
	}
	if html != "" {
		req.Html = html
	}
	_, err := m.client.Emails.SendWithContext(ctx, req)
	if err != nil {
		log.Printf("[RESEND ERROR] Failed sending to %s: %v", to, err)
		// Provider errors may contain request details; keep links and keys private.
		return ErrEmailDeliveryFailed
	}
	return nil
}

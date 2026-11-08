package services

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// SMTPConfig holds connection details for an outbound SMTP server.
type SMTPConfig struct {
	Host          string
	Port          int
	Username      string
	Password      string
	From          string
	UseTLS        bool // true for direct SSL/TLS (port 465)
	SkipTLSVerify bool // for local dev/self-signed certs
}

// SMTPMailer implements Mailer using standard net/smtp.
type SMTPMailer struct {
	cfg SMTPConfig
}

// NewSMTPMailer constructs a new Mailer backed by SMTP.
func NewSMTPMailer(cfg SMTPConfig) *SMTPMailer {
	if cfg.Port <= 0 {
		cfg.Port = 587
	}
	if strings.TrimSpace(cfg.From) == "" {
		if strings.TrimSpace(cfg.Username) != "" && strings.Contains(cfg.Username, "@") {
			cfg.From = cfg.Username
		} else {
			cfg.From = "no-reply@veracitymaps.com"
		}
	}
	return &SMTPMailer{cfg: cfg}
}

func (m *SMTPMailer) SendVerification(ctx context.Context, message VerificationEmail) error {
	return m.send(ctx, message.To, "Verify your Veracity Maps email", "Verify your email using this link (expires in 30 minutes):\n"+message.Link, "")
}

func (m *SMTPMailer) SendInvitation(ctx context.Context, message InvitationEmail) error {
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

func (m *SMTPMailer) SendOTP(ctx context.Context, message OTPEmail) error {
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

func (m *SMTPMailer) send(ctx context.Context, to, subject, body, html string) error {
	addr := net.JoinHostPort(m.cfg.Host, strconv.Itoa(m.cfg.Port))
	tlsConfig := &tls.Config{
		ServerName:         m.cfg.Host,
		InsecureSkipVerify: m.cfg.SkipTLSVerify,
	}

	envelopeFrom := m.cfg.From
	if parsed, err := mail.ParseAddress(m.cfg.From); err == nil && parsed.Address != "" {
		envelopeFrom = parsed.Address
	}

	var client *smtp.Client
	var err error

	// Channel for dial with timeout
	dialer := &net.Dialer{Timeout: 15 * time.Second}

	if m.cfg.UseTLS || m.cfg.Port == 465 {
		// Implicit TLS (Port 465)
		conn, dialErr := tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
		if dialErr != nil {
			log.Printf("[SMTP ERROR] Direct TLS dial to %s failed: %v", addr, dialErr)
			return ErrEmailDeliveryFailed
		}
		defer conn.Close()
		client, err = smtp.NewClient(conn, m.cfg.Host)
	} else {
		// Plain or STARTTLS (Port 587 or 25)
		conn, dialErr := dialer.DialContext(ctx, "tcp", addr)
		if dialErr != nil {
			log.Printf("[SMTP ERROR] TCP dial to %s failed: %v", addr, dialErr)
			return ErrEmailDeliveryFailed
		}
		defer conn.Close()
		client, err = smtp.NewClient(conn, m.cfg.Host)
		if err == nil {
			if ok, _ := client.Extension("STARTTLS"); ok {
				if startErr := client.StartTLS(tlsConfig); startErr != nil {
					log.Printf("[SMTP ERROR] STARTTLS failed: %v", startErr)
					_ = client.Close()
					return ErrEmailDeliveryFailed
				}
			}
		}
	}

	if err != nil {
		log.Printf("[SMTP ERROR] Failed creating SMTP client: %v", err)
		return ErrEmailDeliveryFailed
	}
	defer client.Quit()

	// Authenticate if credentials provided
	if m.cfg.Username != "" && m.cfg.Password != "" {
		auth := smtp.PlainAuth("", m.cfg.Username, m.cfg.Password, m.cfg.Host)
		if authErr := client.Auth(auth); authErr != nil {
			log.Printf("[SMTP ERROR] Authentication failed for user %s: %v", m.cfg.Username, authErr)
			return ErrEmailDeliveryFailed
		}
	}

	// Set envelope sender & recipient
	if err := client.Mail(envelopeFrom); err != nil {
		log.Printf("[SMTP ERROR] MAIL FROM failed for %s: %v", envelopeFrom, err)
		return ErrEmailDeliveryFailed
	}
	if err := client.Rcpt(to); err != nil {
		log.Printf("[SMTP ERROR] RCPT TO failed for %s: %v", to, err)
		return ErrEmailDeliveryFailed
	}

	// Write payload
	w, err := client.Data()
	if err != nil {
		log.Printf("[SMTP ERROR] DATA command failed: %v", err)
		return ErrEmailDeliveryFailed
	}

	msgBytes := buildMIMEMessage(m.cfg.From, to, subject, body, html)
	if _, err := w.Write(msgBytes); err != nil {
		log.Printf("[SMTP ERROR] Writing email payload failed: %v", err)
		_ = w.Close()
		return ErrEmailDeliveryFailed
	}
	if err := w.Close(); err != nil {
		log.Printf("[SMTP ERROR] Closing data writer failed: %v", err)
		return ErrEmailDeliveryFailed
	}

	log.Printf("[SMTP SUCCESS] Dispatched email to %s via %s:%d", to, m.cfg.Host, m.cfg.Port)
	return nil
}

func buildMIMEMessage(from, to, subject, textBody, htmlBody string) []byte {
	boundary := fmt.Sprintf("boundary_%d", time.Now().UnixNano())
	var msg bytes.Buffer

	msg.WriteString(fmt.Sprintf("From: %s\r\n", from))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().Format(time.RFC1123Z)))
	msg.WriteString("MIME-Version: 1.0\r\n")

	if htmlBody != "" {
		msg.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n\r\n", boundary))
		msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		msg.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
		msg.WriteString("Content-Transfer-Encoding: 7bit\r\n\r\n")
		msg.WriteString(textBody + "\r\n\r\n")
		msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		msg.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
		msg.WriteString("Content-Transfer-Encoding: 7bit\r\n\r\n")
		msg.WriteString(htmlBody + "\r\n\r\n")
		msg.WriteString(fmt.Sprintf("--%s--\r\n", boundary))
	} else {
		msg.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
		msg.WriteString("Content-Transfer-Encoding: 7bit\r\n\r\n")
		msg.WriteString(textBody + "\r\n")
	}

	return msg.Bytes()
}

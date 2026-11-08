package tests

import (
	"bufio"
	"context"
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/malikharman0823/veracity-maps/services"
)

// mockSMTPServer runs a lightweight in-memory SMTP server for unit tests.
func mockSMTPServer(t *testing.T, responseCode string) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed starting mock SMTP listener: %v", err)
	}

	done := make(chan struct{})
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				select {
				case <-done:
					return
				default:
					return
				}
			}

			go func(c net.Conn) {
				defer c.Close()
				reader := bufio.NewReader(c)
				// Send greeting
				c.Write([]byte(responseCode + " mock.smtp.local Service ready\r\n"))

				for {
					line, err := reader.ReadString('\n')
					if err != nil {
						return
					}
					line = strings.TrimSpace(line)
					upper := strings.ToUpper(line)

					if strings.HasPrefix(upper, "EHLO") || strings.HasPrefix(upper, "HELO") {
						c.Write([]byte("250-mock.smtp.local\r\n250 HELP\r\n"))
					} else if strings.HasPrefix(upper, "MAIL FROM:") {
						c.Write([]byte("250 2.1.0 Ok\r\n"))
					} else if strings.HasPrefix(upper, "RCPT TO:") {
						c.Write([]byte("250 2.1.5 Ok\r\n"))
					} else if upper == "DATA" {
						c.Write([]byte("354 End data with <CR><LF>.<CR><LF>\r\n"))
						for {
							dataLine, err := reader.ReadString('\n')
							if err != nil {
								return
							}
							if strings.TrimSpace(dataLine) == "." {
								c.Write([]byte("250 2.0.0 Ok: queued\r\n"))
								break
							}
						}
					} else if upper == "QUIT" {
						c.Write([]byte("221 2.0.0 Bye\r\n"))
						return
					} else {
						c.Write([]byte("250 Ok\r\n"))
					}
				}
			}(conn)
		}
	}()

	cleanup := func() {
		close(done)
		ln.Close()
	}

	return ln.Addr().String(), cleanup
}

func TestSMTPMailerSendsOTPVerificationAndInvitation(t *testing.T) {
	addr, cleanup := mockSMTPServer(t, "220")
	defer cleanup()

	host, portStr, _ := net.SplitHostPort(addr)
	port := 25
	if p, err := net.LookupPort("tcp", portStr); err == nil {
		port = p
	}

	mailer := services.NewSMTPMailer(services.SMTPConfig{
		Host:          host,
		Port:          port,
		From:          "Veracity Maps <no-reply@veracitymaps.com>",
		SkipTLSVerify: true,
	})

	ctx := context.Background()

	// 1. Send OTP
	err := mailer.SendOTP(ctx, services.OTPEmail{
		To:      "testuser@example.com",
		Code:    "847291",
		Purpose: "login",
	})
	if err != nil {
		t.Fatalf("SendOTP failed: %v", err)
	}

	// 2. Send Verification
	err = mailer.SendVerification(ctx, services.VerificationEmail{
		To:   "verify@example.com",
		Link: "https://maps.example.com/verify?token=abc",
	})
	if err != nil {
		t.Fatalf("SendVerification failed: %v", err)
	}

	// 3. Send Invitation
	err = mailer.SendInvitation(ctx, services.InvitationEmail{
		To:   "collab@example.com",
		Link: "https://maps.example.com/invite?id=123",
	})
	if err != nil {
		t.Fatalf("SendInvitation failed: %v", err)
	}
}

func TestSMTPMailerErrorHandling(t *testing.T) {
	// Connect to an unreachable port
	mailer := services.NewSMTPMailer(services.SMTPConfig{
		Host: "127.0.0.1",
		Port: 65432,
		From: "no-reply@veracitymaps.com",
	})

	err := mailer.SendOTP(context.Background(), services.OTPEmail{
		To:      "someone@example.com",
		Code:    "123456",
		Purpose: "login",
	})
	if err == nil {
		t.Fatal("expected error on unreachable SMTP host, got nil")
	}
	if !errors.Is(err, services.ErrEmailDeliveryFailed) {
		t.Fatalf("expected ErrEmailDeliveryFailed, got: %v", err)
	}
}

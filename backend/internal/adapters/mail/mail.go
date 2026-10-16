// Package mail sends the app's transactional emails (address verification,
// password reset). Implements ports.Mailer.
package mail

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// SMTPMailer delivers through an SMTP relay over TLS: implicit TLS on port
// 465, otherwise mandatory STARTTLS. There is deliberately no plaintext
// fallback — the messages carry account-recovery links.
type SMTPMailer struct {
	Host     string
	Port     int
	Username string
	Password string
	// From is the sender, e.g. "Lagune <no-reply@example.com>".
	From string

	// dial is replaced in tests.
	dial func(ctx context.Context, addr string) (net.Conn, error)
}

func (m *SMTPMailer) Send(ctx context.Context, to, subject, body string) error {
	from, err := mail.ParseAddress(m.From)
	if err != nil {
		return fmt.Errorf("mail: invalid SMTP_FROM: %w", err)
	}
	recipient, err := mail.ParseAddress(to)
	if err != nil {
		return fmt.Errorf("mail: invalid recipient: %w", err)
	}
	// ParseAddress already refuses CR/LF in addresses; the subject is ours
	// but stays guarded so a future caller can't smuggle in a header.
	if strings.ContainsAny(subject, "\r\n") {
		return errors.New("mail: newline in subject")
	}

	addr := net.JoinHostPort(m.Host, strconv.Itoa(m.Port))
	dial := m.dial
	if dial == nil {
		dial = func(ctx context.Context, addr string) (net.Conn, error) {
			d := net.Dialer{Timeout: 15 * time.Second}
			if m.Port == 465 {
				return (&tls.Dialer{NetDialer: &d, Config: &tls.Config{ServerName: m.Host, MinVersion: tls.VersionTLS12}}).DialContext(ctx, "tcp", addr)
			}
			return d.DialContext(ctx, "tcp", addr)
		}
	}
	conn, err := dial(ctx, addr)
	if err != nil {
		return fmt.Errorf("mail: connect: %w", err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	}
	client, err := smtp.NewClient(conn, m.Host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("mail: handshake: %w", err)
	}
	defer client.Close()

	if m.Port != 465 {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("mail: server does not offer STARTTLS, refusing to send in clear")
		}
		if err := client.StartTLS(&tls.Config{ServerName: m.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("mail: starttls: %w", err)
		}
	}
	if m.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", m.Username, m.Password, m.Host)); err != nil {
			return fmt.Errorf("mail: auth: %w", err)
		}
	}
	if err := client.Mail(from.Address); err != nil {
		return fmt.Errorf("mail: MAIL FROM: %w", err)
	}
	if err := client.Rcpt(recipient.Address); err != nil {
		return fmt.Errorf("mail: RCPT TO: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("mail: DATA: %w", err)
	}
	if _, err := w.Write(buildMessage(from, recipient, subject, body, time.Now())); err != nil {
		return fmt.Errorf("mail: write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("mail: finish: %w", err)
	}
	return client.Quit()
}

func buildMessage(from, to *mail.Address, subject, body string, now time.Time) []byte {
	var b strings.Builder
	b.WriteString("From: " + from.String() + "\r\n")
	b.WriteString("To: " + to.String() + "\r\n")
	b.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", subject) + "\r\n")
	b.WriteString("Date: " + now.Format(time.RFC1123Z) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
	enc := base64.StdEncoding.EncodeToString([]byte(body))
	for len(enc) > 76 {
		b.WriteString(enc[:76] + "\r\n")
		enc = enc[76:]
	}
	b.WriteString(enc + "\r\n")
	return []byte(b.String())
}

// LogMailer writes emails to the server log instead of sending them: the
// development stand-in when no SMTP relay is configured. LogBody must stay
// false outside development, since the body holds a live recovery link.
type LogMailer struct{ LogBody bool }

func (m LogMailer) Send(_ context.Context, to, subject, body string) error {
	if m.LogBody {
		log.Printf("mail (SMTP not configured, not sent): to=%s subject=%q\n%s", to, subject, body)
		return nil
	}
	log.Printf("mail (SMTP not configured, not sent): to=%s subject=%q", to, subject)
	return nil
}

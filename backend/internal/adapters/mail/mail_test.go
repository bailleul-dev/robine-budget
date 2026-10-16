package mail

import (
	"context"
	"encoding/base64"
	"net/mail"
	"strings"
	"testing"
	"time"
)

func TestBuildMessage_EncodesSubjectAndBody(t *testing.T) {
	from := &mail.Address{Name: "Lagune", Address: "no-reply@example.com"}
	to := &mail.Address{Address: "person@example.com"}
	msg := string(buildMessage(from, to, "Réinitialisation de votre mot de passe", "Bonjour é\nlien: https://x/y", time.Unix(0, 0).UTC()))

	if !strings.Contains(msg, "Subject: =?utf-8?q?R=C3=A9initialisation") {
		t.Errorf("subject not Q-encoded:\n%s", msg)
	}
	if !strings.Contains(msg, "To: <person@example.com>") {
		t.Errorf("missing To header:\n%s", msg)
	}
	_, body, _ := strings.Cut(msg, "\r\n\r\n")
	decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(body, "\r\n", ""))
	if err != nil || !strings.Contains(string(decoded), "https://x/y") {
		t.Errorf("body did not round-trip: %q (%v)", decoded, err)
	}
}

func TestSMTPMailer_RejectsHeaderInjection(t *testing.T) {
	m := &SMTPMailer{Host: "localhost", Port: 587, From: "Lagune <no-reply@example.com>"}
	if err := m.Send(context.Background(), "a@example.com", "hi\r\nBcc: evil@example.com", "x"); err == nil {
		t.Error("newline in subject must be refused")
	}
	if err := m.Send(context.Background(), "a@example.com\r\nBcc: evil@example.com", "hi", "x"); err == nil {
		t.Error("newline in recipient must be refused")
	}
}

func TestLogMailer_NeverFails(t *testing.T) {
	if err := (LogMailer{}).Send(context.Background(), "a@example.com", "s", "secret-link"); err != nil {
		t.Fatal(err)
	}
}

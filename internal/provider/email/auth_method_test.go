package email

import (
	"context"
	"github.com/darkrain/message-delivery/internal/provider"
	"net/smtp"
	"os"
	"testing"
	"time"
)

func TestExplicitPlainUsesVerifiedTLSAndTCPHost(t *testing.T) {
	p := NewSMTP("smtp", "proxy.example.test", 2525, "mail.example.test", "user", "password", "from@example.test", "starttls", time.Second, "plain")
	d := p.newDialer()
	name, _, err := d.Auth.Start(&smtp.ServerInfo{Name: "proxy.example.test", TLS: true, Auth: []string{"CRAM-MD5", "PLAIN", "LOGIN"}})
	if err != nil || name != "PLAIN" {
		t.Fatalf("plain selection: %s %v", name, err)
	}
	if d.TLSConfig.ServerName != "mail.example.test" {
		t.Fatal("certificate name changed")
	}
	for _, host := range []string{"localhost", "proxy.example.test"} {
		if _, _, err := d.Auth.Start(&smtp.ServerInfo{Name: host, TLS: false}); err == nil {
			t.Fatal("credentials allowed without TLS")
		}
	}
	if _, _, err := d.Auth.Start(&smtp.ServerInfo{Name: "wrong.example.test", TLS: true}); err == nil {
		t.Fatal("wrong endpoint accepted")
	}
	bad := NewSMTP("smtp", "host", 25, "", "user", "password", "from@example.test", "starttls", time.Second, "unknown")
	if r := bad.Send(context.Background(), provider.Message{}); r.ErrorCode != "smtp_auth_method_invalid" {
		t.Fatal("invalid auth method not rejected")
	}
}

// Opt-in diagnostic: authenticates with the production dialer, sends no DATA.
func TestSMTPPlainAuthLiveWithoutSending(t *testing.T) {
	if os.Getenv("SMTP_AUTH_LIVE") != "1" {
		t.Skip("opt-in authentication-only check")
	}
	p := NewSMTP("smtp", os.Getenv("SMTP_HOST"), intEnvOrDefault("SMTP_PORT", 2525), os.Getenv("SMTP_AUTH_HOST"), os.Getenv("SMTP_USERNAME"), os.Getenv("SMTP_PASSWORD"), os.Getenv("SMTP_FROM"), "starttls", 20*time.Second, "plain")
	sender, err := p.newDialer().Dial()
	if err != nil {
		t.Fatal("SMTP PLAIN authentication failed")
	}
	if err := sender.Close(); err != nil {
		t.Fatal("SMTP session close failed")
	}
}

package email

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/darkrain/message-delivery/internal/provider"
)

func TestSMTPSendLive(t *testing.T) {
	if os.Getenv("SMTP_LIVE_TEST") != "1" {
		t.Skip("set SMTP_LIVE_TEST=1 to run SMTP live test")
	}
	host := envOrDefault("SMTP_HOST", "smtp.yandex.com")
	port := intEnvOrDefault("SMTP_PORT", 465)
	security := envOrDefault("SMTP_SECURITY", "tls")
	authHost := envOrDefault("SMTP_AUTH_HOST", host)
	username := os.Getenv("SMTP_USERNAME")
	password := os.Getenv("SMTP_PASSWORD")
	from := os.Getenv("SMTP_FROM")
	to := os.Getenv("SMTP_TO")
	if username == "" || password == "" || from == "" || to == "" {
		t.Skip("set SMTP_USERNAME, SMTP_PASSWORD, SMTP_FROM and SMTP_TO to run SMTP live test")
	}

	smtpProvider := NewSMTP("smtp", host, port, authHost, username, password, from, security, 20*time.Second)
	result := smtpProvider.Send(context.Background(), provider.Message{
		Subject:   "message-delivery SMTP live test",
		Body:      "SMTP live test code: 112233",
		Recipient: to,
		Variables: map[string]string{"code": "112233", "ttl_sec": "300"},
	})
	if result.Status != provider.StatusSent {
		t.Fatalf("result = %#v", result)
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func intEnvOrDefault(key string, fallback int) int {
	if value := os.Getenv(key); value != "" {
		n, err := strconv.Atoi(value)
		if err == nil {
			return n
		}
	}
	return fallback
}

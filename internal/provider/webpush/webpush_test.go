package webpush

import (
	"context"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"testing"

	webpushgo "github.com/SherClockHolmes/webpush-go"
	"github.com/darkrain/message-delivery/internal/provider"
)

type responseClient struct {
	statusCode int
	request    *http.Request
}

func (client *responseClient) Do(request *http.Request) (*http.Response, error) {
	client.request = request
	return &http.Response{StatusCode: client.statusCode, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
}

func TestProviderClassifiesExpiredSubscription(t *testing.T) {
	privateKey, publicKey, err := webpushgo.GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	client := &responseClient{statusCode: http.StatusGone}
	sender := New("webpush", publicKey, privateKey, "notifications@example.test", 0)
	sender.client = client

	result := sender.Send(context.Background(), validPushMessage(t))
	if result.Status != provider.StatusUndeliverable || result.ErrorCode != "webpush_subscription_expired" {
		t.Fatalf("result = %#v", result)
	}
	if client.request == nil || client.request.URL.String() != "https://fcm.googleapis.com/subscription" {
		t.Fatalf("request = %#v", client.request)
	}
}

func TestProviderRejectsUnknownPushServiceWithoutNetwork(t *testing.T) {
	privateKey, publicKey, err := webpushgo.GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	client := &responseClient{statusCode: http.StatusGone}
	sender := New("webpush", publicKey, privateKey, "notifications@example.test", 0)
	sender.client = client
	message := validPushMessage(t)
	message.Recipient = "https://push.example.test/subscription"
	result := sender.Send(context.Background(), message)
	if result.Status != provider.StatusUndeliverable || result.ErrorCode != "webpush_endpoint_rejected" {
		t.Fatalf("result = %#v", result)
	}
	if client.request != nil {
		t.Fatal("a rejected push service must not receive a request")
	}
}

func TestProviderRejectsIncompleteSubscriptionWithoutNetwork(t *testing.T) {
	sender := New("webpush", "public", "private", "notifications@example.test", 0)
	result := sender.Send(context.Background(), provider.Message{Recipient: "https://push.example.test/subscription"})
	if result.Status != provider.StatusFailed || result.ErrorCode != "webpush_subscription_invalid" {
		t.Fatalf("result = %#v", result)
	}
}

func validPushMessage(t *testing.T) provider.Message {
	t.Helper()
	privateKey, x, y, err := elliptic.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(privateKey) == 0 {
		t.Fatal("subscription private key is empty")
	}
	authKey := make([]byte, 16)
	if _, err := rand.Read(authKey); err != nil {
		t.Fatal(err)
	}
	return provider.Message{
		EventID: "notification-push-1-7", Subject: "New message", Body: "Hello", Recipient: "https://fcm.googleapis.com/subscription",
		Metadata: map[string]string{
			"push_p256dh": base64.RawURLEncoding.EncodeToString(elliptic.Marshal(elliptic.P256(), x, y)),
			"push_auth":   base64.RawURLEncoding.EncodeToString(authKey),
		},
	}
}

// Mozilla's push service counts the base64 of the encrypted record and
// refuses past 4096 bytes; the library's default record made every push to
// Firefox fail with 413 (alert 20.11). A long notice still fits.
func TestProviderRecordFitsMozillaLimit(t *testing.T) {
	privateKey, publicKey, err := webpushgo.GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	client := &responseClient{statusCode: http.StatusCreated}
	sender := New("webpush", publicKey, privateKey, "notifications@example.test", 0)
	sender.client = client
	message := validPushMessage(t)
	message.Metadata["push_title"] = strings.Repeat("Заголовок ", 40)
	message.Metadata["push_body"] = strings.Repeat("Длинный текст уведомления. ", 200)
	if result := sender.Send(context.Background(), message); result.Status != provider.StatusSent {
		t.Fatalf("result = %#v", result)
	}
	body, err := io.ReadAll(client.request.Body)
	if err != nil {
		t.Fatal(err)
	}
	if encoded := base64.RawURLEncoding.EncodedLen(len(body)); encoded > 4096 {
		t.Fatalf("encrypted record is %d bytes, base64 %d > 4096", len(body), encoded)
	}
}

package webpush

import (
	"context"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"

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

// sendCaptured sends a message through a push service that answers 201 and
// returns the encrypted body it was given.
func sendCaptured(t *testing.T, message provider.Message) []byte {
	t.Helper()
	privateKey, publicKey, err := webpushgo.GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	client := &responseClient{statusCode: http.StatusCreated}
	sender := New("webpush", publicKey, privateKey, "notifications@example.test", 0)
	sender.client = client
	if result := sender.Send(context.Background(), message); result.Status != provider.StatusSent {
		t.Fatalf("result = %#v", result)
	}
	body, err := io.ReadAll(client.request.Body)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

// Mozilla's push service counts the base64 of the encrypted record plus about
// 75 bytes and refuses past 4096: a 4096-byte record failed every push to
// Firefox (alert 20.11), a 3070-byte one was "too long by 73 bytes" (alert
// 21.11). The largest notice the provider sends stays inside it.
func TestProviderRecordFitsMozillaLimit(t *testing.T) {
	if base64.RawURLEncoding.EncodedLen(3070)+mozillaPushOverhead != mozillaPushDataLimit+73 {
		t.Fatal("the overhead no longer reproduces the 413 seen on 21.11")
	}
	cases := map[string]provider.Message{}
	long := validPushMessage(t)
	long.Metadata["push_title"] = strings.Repeat("Заголовок ", 40)
	long.Metadata["push_body"] = strings.Repeat("Длинный текст уведомления. ", 200)
	cases["cyrillic"] = long
	// The widest text there is: four-byte runes and characters JSON escapes,
	// at the rune limits and past them, with a long target path and tag.
	widest := validPushMessage(t)
	widest.Metadata["push_title"] = strings.Repeat("😀<", 200)
	widest.Metadata["push_body"] = strings.Repeat("😀&\"\\", 1000)
	widest.Metadata["push_target_path"] = "/chats/" + strings.Repeat("x", 200)
	widest.Metadata["push_tag"] = "notification-push-" + strings.Repeat("9", 64)
	cases["widest"] = widest
	for name, message := range cases {
		body := sendCaptured(t, message)
		if len(body) != webpushRecordSize {
			t.Fatalf("%s: encrypted body is %d bytes, want the record size %d", name, len(body), webpushRecordSize)
		}
		if counted := base64.RawURLEncoding.EncodedLen(len(body)) + mozillaPushOverhead; counted > mozillaPushDataLimit-128 {
			t.Fatalf("%s: Mozilla counts %d bytes, over %d less the margin", name, counted, mozillaPushDataLimit)
		}
	}
}

// A text too wide for the record is cut until it fits, keeping its start, the
// mark of the cut, the target path and the tag; an ordinary one is unchanged.
func TestPushPayloadCutsTextToFitTheRecord(t *testing.T) {
	payload, ok := pushPayload("Новое сообщение", "Привет <b>&", "/chats/7", "notification-push-1-7")
	if !ok || string(payload) != `{"body":"Привет <b>&","tag":"notification-push-1-7","target_path":"/chats/7","title":"Новое сообщение"}` {
		t.Fatalf("payload = %s, %v", payload, ok)
	}
	payload, ok = pushPayload(strings.Repeat("😀", 500), strings.Repeat("😀", 5000), "/chats/7", "tag-7")
	if !ok || len(payload) > webpushPlaintextLimit {
		t.Fatalf("payload is %d bytes > %d (%v)", len(payload), webpushPlaintextLimit, ok)
	}
	var decoded map[string]string
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(decoded["body"], "😀") || !strings.HasSuffix(decoded["body"], "…") || decoded["target_path"] != "/chats/7" || decoded["tag"] != "tag-7" {
		t.Fatalf("decoded = %#v", decoded)
	}
	if utf8.RuneCountInString(decoded["title"]) != webpushTitleLimit {
		t.Fatalf("title has %d runes", utf8.RuneCountInString(decoded["title"]))
	}
	if _, ok := pushPayload("t", "b", "/"+strings.Repeat("x", 4000), "tag"); ok {
		t.Fatal("a target path larger than the record must not be sent")
	}
}

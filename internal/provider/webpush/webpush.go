package webpush

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	webpushgo "github.com/SherClockHolmes/webpush-go"
	"github.com/darkrain/message-delivery/internal/provider"
)

type Provider struct {
	name            string
	vapidPublicKey  string
	vapidPrivateKey string
	subscriber      string
	timeout         time.Duration
	client          webpushgo.HTTPClient
}

func New(name, vapidPublicKey, vapidPrivateKey, subscriber string, timeout time.Duration) *Provider {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Provider{
		name: name, vapidPublicKey: strings.TrimSpace(vapidPublicKey), vapidPrivateKey: strings.TrimSpace(vapidPrivateKey), subscriber: strings.TrimSpace(subscriber), timeout: timeout,
		client: &http.Client{Timeout: timeout},
	}
}

func (p *Provider) Name() string { return p.name }

// vapidSubscriber hands the signing library the shape it expects. The library
// prefixes anything that is not an https URL with "mailto:", so a subscriber
// written the RFC way - "mailto:someone@example.com" - would be signed as
// "mailto:mailto:someone@example.com" and Apple answers 403 BadJwtToken.
func vapidSubscriber(value string) string {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(trimmed), "mailto:") {
		return strings.TrimSpace(trimmed[len("mailto:"):])
	}
	return trimmed
}

// pushServiceHosts are the push services a subscription may point at.
var pushServiceHosts = []string{
	"fcm.googleapis.com",
	"android.googleapis.com",
	"updates.push.services.mozilla.com",
	"push.apple.com",
	"notify.windows.com",
}

func endpointAllowed(raw string) bool {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" {
		return false
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if host == "" {
		return false
	}
	for _, allowed := range pushServiceHosts {
		if host == allowed || strings.HasSuffix(host, "."+allowed) {
			return true
		}
	}
	return false
}

func (p *Provider) Send(ctx context.Context, message provider.Message) provider.Result {
	if p.vapidPublicKey == "" || p.vapidPrivateKey == "" || p.subscriber == "" {
		return provider.Result{Status: provider.StatusUndeliverable, ErrorCode: "webpush_not_configured"}
	}
	p256dh, authKey := message.Metadata["push_p256dh"], message.Metadata["push_auth"]
	if strings.TrimSpace(message.Recipient) == "" || strings.TrimSpace(p256dh) == "" || strings.TrimSpace(authKey) == "" {
		return provider.Result{Status: provider.StatusFailed, ErrorCode: "webpush_subscription_invalid"}
	}
	// The recipient is a URL that arrived from a browser and this worker posts
	// to it. Whoever stores the subscription decides where the request goes, so
	// the address is checked again here, next to the call that makes it.
	if !endpointAllowed(message.Recipient) {
		return provider.Result{Status: provider.StatusUndeliverable, ErrorCode: "webpush_endpoint_rejected"}
	}
	// A notice longer than the record is cut, not refused: a phone shows a
	// few lines of it anyway, and the app holds the whole text.
	payload, ok := pushPayload(
		first(message.Metadata["push_title"], message.Subject),
		first(message.Metadata["push_body"], message.Body),
		message.Metadata["push_target_path"],
		first(message.Metadata["push_tag"], message.EventID),
	)
	if !ok {
		return provider.Result{Status: provider.StatusFailed, ErrorCode: "webpush_payload_invalid"}
	}
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	response, err := webpushgo.SendNotificationWithContext(ctx, payload, &webpushgo.Subscription{
		Endpoint: message.Recipient,
		Keys:     webpushgo.Keys{P256dh: p256dh, Auth: authKey},
	}, &webpushgo.Options{
		HTTPClient: p.client, Subscriber: vapidSubscriber(p.subscriber), VAPIDPublicKey: p.vapidPublicKey, VAPIDPrivateKey: p.vapidPrivateKey, TTL: 60,
		// The library pads every message to the record size, and the encrypted
		// body is exactly that long. See webpushRecordSize for the budget.
		RecordSize: webpushRecordSize,
	})
	if err != nil {
		if ctx.Err() != nil {
			return provider.Result{Status: provider.StatusFailed, ErrorCode: "webpush_timeout"}
		}
		return provider.Result{Status: provider.StatusFailed, ErrorCode: "webpush_send_failed"}
	}
	defer response.Body.Close()
	switch {
	case response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices:
		return provider.Result{Status: provider.StatusSent}
	case response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusGone:
		return provider.Result{Status: provider.StatusUndeliverable, ErrorCode: "webpush_subscription_expired"}
	case response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= http.StatusInternalServerError:
		return provider.Result{Status: provider.StatusFailed, ErrorCode: "webpush_service_unavailable"}
	default:
		// A push service refuses a message for reasons an operator has to act on
		// - a stale VAPID key, an unacceptable subscriber claim, a payload the
		// service will not take. The status and the answer say which one it is,
		// so they travel with the failure instead of dying in this function.
		body, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		log.Printf("webpush rejected: endpoint=%s status=%d body=%s", message.Recipient, response.StatusCode, strings.TrimSpace(string(body)))
		return provider.Result{Status: provider.StatusFailed, ErrorCode: fmt.Sprintf("webpush_rejected_%d", response.StatusCode)}
	}
}

func first(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

// Mozilla's push service refuses a message whose base64 data, together with
// what it stores alongside, passes 4096 bytes. A record of 4096 bytes failed
// every push to Firefox (alert 20.11); one of 3070 - base64 4094 - still came
// back 413 "too long by 73 bytes" (alert 21.11), so the service counts about
// 75 bytes beyond the base64 of the body. The record is sized for that
// overhead plus a margin of 128 bytes:
//
//	base64(recordSize) + 75 + 128 <= 4096  =>  recordSize <= 2919
//
// 2900 leaves base64 3867 bytes, 154 bytes under the observed limit. The
// plaintext a record carries is recordSize minus the 16-byte tag, the 86-byte
// aes128gcm header and the padding delimiter.
const (
	mozillaPushDataLimit    = 4096
	mozillaPushOverhead     = 75
	webpushRecordSize       = 2900
	webpushRecordHeaderSize = 16 + 4 + 1 + 65
	webpushPlaintextLimit   = webpushRecordSize - 16 - webpushRecordHeaderSize - 1
	webpushTitleLimit       = 120
	webpushBodyLimit        = 600
)

// pushPayload writes the notice the service worker reads, cut to fit the
// record. The rune limits keep an ordinary notice short; a text of wide
// characters (emoji are 4 bytes each) can still outgrow the record, so the
// body is cut further, then the title, until the JSON fits. It is false only
// when even an empty text does not fit - a target path or tag of kilobytes.
func pushPayload(title, body, targetPath, tag string) ([]byte, bool) {
	title, body = truncateRunes(title, webpushTitleLimit), truncateRunes(body, webpushBodyLimit)
	for {
		payload, err := encodePushPayload(map[string]string{"title": title, "body": body, "target_path": targetPath, "tag": tag})
		if err != nil {
			return nil, false
		}
		overflow := len(payload) - webpushPlaintextLimit
		if overflow <= 0 {
			return payload, true
		}
		// Every rune is at least one byte; the mark of the cut adds three.
		switch bodyRunes, titleRunes := utf8.RuneCountInString(body), utf8.RuneCountInString(title); {
		case bodyRunes > 0:
			body = truncateRunes(body, bodyRunes-overflow-3)
		case titleRunes > 1:
			title = truncateRunes(title, titleRunes-overflow-3)
		default:
			return nil, false
		}
	}
}

// encodePushPayload keeps <, > and & as they are: JSON.parse in the service
// worker reads them either way, and escaped they take six bytes each.
func encodePushPayload(value map[string]string) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buffer.Bytes(), "\n"), nil
}

// truncateRunes cuts a text to at most limit characters, marking the cut.
func truncateRunes(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	if limit <= 1 {
		return ""
	}
	return string(runes[:limit-1]) + "…"
}

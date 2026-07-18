package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/darkrain/message-delivery/internal/broker"
	"github.com/darkrain/message-delivery/internal/config"
	"github.com/darkrain/message-delivery/internal/delivery"
)

type options struct {
	configPath       string
	recipientType    string
	recipient        string
	template         string
	purpose          string
	source           string
	eventID          string
	code             string
	ttlSec           string
	locale           string
	selectedProvider string
	providerChain    string
	allowFallback    bool
	variablesJSON    string
	metadataJSON     string
	userID           int64
	waitResult       bool
	timeout          time.Duration
	pretty           bool
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	opts, err := parseOptions(args)
	if err != nil {
		return err
	}
	cfg, err := config.Load(opts.configPath)
	if err != nil {
		return err
	}
	event, err := buildRequest(opts)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.timeout)
	defer cancel()

	b, err := broker.Connect(cfg.Broker, cfg.BrokerURL())
	if err != nil {
		return err
	}
	defer b.Close()
	if err := b.Setup(); err != nil {
		return err
	}

	var results <-chan broker.Delivery
	closeResults := func() error { return nil }
	if opts.waitResult {
		results, closeResults, err = b.ConsumeResultsTemporary()
		if err != nil {
			return err
		}
		defer closeResults()
	}

	if err := b.PublishRequest(ctx, event); err != nil {
		return err
	}
	if !opts.waitResult {
		return printJSON(stdout, opts.pretty, event)
	}

	result, err := waitForResult(ctx, results, event.EventID)
	if err != nil {
		return err
	}
	return printJSON(stdout, opts.pretty, result)
}

func parseOptions(args []string) (options, error) {
	opts := options{}
	fs := flag.NewFlagSet("send-test-message", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.configPath, "config", "message-delivery.example.json", "path to config JSON file")
	fs.StringVar(&opts.recipientType, "recipient-type", delivery.RecipientTypePhone, "recipient type: phone or email")
	fs.StringVar(&opts.recipient, "recipient", "", "recipient phone in E.164 or email address")
	fs.StringVar(&opts.template, "template", "auth_verification_code", "template key")
	fs.StringVar(&opts.purpose, "purpose", "manual_test", "delivery purpose")
	fs.StringVar(&opts.source, "source", "manual-client", "event source")
	fs.StringVar(&opts.eventID, "event-id", "", "event id; generated when empty")
	fs.StringVar(&opts.code, "code", "123456", "code variable")
	fs.StringVar(&opts.ttlSec, "ttl-sec", "300", "ttl_sec variable")
	fs.StringVar(&opts.locale, "locale", "en", "template locale")
	fs.StringVar(&opts.selectedProvider, "provider", "", "selected provider")
	fs.StringVar(&opts.providerChain, "provider-chain", "", "comma-separated provider chain")
	fs.BoolVar(&opts.allowFallback, "allow-fallback", true, "allow provider fallback")
	fs.StringVar(&opts.variablesJSON, "variables", "", "additional variables as JSON object")
	fs.StringVar(&opts.metadataJSON, "metadata", "", "additional metadata as JSON object")
	fs.Int64Var(&opts.userID, "user-id", 0, "optional user id")
	fs.BoolVar(&opts.waitResult, "wait-result", true, "wait for matching message.delivery.result")
	fs.DurationVar(&opts.timeout, "timeout", 15*time.Second, "publish/result timeout")
	fs.BoolVar(&opts.pretty, "pretty", true, "pretty-print JSON output")
	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	if opts.recipient == "" {
		return opts, fmt.Errorf("recipient is required")
	}
	if opts.recipientType != delivery.RecipientTypePhone && opts.recipientType != delivery.RecipientTypeEmail {
		return opts, fmt.Errorf("unsupported recipient-type %q", opts.recipientType)
	}
	return opts, nil
}

func buildRequest(opts options) (delivery.RequestEvent, error) {
	variables := map[string]string{
		"code":    opts.code,
		"ttl_sec": opts.ttlSec,
	}
	extraVariables, err := parseStringMap(opts.variablesJSON)
	if err != nil {
		return delivery.RequestEvent{}, fmt.Errorf("parse variables: %w", err)
	}
	for key, value := range extraVariables {
		variables[key] = value
	}

	metadata := map[string]string{"locale": opts.locale}
	extraMetadata, err := parseStringMap(opts.metadataJSON)
	if err != nil {
		return delivery.RequestEvent{}, fmt.Errorf("parse metadata: %w", err)
	}
	for key, value := range extraMetadata {
		metadata[key] = value
	}

	eventID := opts.eventID
	if eventID == "" {
		eventID = "manual-" + randomHex(8)
	}
	event := delivery.RequestEvent{
		Version:       "v1",
		EventID:       eventID,
		Type:          delivery.EventTypeDeliveryRequested,
		Source:        opts.source,
		Template:      opts.template,
		Purpose:       opts.purpose,
		RecipientType: opts.recipientType,
		Recipient:     opts.recipient,
		Variables:     variables,
		UserID:        opts.userID,
		CreatedAt:     time.Now().UTC(),
		Delivery: delivery.DeliveryPolicy{
			SelectedProvider: opts.selectedProvider,
			ProviderChain:    splitCSV(opts.providerChain),
			AllowFallback:    opts.allowFallback,
		},
		Metadata: metadata,
	}
	if err := event.Validate(); err != nil {
		return delivery.RequestEvent{}, err
	}
	return event, nil
}

func waitForResult(ctx context.Context, results <-chan broker.Delivery, requestEventID string) (delivery.ResultEvent, error) {
	for {
		select {
		case <-ctx.Done():
			return delivery.ResultEvent{}, fmt.Errorf("timed out waiting for result for %q", requestEventID)
		case msg, ok := <-results:
			if !ok {
				return delivery.ResultEvent{}, fmt.Errorf("result consumer closed")
			}
			var result delivery.ResultEvent
			if err := json.Unmarshal(msg.Body, &result); err != nil {
				return delivery.ResultEvent{}, fmt.Errorf("decode result: %w", err)
			}
			if result.RequestEventID == requestEventID {
				return result, nil
			}
		}
	}
}

func parseStringMap(value string) (map[string]string, error) {
	if strings.TrimSpace(value) == "" {
		return map[string]string{}, nil
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(value), &raw); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(raw))
	for key, v := range raw {
		switch typed := v.(type) {
		case string:
			out[key] = typed
		case float64, bool:
			out[key] = fmt.Sprint(typed)
		default:
			return nil, fmt.Errorf("field %q must be scalar", key)
		}
	}
	return out, nil
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func randomHex(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func printJSON(w io.Writer, pretty bool, value any) error {
	encoder := json.NewEncoder(w)
	if pretty {
		encoder.SetIndent("", "  ")
	}
	return encoder.Encode(value)
}

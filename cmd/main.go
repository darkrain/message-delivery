package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/darkrain/message-delivery/internal/broker"
	"github.com/darkrain/message-delivery/internal/config"
	"github.com/darkrain/message-delivery/internal/delivery"
	"github.com/darkrain/message-delivery/internal/provider/factory"
	telegrambot "github.com/darkrain/message-delivery/internal/telegrambot"
	templater "github.com/darkrain/message-delivery/internal/template"
	"github.com/darkrain/message-delivery/internal/worker"
)

var (
	Version     = "dev"
	Build       = "unknown"
	ProjectName = "message-delivery"
)

func main() {
	configPath := flag.String("config", "message-delivery.example.json", "path to config JSON file")
	healthcheckURL := flag.String("healthcheck", "", "healthcheck URL")
	flag.Parse()

	if *healthcheckURL != "" {
		os.Exit(runHealthcheck(*healthcheckURL))
	}

	logger := log.Default()
	logger.Printf("%s version=%s build=%s", ProjectName, Version, Build)

	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Fatalf("failed to load config: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	registry := factory.NewRegistryFromConfig(cfg)

	b, deliveries, closeConsumer, err := connectBroker(ctx, cfg, logger)
	if err != nil {
		logger.Fatalf("failed to start broker consumer: %v", err)
	}
	defer b.Close()
	defer closeConsumer()

	orchestrator := delivery.NewOrchestrator(
		cfg,
		registry,
		templater.NewRenderer(cfg.Templates),
		b,
		delivery.NewMemoryIdempotencyStore(),
	)

	handler := healthHandler(Version)
	if cfg.TelegramBot.Enabled {
		welcomeSender, ok := registry.Get(cfg.Providers.Telegram.DefaultProvider)
		if !ok {
			logger.Fatalf("failed to find Telegram Bot provider %q", cfg.Providers.Telegram.DefaultProvider)
		}
		webhook, err := telegrambot.NewWebhookHandler(cfg.TelegramBot, os.Getenv(cfg.TelegramBot.WebhookSecretEnv), b, welcomeSender, logger)
		if err != nil {
			logger.Fatalf("failed to create Telegram Bot webhook: %v", err)
		}
		handler = withTelegramWebhook(handler, cfg.TelegramBot.WebhookPath, webhook)
	}

	srv := &http.Server{
		Addr:              fmt.Sprintf("%s:%s", cfg.Host, cfg.Port),
		Handler:           handler,
		ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	go func() {
		logger.Printf("starting health server on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("http server error: %v", err)
		}
	}()

	go worker.New(orchestrator, deliveries, logger).Run(ctx)

	<-ctx.Done()
	logger.Println("shutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Printf("http shutdown error: %v", err)
	}
}

func withTelegramWebhook(base http.Handler, path string, webhook http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", base)
	mux.Handle(path, webhook)
	return mux
}

func healthHandler(version string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"status":"ok","version":%q}`, version)
	})
	return mux
}

func runHealthcheck(url string) int {
	client := &http.Client{
		Timeout: 3 * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: 3 * time.Second,
			}).DialContext,
		},
	}
	resp, err := client.Get(url)
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

func connectBroker(ctx context.Context, cfg *config.Config, logger *log.Logger) (*broker.Broker, <-chan broker.Delivery, func() error, error) {
	for {
		b, err := broker.Connect(cfg.Broker, cfg.BrokerURL())
		if err == nil {
			if err = b.Setup(); err == nil {
				deliveries, closeConsumer, err := b.Consume()
				if err == nil {
					return b, deliveries, closeConsumer, nil
				}
			}
			_ = b.Close()
		}
		logger.Printf("broker not ready: %v", err)
		select {
		case <-ctx.Done():
			return nil, nil, nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

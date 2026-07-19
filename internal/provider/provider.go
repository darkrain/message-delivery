package provider

import (
	"context"
	"errors"
)

const (
	StatusSent          = "sent"
	StatusFailed        = "failed"
	StatusUndeliverable = "undeliverable"
)

var (
	ErrProviderNotFound = errors.New("provider not found")
	ErrUndeliverable    = errors.New("undeliverable")
)

type Message struct {
	EventID       string
	Template      string
	Subject       string
	Body          string
	ContentType   string
	RecipientType string
	Recipient     string
	Variables     map[string]string
	Metadata      map[string]string
}

type Result struct {
	Status    string
	ErrorCode string
}

type Provider interface {
	Name() string
	Send(ctx context.Context, msg Message) Result
}

type Registry struct {
	providers map[string]Provider
}

func NewRegistry(providers ...Provider) *Registry {
	registry := &Registry{providers: make(map[string]Provider, len(providers))}
	for _, p := range providers {
		registry.Register(p)
	}
	return registry
}

func (r *Registry) Register(p Provider) {
	if p == nil {
		return
	}
	r.providers[p.Name()] = p
}

func (r *Registry) Get(name string) (Provider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

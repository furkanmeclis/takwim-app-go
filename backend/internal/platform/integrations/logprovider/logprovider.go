// Package logprovider is the development provider of a message kind (SMS,
// WhatsApp, e-mail): an account of it makes no network call and its
// operations write a log line instead (TEC-880). It is registered outside
// production only, so the whole job pipeline can be exercised locally
// before a real provider is connected.
package logprovider

import (
	"context"
	"log/slog"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
)

// ProviderCode is the provider code of the log provider.
const ProviderCode = "log"

// Adapter implements integrations.Adapter for one kind.
type Adapter struct {
	kind string
	Log  *slog.Logger
}

// New builds the adapter of kind (integrations.KindSMS …).
func New(kind string, log *slog.Logger) *Adapter {
	if log == nil {
		log = slog.Default()
	}
	return &Adapter{kind: kind, Log: log}
}

// Kind implements integrations.Adapter.
func (a *Adapter) Kind() string { return a.kind }

// Provider implements integrations.Adapter.
func (a *Adapter) Provider() string { return ProviderCode }

// Fields implements integrations.Adapter: only a sender name.
func (a *Adapter) Fields() []integrations.Field {
	return []integrations.Field{
		{Key: "sender", LabelKey: "integrations.fields.sender", Secret: false, Required: true},
	}
}

// Check implements integrations.Adapter (always reachable).
func (a *Adapter) Check(context.Context, integrations.Account) error { return nil }

// Adapters returns the log adapters of every message kind.
func Adapters(log *slog.Logger) []integrations.Adapter {
	return []integrations.Adapter{
		New(integrations.KindSMS, log), New(integrations.KindWhatsApp, log), New(integrations.KindEmail, log),
	}
}

package resourceevents_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/config"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/realtime"
	"github.com/google/uuid"
)

func newRegistry() *resourceevents.Registry {
	reg := resourceevents.NewRegistry()
	reg.Register("platform.users", "platform.users.read")
	reg.Register("platform.roles", "platform.roles.read")
	return reg
}

func principalCtx(id uuid.UUID, superAdmin bool, perms ...string) context.Context {
	return authctx.WithPrincipal(context.Background(), authctx.Principal{
		UserID: id, Permissions: perms, IsSuperAdmin: superAdmin,
	})
}

func TestChannelRuleAuthorizesByReadPermission(t *testing.T) {
	t.Parallel()
	rule := newRegistry().ChannelRule()
	id := uuid.New()

	tests := []struct {
		name        string
		ctx         context.Context
		channel     string
		super       bool
		wantHandled bool
		wantAllowed bool
	}{
		{"other channel family", principalCtx(id, false), "user:" + id.String(), false, false, false},
		{"has read permission", principalCtx(id, false, "platform.users.read"), "resource:platform.users", false, true, true},
		{"missing permission", principalCtx(id, false, "platform.roles.read"), "resource:platform.users", false, true, false},
		{"unregistered resource", principalCtx(id, false, "platform.users.read"), "resource:platform.secrets", false, true, false},
		{"super admin", principalCtx(id, true), "resource:platform.roles", true, true, true},
		{"super admin, unregistered", principalCtx(id, true), "resource:platform.secrets", true, true, false},
		{"no principal", context.Background(), "resource:platform.users", false, true, false},
		{"principal mismatch", principalCtx(uuid.New(), false, "platform.users.read"), "resource:platform.users", false, true, false},
	}
	for _, tc := range tests {
		handled, allowed, err := rule(tc.ctx, id, tc.super, tc.channel)
		if err != nil {
			t.Fatalf("%s: err %v", tc.name, err)
		}
		if handled != tc.wantHandled || allowed != tc.wantAllowed {
			t.Errorf("%s: handled=%v allowed=%v, want %v/%v", tc.name, handled, allowed, tc.wantHandled, tc.wantAllowed)
		}
	}
}

// rulesAuthorizer mirrors how the auth module wires channel rules in front of
// its own checks (here: deny everything else).
type rulesAuthorizer struct{ rules []realtime.ChannelRule }

func (a rulesAuthorizer) CanSubscribeChannel(ctx context.Context, userUUID uuid.UUID, isSuperAdmin bool, channel string) (bool, error) {
	handled, allowed, err := realtime.RunChannelRules(ctx, a.rules, userUUID, isSuperAdmin, channel)
	if handled {
		return allowed, err
	}
	return false, nil
}

func TestSubscriptionTokenEndpointChecksResourcePermission(t *testing.T) {
	t.Parallel()
	issuer, err := realtime.NewTokenIssuer(config.CentrifugoConfig{
		Enabled: true, TokenHMAC: "unit-test-hmac-secret", TokenTTL: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	h := realtime.NewHandler(issuer, rulesAuthorizer{rules: []realtime.ChannelRule{newRegistry().ChannelRule()}})
	id := uuid.New()

	call := func(channel string, perms ...string) int {
		body := strings.NewReader(`{"channel":"` + channel + `"}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/realtime/subscription-token", body)
		req = req.WithContext(principalCtx(id, false, perms...))
		rec := httptest.NewRecorder()
		h.SubscriptionToken(rec, req)
		return rec.Code
	}

	if code := call("resource:platform.users", "platform.users.read"); code != http.StatusOK {
		t.Fatalf("with permission: status %d, want 200", code)
	}
	if code := call("resource:platform.users"); code != http.StatusForbidden {
		t.Fatalf("without permission: status %d, want 403", code)
	}
	if code := call("resource:platform.unknown", "platform.users.read"); code != http.StatusForbidden {
		t.Fatalf("unknown resource: status %d, want 403", code)
	}
}

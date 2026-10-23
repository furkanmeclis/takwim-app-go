package tenant

import (
	"context"
	"errors"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/google/uuid"
)

func TestParseResourceChannel(t *testing.T) {
	org := uuid.New()
	id, resource, ok := ParseResourceChannel(ResourceChannel(org, "tenant.branches"))
	if !ok || id != org || resource != "tenant.branches" {
		t.Fatalf("round trip = %v %q %v", id, resource, ok)
	}
	for _, bad := range []string{
		"resource:tenant.branches",
		"org:not-a-uuid:resource:tenant.branches",
		"org:" + org.String(),
		"org:" + org.String() + ":resource:",
		"org:" + org.String() + ":resource:Bad Name",
	} {
		if _, _, ok := ParseResourceChannel(bad); ok {
			t.Errorf("ParseResourceChannel(%q) accepted", bad)
		}
	}
}

func TestChannelResolver(t *testing.T) {
	org := uuid.New()
	lookups := 0
	resolve := ChannelResolver(func(_ context.Context, id int64) (uuid.UUID, error) {
		lookups++
		if id != 7 {
			return uuid.Nil, errors.New("unknown")
		}
		return org, nil
	})
	want := ResourceChannel(org, "tenant.branches")

	if _, handled, _ := resolve(context.Background(), "platform.users"); handled {
		t.Fatal("platform resource must fall through to the default channel")
	}
	if ch, handled, err := resolve(context.Background(), "tenant.branches"); !handled || ch != "" || err != nil {
		t.Fatalf("tenant resource without organization = %q %v %v, want skip", ch, handled, err)
	}
	scoped := orgctx.WithScope(context.Background(), orgctx.Scope{ID: 7, UUID: org})
	if ch, _, err := resolve(scoped, "tenant.branches"); ch != want || err != nil || lookups != 0 {
		t.Fatalf("request scope = %q %v lookups=%d", ch, err, lookups)
	}
	job := orgctx.WithOrganizationID(context.Background(), 7)
	if ch, _, err := resolve(job, "tenant.branches"); ch != want || err != nil || lookups != 1 {
		t.Fatalf("job scope = %q %v lookups=%d", ch, err, lookups)
	}
}

type liveStubResolver struct {
	scope orgctx.Scope
	err   error
}

func (s liveStubResolver) Resolve(context.Context, string, int64) (orgctx.Scope, error) {
	return s.scope, s.err
}

func TestChannelRule(t *testing.T) {
	org := uuid.New()
	user := uuid.New()
	reg := resourceevents.NewRegistry()
	reg.Register("tenant.branches", "tenant.branches.read")
	ctx := authctx.WithPrincipal(context.Background(), authctx.Principal{UserID: user, UserInternal: 3})
	channel := ResourceChannel(org, "tenant.branches")

	cases := []struct {
		name     string
		resolver orgctx.Resolver
		channel  string
		want     bool
		wantErr  bool
	}{
		{"member with permission", liveStubResolver{scope: orgctx.Scope{UUID: org, Permissions: []string{"tenant.branches.read"}}}, channel, true, false},
		{"member without permission", liveStubResolver{scope: orgctx.Scope{UUID: org}}, channel, false, false},
		{"not a member", liveStubResolver{err: orgctx.ErrNotMember}, channel, false, false},
		{"suspended", liveStubResolver{err: orgctx.ErrSuspended}, channel, false, false},
		{"resolver failure", liveStubResolver{err: errors.New("db down")}, channel, false, true},
		{"tenancy off", nil, channel, false, false},
		{"unregistered resource", liveStubResolver{scope: orgctx.Scope{UUID: org, Permissions: []string{"tenant.branches.read"}}}, ResourceChannel(org, "tenant.other"), false, false},
	}
	for _, tc := range cases {
		handled, allowed, err := ChannelRule(reg, tc.resolver)(ctx, user, false, tc.channel)
		if !handled || allowed != tc.want || (err != nil) != tc.wantErr {
			t.Errorf("%s: handled=%v allowed=%v err=%v", tc.name, handled, allowed, err)
		}
	}
	if handled, _, _ := ChannelRule(reg, nil)(ctx, user, true, "user:"+user.String()); handled {
		t.Error("other channel families must pass through")
	}
}

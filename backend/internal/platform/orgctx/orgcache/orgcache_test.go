package orgcache

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// fakeResolver returns scopes[ref] (or err) and counts calls.
type fakeResolver struct {
	mu     sync.Mutex
	calls  int
	scopes map[string]orgctx.Scope
	err    error
}

func (f *fakeResolver) Resolve(_ context.Context, ref string, userID int64) (orgctx.Scope, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return orgctx.Scope{}, f.err
	}
	s, ok := f.scopes[ref]
	if !ok {
		return orgctx.Scope{}, orgctx.ErrOrganizationNotFound
	}
	s.Permissions = append([]string(nil), s.Permissions...)
	if userID != 1 {
		s.MemberRole = orgctx.RoleMember
	}
	return s, nil
}

func (f *fakeResolver) set(ref string, s orgctx.Scope) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.scopes[ref] = s
}

func (f *fakeResolver) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func acmeScope() orgctx.Scope {
	return orgctx.Scope{
		ID: 5, UUID: uuid.MustParse("6f1c9a52-1f1e-4a51-9a39-4b1f7d0e2c11"), Slug: "acme", Name: "Acme",
		Status: orgctx.StatusActive, AccessStartsAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		MemberRole: orgctx.RoleOwner, Permissions: []string{"tenant.settings.read", "tenant.settings.write"},
	}
}

func setup(t *testing.T) (*CachedResolver, *fakeResolver, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = rdb.Close() })
	inner := &fakeResolver{scopes: map[string]orgctx.Scope{"acme": acmeScope()}}
	return New(inner, rdb, time.Minute, "test"), inner, mr
}

func mustResolve(t *testing.T, c *CachedResolver, ref string, userID int64) orgctx.Scope {
	t.Helper()
	s, err := c.Resolve(context.Background(), ref, userID)
	if err != nil {
		t.Fatalf("Resolve(%q, %d): %v", ref, userID, err)
	}
	return s
}

func TestHitAfterMappingIsLearned(t *testing.T) {
	t.Parallel()
	c, inner, _ := setup(t)
	want := acmeScope()

	for i := range 4 {
		got := mustResolve(t, c, "acme", 1)
		if got.ID != want.ID || got.UUID != want.UUID || got.MemberRole != want.MemberRole ||
			len(got.Permissions) != 2 || !got.AccessStartsAt.Equal(want.AccessStartsAt) {
			t.Fatalf("call %d: scope = %+v", i, got)
		}
	}
	// 1st call learns ref -> id, 2nd stores the entry, the rest are hits.
	if n := inner.count(); n != 2 {
		t.Fatalf("inner calls = %d, want 2", n)
	}

	// Another user in the same org has its own entry.
	if got := mustResolve(t, c, "acme", 2); got.MemberRole != orgctx.RoleMember {
		t.Fatalf("user 2 role = %q", got.MemberRole)
	}
	if n := inner.count(); n != 3 {
		t.Fatalf("inner calls = %d, want 3", n)
	}
}

func TestInvalidateBumpsVersion(t *testing.T) {
	t.Parallel()
	c, inner, _ := setup(t)
	mustResolve(t, c, "acme", 1)
	mustResolve(t, c, "acme", 1)
	mustResolve(t, c, "acme", 1)
	before := inner.count()

	updated := acmeScope()
	updated.MemberRole = orgctx.RoleAdmin
	updated.Permissions = []string{"tenant.settings.read"}
	inner.set("acme", updated)
	if err := c.InvalidateOrganization(context.Background(), 5); err != nil {
		t.Fatal(err)
	}

	if got := mustResolve(t, c, "acme", 1); got.MemberRole != orgctx.RoleAdmin || got.Has("tenant.settings.write") {
		t.Fatalf("stale scope after invalidation: %+v", got)
	}
	if got := mustResolve(t, c, "acme", 1); got.MemberRole != orgctx.RoleAdmin {
		t.Fatalf("new entry not cached correctly: %+v", got)
	}
	if n := inner.count() - before; n != 1 {
		t.Fatalf("inner calls after invalidation = %d, want 1", n)
	}

	// Invalidating another organization leaves this one cached.
	if err := c.InvalidateOrganization(context.Background(), 99); err != nil {
		t.Fatal(err)
	}
	mustResolve(t, c, "acme", 1)
	if n := inner.count() - before; n != 1 {
		t.Fatalf("unrelated invalidation caused a miss (%d calls)", n)
	}
}

func TestEvictedVersionIsReseeded(t *testing.T) {
	t.Parallel()
	c, inner, mr := setup(t)
	for range 3 {
		mustResolve(t, c, "acme", 1)
	}
	before := inner.count()
	mr.Del(c.versionKey(5))
	mustResolve(t, c, "acme", 1)
	if inner.count() == before {
		t.Fatal("entry under the evicted version was read")
	}
}

func TestErrorsAreNotCached(t *testing.T) {
	t.Parallel()
	c, inner, _ := setup(t)
	setErr := func(err error) {
		inner.mu.Lock()
		inner.err = err
		inner.mu.Unlock()
	}

	for _, wantErr := range []error{orgctx.ErrNotMember, orgctx.ErrSuspended, orgctx.ErrAccessExpired} {
		setErr(nil)
		mustResolve(t, c, "acme", 1)
		mustResolve(t, c, "acme", 1)
		setErr(wantErr)
		// The cached success stays valid until the organization is invalidated.
		mustResolve(t, c, "acme", 1)
		if err := c.InvalidateOrganization(context.Background(), 5); err != nil {
			t.Fatal(err)
		}
		before := inner.count()
		for range 2 {
			if _, err := c.Resolve(context.Background(), "acme", 1); !errors.Is(err, wantErr) {
				t.Fatalf("err = %v, want %v", err, wantErr)
			}
		}
		if n := inner.count() - before; n != 2 {
			t.Fatalf("%v: inner calls = %d, want 2 (negative results must not be cached)", wantErr, n)
		}
	}

	setErr(nil)
	if _, err := c.Resolve(context.Background(), "missing", 1); !errors.Is(err, orgctx.ErrOrganizationNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestSlugMovedToAnotherOrganization(t *testing.T) {
	t.Parallel()
	c, inner, _ := setup(t)
	mustResolve(t, c, "acme", 1) // learns acme -> 5

	// Org 5 renamed (its version is bumped), a new org 9 takes the slug.
	other := acmeScope()
	other.ID = 9
	other.UUID = uuid.New()
	inner.set("acme", other)
	if err := c.InvalidateOrganization(context.Background(), 5); err != nil {
		t.Fatal(err)
	}

	for i := range 3 {
		if got := mustResolve(t, c, "acme", 1); got.ID != 9 {
			t.Fatalf("call %d: got org %d, want 9", i, got.ID)
		}
	}
}

func TestCachedEntryHonoursAccessWindow(t *testing.T) {
	t.Parallel()
	c, inner, _ := setup(t)
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }
	ends := now.Add(30 * time.Second)
	s := acmeScope()
	s.AccessEndsAt = &ends
	inner.set("acme", s)

	mustResolve(t, c, "acme", 1)
	mustResolve(t, c, "acme", 1)
	if got := mustResolve(t, c, "acme", 1); got.AccessEndsAt == nil || !got.AccessEndsAt.Equal(ends) {
		t.Fatalf("access_ends_at = %v", got.AccessEndsAt)
	}

	now = ends
	if _, err := c.Resolve(context.Background(), "acme", 1); !errors.Is(err, orgctx.ErrAccessExpired) {
		t.Fatalf("err = %v, want ErrAccessExpired", err)
	}
}

func TestNilRedisPassThrough(t *testing.T) {
	t.Parallel()
	inner := &fakeResolver{scopes: map[string]orgctx.Scope{"acme": acmeScope()}}
	c := New(inner, nil, 0, "")
	for range 3 {
		mustResolve(t, c, "acme", 1)
	}
	if n := inner.count(); n != 3 {
		t.Fatalf("inner calls = %d, want 3", n)
	}
	if err := c.InvalidateOrganization(context.Background(), 5); err != nil {
		t.Fatalf("invalidate with nil redis: %v", err)
	}
	if err := orgctx.Invalidate(context.Background(), c, 5); err != nil {
		t.Fatal(err)
	}
}

func TestRedisDownFailsOpen(t *testing.T) {
	t.Parallel()
	c, inner, mr := setup(t)
	mustResolve(t, c, "acme", 1)
	mustResolve(t, c, "acme", 1)
	mr.Close()

	before := inner.count()
	for range 2 {
		if got := mustResolve(t, c, "acme", 1); got.ID != 5 {
			t.Fatalf("got %+v", got)
		}
	}
	if n := inner.count() - before; n != 2 {
		t.Fatalf("inner calls = %d, want 2", n)
	}
	if err := c.InvalidateOrganization(context.Background(), 5); err == nil {
		t.Fatal("invalidate should report the redis error")
	}
}

func TestDefaultTTL(t *testing.T) {
	t.Parallel()
	c, _, mr := setup(t)
	c.ttl = DefaultTTL
	mustResolve(t, c, "acme", 1)
	mustResolve(t, c, "acme", 1)
	ver, err := c.version(context.Background(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if ttl := mr.TTL(c.entryKey(5, ver, 1, "acme")); ttl != DefaultTTL {
		t.Fatalf("entry ttl = %v, want %v", ttl, DefaultTTL)
	}
	if ttl := mr.TTL(c.versionKey(5)); ttl != 0 {
		t.Fatalf("version key must not expire, ttl = %v", ttl)
	}
	if New(nil, nil, 0, "").ttl != DefaultTTL {
		t.Fatal("non-positive ttl must fall back to DefaultTTL")
	}
}

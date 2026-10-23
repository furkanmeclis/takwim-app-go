// Package orgcache caches orgctx resolutions in Redis.
//
// Keys (prefix app:{env}:orgctx):
//
//	ref:{ref}                      -> organization id (slug or uuid lookup)
//	ver:{orgID}                    -> version counter, no TTL
//	m:{orgID}:{ver}:{userID}:{ref} -> JSON Scope, TTL
//
// InvalidateOrganization increments ver:{orgID}, so every entry written under
// the old version is never read again and simply expires. The version is
// always read before the underlying resolver runs: a resolution that raced an
// invalidation is stored under the old (dead) version. The ref is part of the
// entry key and a fresh resolution whose id differs from the ref mapping only
// fixes the mapping, so a slug that moved to another organization is never
// served from the old organization's entries.
//
// Only successful resolutions are cached. Negative results (unknown
// organization, not a member, suspended, expired) always go to the
// resolver: they are rare in normal use (the UI only links organizations
// the user belongs to), they are cheap indexed lookups, and an unknown
// organization has no id whose version could invalidate the entry, so
// caching them would delay a just-created organization or membership.
//
// Redis errors fail open to the underlying resolver.
package orgcache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// DefaultTTL is used when New gets a non-positive ttl.
const DefaultTTL = 60 * time.Second

// CachedResolver wraps an orgctx.Resolver with a Redis cache. It implements
// orgctx.Resolver and orgctx.Invalidator.
type CachedResolver struct {
	inner  orgctx.Resolver
	rdb    *redis.Client
	ttl    time.Duration
	prefix string
	now    func() time.Time
}

var (
	_ orgctx.Resolver       = (*CachedResolver)(nil)
	_ orgctx.Invalidator    = (*CachedResolver)(nil)
	_ orgctx.BranchSelector = (*CachedResolver)(nil)
)

// New wraps inner. rdb may be nil (pass-through, invalidation is a no-op).
func New(inner orgctx.Resolver, rdb *redis.Client, ttl time.Duration, appEnv string) *CachedResolver {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	env := strings.TrimSpace(appEnv)
	if env == "" {
		env = "dev"
	}
	return &CachedResolver{
		inner: inner,
		rdb:   rdb,
		ttl:   ttl,
		// v2: the scope carries sector, zone, currency and entitlements.
		prefix: "app:" + env + ":orgctx:v2:",
		now:    time.Now,
	}
}

// Resolve implements orgctx.Resolver.
func (c *CachedResolver) Resolve(ctx context.Context, ref string, userID int64) (orgctx.Scope, error) {
	if c.rdb == nil {
		return c.inner.Resolve(ctx, ref, userID)
	}

	orgID, err := c.rdb.Get(ctx, c.refKey(ref)).Int64()
	if errors.Is(err, redis.Nil) {
		// Id unknown, so no version to read before resolving: only learn
		// the mapping. The next request caches the scope.
		scope, err := c.inner.Resolve(ctx, ref, userID)
		if err == nil {
			c.rdb.Set(ctx, c.refKey(ref), scope.ID, c.ttl)
		}
		return scope, err
	}
	if err != nil {
		return c.inner.Resolve(ctx, ref, userID)
	}

	ver, err := c.version(ctx, orgID)
	if err != nil {
		return c.inner.Resolve(ctx, ref, userID)
	}
	key := c.entryKey(orgID, ver, userID, ref)
	if raw, err := c.rdb.Get(ctx, key).Bytes(); err == nil {
		var scope orgctx.Scope
		if json.Unmarshal(raw, &scope) == nil {
			// The access window can close while the entry is live.
			if scope.AccessEndsAt != nil && !c.now().Before(*scope.AccessEndsAt) {
				return orgctx.Scope{}, orgctx.ErrAccessExpired
			}
			return scope, nil
		}
	}

	scope, err := c.inner.Resolve(ctx, ref, userID)
	if err != nil {
		return scope, err
	}
	if scope.ID != orgID {
		// The ref now points at another organization (slug reused).
		c.rdb.Set(ctx, c.refKey(ref), scope.ID, c.ttl)
		return scope, nil
	}
	if raw, err := json.Marshal(scope); err == nil {
		c.rdb.Set(ctx, key, raw, c.ttl)
	}
	return scope, nil
}

// BranchIDByUUID implements orgctx.BranchSelector by delegating to the inner
// resolver (not cached: one indexed lookup, and a deleted branch must stop
// matching at once).
func (c *CachedResolver) BranchIDByUUID(ctx context.Context, organizationID int64, branchUUID uuid.UUID) (int64, error) {
	sel, ok := c.inner.(orgctx.BranchSelector)
	if !ok {
		return 0, orgctx.ErrBranchNotAllowed
	}
	return sel.BranchIDByUUID(ctx, organizationID, branchUUID)
}

// InvalidateOrganization implements orgctx.Invalidator: it bumps the version
// of organizationID so its cached resolutions are never read again.
func (c *CachedResolver) InvalidateOrganization(ctx context.Context, organizationID int64) error {
	if c.rdb == nil {
		return nil
	}
	if err := c.rdb.Incr(ctx, c.versionKey(organizationID)).Err(); err != nil {
		return fmt.Errorf("orgcache: invalidate organization %d: %w", organizationID, err)
	}
	return nil
}

// version returns the current version of orgID. A missing counter (first use,
// or evicted) is seeded with a random value rather than 0, so an evicted and
// recreated counter does not land on a version that still has live entries.
func (c *CachedResolver) version(ctx context.Context, orgID int64) (string, error) {
	key := c.versionKey(orgID)
	v, err := c.rdb.Get(ctx, key).Result()
	if err == nil {
		return v, nil
	}
	if !errors.Is(err, redis.Nil) {
		return "", err
	}
	seed := strconv.FormatInt(rand.Int64N(1<<62), 10)
	set, err := c.rdb.SetNX(ctx, key, seed, 0).Result()
	if err != nil {
		return "", err
	}
	if set {
		return seed, nil
	}
	return c.rdb.Get(ctx, key).Result()
}

func (c *CachedResolver) refKey(ref string) string {
	return c.prefix + "ref:" + ref
}

func (c *CachedResolver) versionKey(orgID int64) string {
	return c.prefix + "ver:" + strconv.FormatInt(orgID, 10)
}

func (c *CachedResolver) entryKey(orgID int64, ver string, userID int64, ref string) string {
	return fmt.Sprintf("%sm:%d:%s:%d:%s", c.prefix, orgID, ver, userID, ref)
}

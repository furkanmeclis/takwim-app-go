package tenant

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/realtime"
	"github.com/google/uuid"
)

// Live lists of organization-owned resources use one channel per
// organization and resource: org:{org_uuid}:resource:{name} (Centrifugo
// namespace "org"). Platform resources keep resource:{name}.
const (
	orgChannelPrefix  = "org:"
	orgResourceMarker = ":resource:"
)

// ResourceChannel returns the live list channel of a tenant resource.
func ResourceChannel(orgUUID uuid.UUID, resource string) string {
	return orgChannelPrefix + orgUUID.String() + orgResourceMarker + resource
}

// ParseResourceChannel splits org:{uuid}:resource:{name}.
func ParseResourceChannel(channel string) (uuid.UUID, string, bool) {
	rest, ok := strings.CutPrefix(channel, orgChannelPrefix)
	if !ok {
		return uuid.Nil, "", false
	}
	rawID, resource, ok := strings.Cut(rest, orgResourceMarker)
	if !ok || !resourceevents.ValidResourceName(resource) {
		return uuid.Nil, "", false
	}
	id, err := uuid.Parse(rawID)
	if err != nil || id == uuid.Nil {
		return uuid.Nil, "", false
	}
	return id, resource, true
}

// OrganizationUUIDLookup maps an organization id to its uuid. Background
// jobs only carry the id (orgctx.WithOrganizationID).
type OrganizationUUIDLookup func(ctx context.Context, organizationID int64) (uuid.UUID, error)

// QueriesLookup reads organization uuids from the database.
func QueriesLookup(q *db.Queries) OrganizationUUIDLookup {
	return func(ctx context.Context, organizationID int64) (uuid.UUID, error) {
		org, err := q.GetOrganizationByID(ctx, organizationID)
		if err != nil {
			return uuid.Nil, err
		}
		return org.Uuid, nil
	}
}

// ChannelResolver scopes tenant.* resource events to the organization on the
// context. A tenant resource published without an organization is dropped
// rather than leaked on the shared resource:{name} channel.
func ChannelResolver(lookup OrganizationUUIDLookup) resourceevents.ChannelResolver {
	return func(ctx context.Context, resource string) (string, bool, error) {
		if !orgctx.IsTenantResource(resource) {
			return "", false, nil
		}
		scope, ok := orgctx.From(ctx)
		if !ok || scope.ID <= 0 {
			return "", true, nil
		}
		orgUUID := scope.UUID
		if orgUUID == uuid.Nil {
			if lookup == nil {
				return "", true, errors.New("tenant: organization uuid lookup missing")
			}
			id, err := lookup(ctx, scope.ID)
			if err != nil {
				return "", true, fmt.Errorf("tenant: organization uuid: %w", err)
			}
			orgUUID = id
		}
		return ResourceChannel(orgUUID, resource), true, nil
	}
}

// ChannelRule authorizes org:{uuid}:resource:{name} subscriptions: the
// resource must be registered in reg, and the caller must be a member of the
// organization (same resolution as RequireOrganization: active, inside the
// access window) whose tenant role grants the registered permission.
// A nil resolver (tenancy off) denies every tenant channel.
func ChannelRule(reg *resourceevents.Registry, resolver orgctx.Resolver) realtime.ChannelRule {
	return func(ctx context.Context, userUUID uuid.UUID, _ bool, channel string) (bool, bool, error) {
		orgUUID, resource, ok := ParseResourceChannel(channel)
		if !ok {
			return false, false, nil
		}
		perm, registered := reg.Permission(resource)
		if !registered || resolver == nil {
			return true, false, nil
		}
		p, ok := authctx.PrincipalFrom(ctx)
		if !ok || p.UserID != userUUID || p.UserInternal <= 0 {
			return true, false, nil
		}
		scope, err := resolver.Resolve(ctx, orgUUID.String(), p.UserInternal)
		if err != nil {
			if isMembershipError(err) {
				return true, false, nil
			}
			return true, false, err
		}
		// The reference was a uuid: make sure it named this organization.
		if scope.UUID != orgUUID {
			return true, false, nil
		}
		return true, scope.Has(perm), nil
	}
}

func isMembershipError(err error) bool {
	return errors.Is(err, orgctx.ErrOrganizationNotFound) ||
		errors.Is(err, orgctx.ErrNotMember) ||
		errors.Is(err, orgctx.ErrSuspended) ||
		errors.Is(err, orgctx.ErrAccessExpired)
}

package resourceevents

import (
	"context"
	"sync"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/realtime"
	"github.com/google/uuid"
)

// Registry maps a live resource to the permission needed to watch it
// (normally the resource's read permission). Modules register their
// resources at startup; unknown resources cannot be subscribed to.
type Registry struct {
	mu    sync.RWMutex
	perms map[string]string
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{perms: map[string]string{}}
}

// Register declares a live resource and its read permission.
func (r *Registry) Register(resource, permission string) {
	if r == nil || !ValidResourceName(resource) || permission == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.perms[resource] = permission
}

// Permission returns the permission registered for resource.
func (r *Registry) Permission(resource string) (string, bool) {
	if r == nil {
		return "", false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.perms[resource]
	return p, ok
}

// ChannelRule authorizes "resource:{name}" subscriptions: the resource must
// be registered and the request principal must hold its permission (super
// admins always pass). Other channels are left to the next rule.
func (r *Registry) ChannelRule() realtime.ChannelRule {
	return func(ctx context.Context, userUUID uuid.UUID, isSuperAdmin bool, channel string) (bool, bool, error) {
		resource, ok := ParseChannel(channel)
		if !ok {
			return false, false, nil
		}
		return true, r.Allowed(ctx, userUUID, isSuperAdmin, resource), nil
	}
}

// Allowed reports whether the request principal may watch resource.
func (r *Registry) Allowed(ctx context.Context, userUUID uuid.UUID, isSuperAdmin bool, resource string) bool {
	perm, ok := r.Permission(resource)
	if !ok {
		return false
	}
	if isSuperAdmin {
		return true
	}
	p, ok := authctx.PrincipalFrom(ctx)
	if !ok || p.UserID != userUUID {
		return false
	}
	return p.HasPermission(perm)
}

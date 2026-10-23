package usecase

import (
	"context"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/realtime"
)

// Live list resources owned by this module (same slugs as the io / bulk
// engines and resource meta).
const (
	ResourceUsers = "platform.users"
	ResourceRoles = "platform.roles"
)

type (
	resourceEventsPublisher = resourceevents.Publisher
	channelRule             = realtime.ChannelRule
)

// SetResourceEvents attaches the resource-change publisher (optional).
func (u *AuthUseCase) SetResourceEvents(pub resourceevents.Publisher) {
	u.events = pub
}

// AddChannelRules lets other packages authorize their own channel families
// in CanSubscribeChannel (for example resourceevents "resource:{name}").
// Rules run before the built-in checks.
func (u *AuthUseCase) AddChannelRules(rules ...realtime.ChannelRule) {
	u.channelRules = append(u.channelRules, rules...)
}

// publishResource emits a resource-change event; fail-soft and nil-safe.
func (u *AuthUseCase) publishResource(ctx context.Context, resource, action string, uuids ...string) {
	if u.events == nil {
		return
	}
	u.events.Publish(ctx, resource, action, uuids...)
}

// userCreated / userUpdated cover user rows changed outside the platform
// CRUD (self registration, social sign-up, account deactivation).
func (u *AuthUseCase) userCreated(ctx context.Context, userUUID string) {
	u.publishResource(ctx, ResourceUsers, resourceevents.ActionCreated, userUUID)
}

func (u *AuthUseCase) userUpdated(ctx context.Context, userUUID string) {
	u.publishResource(ctx, ResourceUsers, resourceevents.ActionUpdated, userUUID)
}

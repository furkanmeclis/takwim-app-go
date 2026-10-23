package realtime

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

const ChannelSystemNotifications = "system.notifications"

// ChannelAuthorizer decides whether a principal may subscribe to a channel.
type ChannelAuthorizer interface {
	CanSubscribeChannel(ctx context.Context, userUUID uuid.UUID, isSuperAdmin bool, channel string) (bool, error)
}

// ParseUserChannel returns the user UUID from "user:{uuid}".
func ParseUserChannel(channel string) (uuid.UUID, bool) {
	raw, ok := stripPrefix(channel, "user:")
	if !ok {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

func stripPrefix(channel, prefix string) (string, bool) {
	channel = strings.TrimSpace(channel)
	if !strings.HasPrefix(channel, prefix) {
		return "", false
	}
	raw := strings.TrimPrefix(channel, prefix)
	if raw == "" {
		return "", false
	}
	return raw, true
}

// QRLoginChannelPrefix is the Centrifugo namespace for QR sign-in sessions.
// One channel per QR session / browser tab; only the backend publishes.
const QRLoginChannelPrefix = "qrlogin:"

// QRLoginChannel returns the channel for an opaque QR channel id.
func QRLoginChannel(channelID string) string {
	return QRLoginChannelPrefix + channelID
}

// ChannelRule authorizes one family of channels. It returns handled=false
// for channels it does not recognize so the next rule (or the base
// authorizer) decides.
type ChannelRule func(ctx context.Context, userUUID uuid.UUID, isSuperAdmin bool, channel string) (handled, allowed bool, err error)

// RunChannelRules applies rules in order. handled=false means no rule
// recognized the channel and the caller's own checks decide.
func RunChannelRules(ctx context.Context, rules []ChannelRule, userUUID uuid.UUID, isSuperAdmin bool, channel string) (handled, allowed bool, err error) {
	for _, rule := range rules {
		if rule == nil {
			continue
		}
		handled, allowed, err := rule(ctx, userUUID, isSuperAdmin, channel)
		if err != nil {
			return true, false, err
		}
		if handled {
			return true, allowed, nil
		}
	}
	return false, false, nil
}

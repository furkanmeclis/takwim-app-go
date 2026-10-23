package qrlogin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// Session states. "scanned" is internal (a signed-in phone opened the
// session); clients see it as pending.
const (
	StatusPending  = "pending"
	StatusScanned  = "scanned"
	StatusApproved = "approved"
	StatusRejected = "rejected"
)

var (
	// ErrNotFound covers unknown, expired and already exchanged sessions.
	ErrNotFound = errors.New("qr login session not found")
	// ErrResolved means the session was already approved or rejected.
	ErrResolved = errors.New("qr login session already resolved")
	// ErrClaimed means another account scanned the session first.
	ErrClaimed = errors.New("qr login session claimed by another account")
	// ErrInvalidSecret covers a wrong browser secret or exchange token.
	ErrInvalidSecret = errors.New("qr login secret is invalid")
)

// Session is the Redis-held state of one QR sign-in attempt (one browser tab).
type Session struct {
	ChannelID     string
	Status        string
	SecretHash    string
	UserAgent     string
	IP            string
	Location      Location
	CreatedAt     time.Time
	ExpiresAt     time.Time
	ClaimedBy     string
	ApprovedOrg   string
	ExchangeToken string
}

// Store keeps QR sessions in Redis (ephemeral, TTL-bound; never in Postgres).
// Every state transition is a single Lua script, so concurrent approve /
// reject / exchange calls cannot both win.
type Store struct {
	rdb *redis.Client
	env string
}

// NewStore creates a Redis-backed store.
func NewStore(rdb *redis.Client, appEnv string) *Store {
	if appEnv == "" {
		appEnv = "dev"
	}
	return &Store{rdb: rdb, env: appEnv}
}

// key never contains the raw session id (it is a bearer-ish value in the QR).
func (s *Store) key(sessionID string) string {
	return fmt.Sprintf("app:%s:qrlogin:%s", s.env, hashSecret(sessionID))
}

// tokenHash hashes an exchange token; empty stays empty (rejections).
func tokenHash(v string) string {
	if v == "" {
		return ""
	}
	return hashSecret(v)
}

func hashSecret(v string) string {
	sum := sha256.Sum256([]byte(v))
	return hex.EncodeToString(sum[:])
}

// Create stores a new pending session that expires with ttl.
func (s *Store) Create(ctx context.Context, sessionID string, sess Session, ttl time.Duration) error {
	key := s.key(sessionID)
	pipe := s.rdb.TxPipeline()
	pipe.HSet(ctx, key, map[string]any{
		"status":       StatusPending,
		"channel_id":   sess.ChannelID,
		"secret_hash":  sess.SecretHash,
		"ua":           sess.UserAgent,
		"ip":           sess.IP,
		"country_code": sess.Location.CountryCode,
		"country":      sess.Location.Country,
		"region":       sess.Location.Region,
		"city":         sess.Location.City,
		"geo_source":   sess.Location.Source,
		"created_at":   sess.CreatedAt.UTC().Unix(),
		"expires_at":   sess.ExpiresAt.UTC().Unix(),
		"attempts":     0,
	})
	pipe.Expire(ctx, key, ttl)
	_, err := pipe.Exec(ctx)
	return err
}

// Get loads a session (ErrNotFound when missing / expired).
func (s *Store) Get(ctx context.Context, sessionID string) (Session, error) {
	vals, err := s.rdb.HGetAll(ctx, s.key(sessionID)).Result()
	if err != nil {
		return Session{}, err
	}
	if len(vals) == 0 || vals["status"] == "" {
		return Session{}, ErrNotFound
	}
	return sessionFromHash(vals), nil
}

func sessionFromHash(v map[string]string) Session {
	unix := func(k string) time.Time {
		n, _ := strconv.ParseInt(v[k], 10, 64)
		if n == 0 {
			return time.Time{}
		}
		return time.Unix(n, 0).UTC()
	}
	return Session{
		ChannelID:  v["channel_id"],
		Status:     v["status"],
		SecretHash: v["secret_hash"],
		UserAgent:  v["ua"],
		IP:         v["ip"],
		Location: Location{
			CountryCode: v["country_code"], Country: v["country"], Region: v["region"],
			City: v["city"], Source: v["geo_source"],
		},
		CreatedAt:     unix("created_at"),
		ExpiresAt:     unix("expires_at"),
		ClaimedBy:     v["claimed_by"],
		ApprovedOrg:   v["approved_org"],
		ExchangeToken: v["exchange_token"],
	}
}

// claimScript: first signed-in viewer claims a pending session.
// Returns "missing" | "claimed" | "first" | <current status>.
var claimScript = redis.NewScript(`
local st = redis.call('HGET', KEYS[1], 'status')
if not st then return 'missing' end
local by = redis.call('HGET', KEYS[1], 'claimed_by')
if by and by ~= '' and by ~= ARGV[1] then return 'claimed' end
if st == 'pending' then
  redis.call('HSET', KEYS[1], 'status', 'scanned', 'claimed_by', ARGV[1])
  return 'first'
end
return st
`)

// Claim binds the session to the scanning account. first is true for the
// first scan (the browser is told "scanned" once).
func (s *Store) Claim(ctx context.Context, sessionID, userUUID string) (first bool, err error) {
	res, err := claimScript.Run(ctx, s.rdb, []string{s.key(sessionID)}, userUUID).Text()
	if err != nil {
		return false, err
	}
	switch res {
	case "missing":
		return false, ErrNotFound
	case "claimed":
		return false, ErrClaimed
	case "first":
		return true, nil
	default:
		return false, nil
	}
}

// resolveScript moves pending/scanned → approved|rejected exactly once.
// ARGV: user, new status, exchange token, ttl seconds, token hash.
var resolveScript = redis.NewScript(`
local st = redis.call('HGET', KEYS[1], 'status')
if not st then return 'missing' end
local by = redis.call('HGET', KEYS[1], 'claimed_by')
if by and by ~= '' and by ~= ARGV[1] then return 'claimed' end
if st ~= 'pending' and st ~= 'scanned' then return 'resolved' end
redis.call('HSET', KEYS[1], 'status', ARGV[2], 'claimed_by', ARGV[1],
  'exchange_token', ARGV[3], 'exchange_hash', ARGV[5])
redis.call('EXPIRE', KEYS[1], tonumber(ARGV[4]))
return 'ok'
`)

// Resolve approves or rejects the session for userUUID. exchangeToken is only
// meaningful for approvals. ttl bounds the exchange window.
func (s *Store) Resolve(ctx context.Context, sessionID, userUUID, status, exchangeToken string, ttl time.Duration) error {
	secs := int64(ttl.Seconds())
	if secs < 1 {
		secs = 1
	}
	res, err := resolveScript.Run(ctx, s.rdb, []string{s.key(sessionID)},
		userUUID, status, exchangeToken, secs, tokenHash(exchangeToken)).Text()
	if err != nil {
		return err
	}
	switch res {
	case "ok":
		return nil
	case "missing":
		return ErrNotFound
	case "claimed":
		return ErrClaimed
	default:
		return ErrResolved
	}
}

// stateScript returns the status (and pending exchange token) to the tab that
// owns the browser secret. ARGV: secret hash, max attempts.
var stateScript = redis.NewScript(`
local st = redis.call('HGET', KEYS[1], 'status')
if not st then return {'missing'} end
if redis.call('HGET', KEYS[1], 'secret_hash') ~= ARGV[1] then
  local n = redis.call('HINCRBY', KEYS[1], 'attempts', 1)
  if n >= tonumber(ARGV[2]) then redis.call('DEL', KEYS[1]) end
  return {'invalid'}
end
return {st, redis.call('HGET', KEYS[1], 'exchange_token') or ''}
`)

// State reports the session status to the browser tab.
func (s *Store) State(ctx context.Context, sessionID, secretHash string, maxAttempts int) (status, exchangeToken string, err error) {
	res, err := stateScript.Run(ctx, s.rdb, []string{s.key(sessionID)}, secretHash, maxAttempts).StringSlice()
	if err != nil {
		return "", "", err
	}
	switch res[0] {
	case "missing":
		return "", "", ErrNotFound
	case "invalid":
		return "", "", ErrInvalidSecret
	}
	if len(res) > 1 {
		exchangeToken = res[1]
	}
	return res[0], exchangeToken, nil
}

// exchangeScript consumes an approved session (single use: the key is
// deleted). Wrong secrets burn attempts; the session dies after maxAttempts.
// ARGV: secret hash, exchange token hash, max attempts.
var exchangeScript = redis.NewScript(`
local st = redis.call('HGET', KEYS[1], 'status')
if not st then return {'missing'} end
local sh = redis.call('HGET', KEYS[1], 'secret_hash')
local eh = redis.call('HGET', KEYS[1], 'exchange_hash') or ''
if sh ~= ARGV[1] or eh == '' or eh ~= ARGV[2] then
  local n = redis.call('HINCRBY', KEYS[1], 'attempts', 1)
  if n >= tonumber(ARGV[3]) then redis.call('DEL', KEYS[1]) end
  return {'invalid'}
end
if st ~= 'approved' then return {'state', st} end
local out = {'ok',
  redis.call('HGET', KEYS[1], 'claimed_by') or '',
  redis.call('HGET', KEYS[1], 'ua') or '',
  redis.call('HGET', KEYS[1], 'ip') or ''}
redis.call('DEL', KEYS[1])
return out
`)

// Consumed is the result of a successful exchange.
type Consumed struct {
	UserUUID  string
	UserAgent string
	IP        string
}

// Exchange atomically validates and deletes an approved session.
func (s *Store) Exchange(ctx context.Context, sessionID, secretHash, exchangeToken string, maxAttempts int) (Consumed, error) {
	res, err := exchangeScript.Run(ctx, s.rdb, []string{s.key(sessionID)},
		secretHash, tokenHash(exchangeToken), maxAttempts).StringSlice()
	if err != nil {
		return Consumed{}, err
	}
	switch res[0] {
	case "missing":
		return Consumed{}, ErrNotFound
	case "invalid":
		return Consumed{}, ErrInvalidSecret
	case "state":
		return Consumed{}, ErrResolved
	}
	if len(res) < 4 || res[1] == "" {
		return Consumed{}, ErrNotFound
	}
	return Consumed{UserUUID: res[1], UserAgent: res[2], IP: res[3]}, nil
}

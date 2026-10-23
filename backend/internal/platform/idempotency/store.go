// Package idempotency keeps the outcome of unsafe requests sent with an
// Idempotency-Key header (TEC-879), so a client that retries after a timeout
// or a dropped connection (mobile networks) gets the first response back
// instead of a second side effect.
//
// A key is scoped to the caller (access token subject), the organization
// header and the method + path. The first request reserves the key; a
// concurrent retry gets 409 while it runs; a finished request is replayed
// for TTL. A 5xx outcome releases the key so the retry runs again. The same
// key with a different body is a client bug and gets 422.
package idempotency

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// TTL is how long a finished outcome is replayed.
const TTL = 24 * time.Hour

// pendingTTL bounds a reservation whose request never finished (crash).
const pendingTTL = 5 * time.Minute

// Record is a stored outcome. Pending is true while the first request runs.
type Record struct {
	Fingerprint string            `json:"fp"`
	Pending     bool              `json:"pending,omitempty"`
	Status      int               `json:"status,omitempty"`
	Header      map[string]string `json:"header,omitempty"`
	Body        []byte            `json:"body,omitempty"`
}

// Store reserves keys and keeps outcomes.
type Store interface {
	// Reserve stores a pending record when the key is free and returns
	// (nil, nil); otherwise it returns the existing record.
	Reserve(ctx context.Context, key, fingerprint string) (*Record, error)
	// Complete stores the final outcome of a reserved key.
	Complete(ctx context.Context, key string, rec Record) error
	// Release frees a reserved key (the request failed with 5xx).
	Release(ctx context.Context, key string) error
}

// ------------------------------------------------------------------ redis

// RedisStore keeps records in Redis.
type RedisStore struct {
	rdb    *redis.Client
	prefix string
}

// NewRedisStore returns nil when rdb is nil (idempotency disabled).
func NewRedisStore(rdb *redis.Client, appEnv string) *RedisStore {
	if rdb == nil {
		return nil
	}
	return &RedisStore{rdb: rdb, prefix: fmt.Sprintf("app:%s:idem:", appEnv)}
}

func (s *RedisStore) Reserve(ctx context.Context, key, fingerprint string) (*Record, error) {
	b, _ := json.Marshal(Record{Fingerprint: fingerprint, Pending: true})
	ok, err := s.rdb.SetNX(ctx, s.prefix+key, b, pendingTTL).Result()
	if err != nil {
		return nil, err
	}
	if ok {
		return nil, nil
	}
	raw, err := s.rdb.Get(ctx, s.prefix+key).Bytes()
	if errors.Is(err, redis.Nil) {
		// Expired between SETNX and GET: try once more.
		return s.Reserve(ctx, key, fingerprint)
	}
	if err != nil {
		return nil, err
	}
	var rec Record
	if err := json.Unmarshal(raw, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

func (s *RedisStore) Complete(ctx context.Context, key string, rec Record) error {
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, s.prefix+key, b, TTL).Err()
}

func (s *RedisStore) Release(ctx context.Context, key string) error {
	return s.rdb.Del(ctx, s.prefix+key).Err()
}

// ------------------------------------------------------------------ memory

// MemoryStore is an in-process store for tests and single-node development.
type MemoryStore struct {
	mu   sync.Mutex
	recs map[string]Record
}

// NewMemoryStore returns an empty in-memory store.
func NewMemoryStore() *MemoryStore { return &MemoryStore{recs: map[string]Record{}} }

func (s *MemoryStore) Reserve(_ context.Context, key, fingerprint string) (*Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rec, ok := s.recs[key]; ok {
		return &rec, nil
	}
	s.recs[key] = Record{Fingerprint: fingerprint, Pending: true}
	return nil, nil
}

func (s *MemoryStore) Complete(_ context.Context, key string, rec Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recs[key] = rec
	return nil
}

func (s *MemoryStore) Release(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.recs, key)
	return nil
}

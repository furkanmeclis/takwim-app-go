package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/idempotency"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

const (
	// IdempotencyKeyHeader is the request header a client sets on unsafe
	// requests it may retry.
	IdempotencyKeyHeader = "Idempotency-Key"
	// IdempotentReplayedHeader marks a response replayed from the store.
	IdempotentReplayedHeader = "Idempotent-Replayed"

	// PublicIdempotencyPrefix: anonymous requests under it (the public
	// booking API) are replayed too.
	PublicIdempotencyPrefix = "/v1/public/businesses/"

	maxIdempotencyKey   = 255
	maxIdempotentBody   = 10 << 20
	maxIdempotentRecord = 1 << 20
)

// Idempotency replays the outcome of authenticated POST / PUT / PATCH /
// DELETE requests that carry an Idempotency-Key (see package idempotency),
// and of anonymous ones under PublicIdempotencyPrefix. Requests without the
// header, safe methods, other anonymous requests and a nil store pass
// through unchanged.
func Idempotency(store idempotency.Store, tokens *jwt.Manager, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if store == nil || tokens == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := strings.TrimSpace(r.Header.Get(IdempotencyKeyHeader))
			if key == "" || !unsafeMethod(r.Method) {
				next.ServeHTTP(w, r)
				return
			}
			subject := ""
			if claims, err := tokens.ParseAccess(bearerToken(r.Header.Get("Authorization"))); err == nil {
				subject = claims.Subject
			} else if strings.HasPrefix(r.URL.Path, PublicIdempotencyPrefix) {
				// Public booking API: anonymous callers share one scope; the
				// key (a client-generated uuid) and the body fingerprint
				// separate them.
				subject = "public"
			} else {
				// Authentication rejects it (or the route is public).
				next.ServeHTTP(w, r)
				return
			}
			if len(key) > maxIdempotencyKey || !printable(key) {
				response.BadRequest(w, r, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key must be 1-255 printable ASCII characters")
				return
			}
			body, err := io.ReadAll(io.LimitReader(r.Body, maxIdempotentBody+1))
			if err != nil {
				response.BadRequest(w, r, "INVALID_BODY", "Request body could not be read")
				return
			}
			_ = r.Body.Close()
			r.Body = io.NopCloser(bytes.NewReader(body))
			if len(body) > maxIdempotentBody {
				// Too large to fingerprint (uploads): no replay protection.
				r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(body), r.Body))
				next.ServeHTTP(w, r)
				return
			}

			scope := hash(subject, r.Header.Get("X-Organization"), r.Method, r.URL.Path, key)
			fp := hash(r.Method, r.URL.RequestURI(), string(body))
			ctx := r.Context()
			existing, err := store.Reserve(ctx, scope, fp)
			if err != nil {
				log.Warn("idempotency_store_unavailable", "error", err)
				next.ServeHTTP(w, r)
				return
			}
			if existing != nil {
				switch {
				case existing.Fingerprint != fp:
					response.Error(w, r, http.StatusUnprocessableEntity, "IDEMPOTENCY_KEY_REUSED", "Idempotency-Key was already used with a different request")
				case existing.Pending:
					response.Conflict(w, r, "IDEMPOTENCY_IN_PROGRESS", "A request with this Idempotency-Key is still running")
				default:
					replay(w, existing)
				}
				return
			}

			rec := &recordingWriter{ResponseWriter: w, status: http.StatusOK}
			defer func() {
				if p := recover(); p != nil {
					_ = store.Release(ctx, scope)
					panic(p)
				}
				// 5xx and 429 (rate limited) free the key: the retry runs again.
				if rec.status >= 500 || rec.status == http.StatusTooManyRequests || rec.overflow {
					_ = store.Release(ctx, scope)
					return
				}
				header := map[string]string{}
				for _, h := range []string{"Content-Type", "Location"} {
					if v := w.Header().Get(h); v != "" {
						header[h] = v
					}
				}
				if err := store.Complete(ctx, scope, idempotency.Record{
					Fingerprint: fp, Status: rec.status, Header: header, Body: rec.buf.Bytes(),
				}); err != nil {
					log.Warn("idempotency_store_complete_failed", "error", err)
					_ = store.Release(ctx, scope)
				}
			}()
			next.ServeHTTP(rec, r)
		})
	}
}

func replay(w http.ResponseWriter, rec *idempotency.Record) {
	for k, v := range rec.Header {
		w.Header().Set(k, v)
	}
	w.Header().Set(IdempotentReplayedHeader, "true")
	w.WriteHeader(rec.Status)
	_, _ = w.Write(rec.Body)
}

func unsafeMethod(m string) bool {
	switch m {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

func printable(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x21 || s[i] > 0x7e {
			return false
		}
	}
	return true
}

func hash(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		_, _ = io.WriteString(h, p)
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// recordingWriter tees the response so it can be stored.
type recordingWriter struct {
	http.ResponseWriter
	status   int
	wrote    bool
	buf      bytes.Buffer
	overflow bool
}

func (w *recordingWriter) WriteHeader(status int) {
	if !w.wrote {
		w.status, w.wrote = status, true
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *recordingWriter) Write(b []byte) (int, error) {
	w.wrote = true
	if !w.overflow {
		if w.buf.Len()+len(b) > maxIdempotentRecord {
			w.overflow = true
			w.buf.Reset()
		} else {
			w.buf.Write(b)
		}
	}
	return w.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (w *recordingWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

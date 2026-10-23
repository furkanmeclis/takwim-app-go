package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/auth/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/auth/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ratelimit"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// stubRepo implements only what these handler tests reach; any other
// Repository call panics on the nil embedded interface.
type stubRepo struct {
	usecase.Repository
	user model.User
}

func (s stubRepo) FindUserByEmail(context.Context, string) (model.User, error) {
	return model.User{}, repository.ErrNotFound
}

func (s stubRepo) FindUserByUUID(context.Context, uuid.UUID) (model.User, error) {
	return s.user, nil
}

func (s stubRepo) UserHasRoleSlug(context.Context, int64, string) (bool, error) { return false, nil }

type envelope struct {
	Success bool `json:"success"`
	Error   struct {
		Code    string `json:"code"`
		Details []struct {
			Field   string `json:"field"`
			Message string `json:"message"`
		} `json:"details"`
	} `json:"error"`
}

func newTestHandler(t *testing.T, repo usecase.Repository) *Handler {
	t.Helper()
	tokens, err := jwt.NewManager("test-secret-key-32-bytes-minimum!", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return New(usecase.New(repo, tokens), nil, nil, nil, nil, "adapter-secret", nil, nil)
}

func do(t *testing.T, fn http.HandlerFunc, path, body string, ctx context.Context) (*httptest.ResponseRecorder, envelope) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	if ctx != nil {
		req = req.WithContext(ctx)
	}
	rec := httptest.NewRecorder()
	fn(rec, req)
	var env envelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	return rec, env
}

func TestWriteUsecaseErrorAuthCodes(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{usecase.ErrAccountDeactivated, http.StatusForbidden, "ACCOUNT_DEACTIVATED"},
		{usecase.ErrUserDisabled, http.StatusForbidden, "FORBIDDEN"},
		{usecase.ErrInvalidEmailCode, http.StatusBadRequest, "INVALID_EMAIL_CODE"},
		{usecase.ErrInvalidIDToken, http.StatusUnauthorized, "INVALID_ID_TOKEN"},
		{usecase.ErrOAuthNotLinked, http.StatusConflict, "OAUTH_ACCOUNT_NOT_LINKED"},
		{usecase.ErrInvalidLinkTicket, http.StatusBadRequest, "INVALID_LINK_TICKET"},
		{usecase.ErrOAuthProviderDisabled, http.StatusForbidden, "OAUTH_PROVIDER_DISABLED"},
		{usecase.ErrConfirmationRequired, http.StatusForbidden, "STEP_UP_REQUIRED"},
		{usecase.ErrMFARequired, http.StatusForbidden, "MFA_REQUIRED"},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		writeUsecaseError(rec, httptest.NewRequest(http.MethodPost, "/", nil), c.err)
		var env envelope
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
		if rec.Code != c.status || env.Error.Code != c.code {
			t.Fatalf("%v: got %d %s, want %d %s", c.err, rec.Code, env.Error.Code, c.status, c.code)
		}
	}
}

func TestLinkChoiceRequiredDetails(t *testing.T) {
	rec := httptest.NewRecorder()
	writeUsecaseError(rec, httptest.NewRequest(http.MethodPost, "/", nil), &usecase.LinkChoiceRequiredError{
		Ticket: "tkt", Provider: "apple", ExpiresIn: 900, RegisterAllowed: true,
	})
	var env envelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if rec.Code != http.StatusConflict || env.Error.Code != "OAUTH_LINK_CHOICE_REQUIRED" {
		t.Fatalf("got %d %s", rec.Code, env.Error.Code)
	}
	got := map[string]string{}
	for _, d := range env.Error.Details {
		got[d.Field] = d.Message
	}
	if got["link_ticket"] != "tkt" || got["provider"] != "apple" || got["expires_in"] != "900" || got["register_allowed"] != "true" {
		t.Fatalf("details = %v", got)
	}
}

func TestEmailCodeRequestRateLimitedPerEmail(t *testing.T) {
	mr := miniredis.RunT(t)
	h := newTestHandler(t, stubRepo{})
	h.SetRateLimiter(ratelimit.New(redis.NewClient(&redis.Options{Addr: mr.Addr()}), "test"))
	for i := 0; i < 5; i++ {
		rec, env := do(t, h.RequestEmailCode, "/v1/auth/email-code/request", `{"email":"a@example.com"}`, nil)
		if rec.Code != http.StatusOK || !env.Success {
			t.Fatalf("request %d: %d %s", i, rec.Code, rec.Body.String())
		}
	}
	rec, env := do(t, h.RequestEmailCode, "/v1/auth/email-code/request", `{"email":"A@example.com "}`, nil)
	if rec.Code != http.StatusTooManyRequests || env.Error.Code != "RATE_LIMITED" {
		t.Fatalf("expected 429, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestEmailCodeVerifyInvalid(t *testing.T) {
	h := newTestHandler(t, stubRepo{})
	rec, env := do(t, h.VerifyEmailCode, "/v1/auth/email-code/verify", `{"email":"a@example.com","code":"123456"}`, nil)
	if rec.Code != http.StatusBadRequest || env.Error.Code != "INVALID_EMAIL_CODE" {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	rec, env = do(t, h.VerifyEmailCode, "/v1/auth/email-code/verify", `{"email":"a@example.com","code":"1","extra":1}`, nil)
	if rec.Code != http.StatusBadRequest || env.Error.Code != "VALIDATION_ERROR" {
		t.Fatalf("unknown fields must be rejected: %d %s", rec.Code, rec.Body.String())
	}
}

func TestNativeOAuthHandler(t *testing.T) {
	h := newTestHandler(t, stubRepo{})
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/oauth/github/native", strings.NewReader(`{"id_token":"x"}`))
	req.SetPathValue("provider", "github")
	rec := httptest.NewRecorder()
	h.NativeOAuth(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown provider: %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/auth/oauth/apple/native", strings.NewReader(`{"id_token":"x"}`))
	req.SetPathValue("provider", "apple")
	rec = httptest.NewRecorder()
	h.NativeOAuth(rec, req)
	var env envelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if rec.Code != http.StatusForbidden || env.Error.Code != "OAUTH_PROVIDER_DISABLED" {
		t.Fatalf("unconfigured native oauth: %d %s", rec.Code, rec.Body.String())
	}

	rec, env = do(t, h.OAuthLinkCreate, "/v1/auth/oauth/link/create", `{"link_ticket":"forged"}`, nil)
	if rec.Code != http.StatusBadRequest || env.Error.Code != "INVALID_LINK_TICKET" {
		t.Fatalf("forged ticket: %d %s", rec.Code, rec.Body.String())
	}
}

func TestDeactivateAccountHandler(t *testing.T) {
	uid := uuid.New()
	h := newTestHandler(t, stubRepo{user: model.User{ID: 7, UUID: uid, Email: "a@example.com", Status: "active"}})

	ctx := authctx.WithPrincipal(context.Background(), authctx.Principal{UserID: uid, UserInternal: 7})
	rec, env := do(t, h.DeactivateAccount, "/v1/auth/account/deactivate", `{}`, ctx)
	if rec.Code != http.StatusForbidden || env.Error.Code != "STEP_UP_REQUIRED" {
		t.Fatalf("without confirmation: %d %s", rec.Code, rec.Body.String())
	}

	imp := uuid.New()
	ctx = authctx.WithPrincipal(context.Background(), authctx.Principal{UserID: uid, UserInternal: 7, ImpersonatorUserID: &imp})
	rec, env = do(t, h.DeactivateAccount, "/v1/auth/account/deactivate", `{"code":"123456"}`, ctx)
	if rec.Code != http.StatusForbidden || env.Error.Code != "FORBIDDEN" {
		t.Fatalf("impersonation must be refused: %d %s", rec.Code, rec.Body.String())
	}
}

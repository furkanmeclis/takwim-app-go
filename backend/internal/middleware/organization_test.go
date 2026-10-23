package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

type resolverFunc func(ctx context.Context, ref string, userID int64) (orgctx.Scope, error)

func (f resolverFunc) Resolve(ctx context.Context, ref string, userID int64) (orgctx.Scope, error) {
	return f(ctx, ref, userID)
}

func errorCode(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	var env response.Envelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode body %q: %v", rr.Body.String(), err)
	}
	if env.Error == nil {
		return ""
	}
	return env.Error.Code
}

func withPrincipal(r *http.Request, p authctx.Principal) *http.Request {
	return r.WithContext(authctx.WithPrincipal(r.Context(), p))
}

func TestRequireOrganization(t *testing.T) {
	t.Parallel()
	acme := orgctx.Scope{
		ID: 7, UUID: uuid.New(), Slug: "acme", Name: "Acme", Status: orgctx.StatusActive,
		MemberRole: orgctx.RoleMember, Permissions: []string{"tenant.settings.read"},
	}
	cases := []struct {
		name       string
		header     string
		anon       bool
		superAdmin bool
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "missing header", header: "", wantStatus: http.StatusBadRequest, wantCode: response.CodeOrganizationRequired},
		{name: "blank header", header: "   ", wantStatus: http.StatusBadRequest, wantCode: response.CodeOrganizationRequired},
		{name: "too long", header: strings.Repeat("a", 65), wantStatus: http.StatusForbidden, wantCode: response.CodeNoTenantMembership},
		{name: "not found", header: "nope", err: orgctx.ErrOrganizationNotFound, wantStatus: http.StatusForbidden, wantCode: response.CodeNoTenantMembership},
		{name: "not member", header: "acme", err: fmt.Errorf("load: %w", orgctx.ErrNotMember), wantStatus: http.StatusForbidden, wantCode: response.CodeNoTenantMembership},
		{name: "super admin not member", header: "acme", superAdmin: true, err: orgctx.ErrNotMember, wantStatus: http.StatusForbidden, wantCode: response.CodeNoTenantMembership},
		{name: "suspended", header: "acme", err: orgctx.ErrSuspended, wantStatus: http.StatusForbidden, wantCode: response.CodeOrganizationSuspended},
		{name: "expired", header: "acme", err: orgctx.ErrAccessExpired, wantStatus: http.StatusForbidden, wantCode: response.CodeOrganizationAccessExpired},
		{name: "db error", header: "acme", err: errors.New("connection refused"), wantStatus: http.StatusInternalServerError, wantCode: response.CodeInternalError},
		{name: "unauthenticated", header: "acme", anon: true, wantStatus: http.StatusUnauthorized, wantCode: response.CodeUnauthenticated},
		{name: "ok", header: "  acme ", wantStatus: http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var gotRef string
			var gotUser int64
			resolver := resolverFunc(func(_ context.Context, ref string, userID int64) (orgctx.Scope, error) {
				gotRef, gotUser = ref, userID
				if tc.err != nil {
					return orgctx.Scope{}, tc.err
				}
				return acme, nil
			})
			var scope orgctx.Scope
			var hasScope bool
			h := RequireOrganization(resolver)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				scope, hasScope = orgctx.From(r.Context())
				w.WriteHeader(http.StatusOK)
			}))
			req := httptest.NewRequest(http.MethodGet, "/v1/tenant/context", nil)
			if tc.header != "" {
				req.Header.Set(orgctx.HeaderName, tc.header)
			}
			if !tc.anon {
				req = withPrincipal(req, authctx.Principal{UserInternal: 42, IsSuperAdmin: tc.superAdmin})
			}
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)

			if rr.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rr.Code, tc.wantStatus, rr.Body.String())
			}
			if tc.wantStatus != http.StatusOK {
				if code := errorCode(t, rr); code != tc.wantCode {
					t.Fatalf("code = %q, want %q", code, tc.wantCode)
				}
				if hasScope {
					t.Fatal("next handler must not run")
				}
				return
			}
			if !hasScope || scope.ID != acme.ID || scope.UUID != acme.UUID {
				t.Fatalf("scope = %+v (present %v), want acme", scope, hasScope)
			}
			if gotRef != "acme" || gotUser != 42 {
				t.Fatalf("resolver got (%q, %d), want (acme, 42)", gotRef, gotUser)
			}
		})
	}
}

func TestRequireTenantPermission(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		scope      *orgctx.Scope
		superAdmin bool
		wantStatus int
		wantCode   string
	}{
		{name: "granted", scope: &orgctx.Scope{Permissions: []string{"tenant.members.read"}}, wantStatus: http.StatusOK},
		{name: "missing", scope: &orgctx.Scope{Permissions: []string{"tenant.settings.read"}}, wantStatus: http.StatusForbidden, wantCode: response.CodeForbidden},
		{name: "super admin has no bypass", scope: &orgctx.Scope{}, superAdmin: true, wantStatus: http.StatusForbidden, wantCode: response.CodeForbidden},
		{name: "no scope", wantStatus: http.StatusInternalServerError, wantCode: response.CodeInternalError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := RequireTenantPermission("tenant.members.read")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			req := withPrincipal(httptest.NewRequest(http.MethodGet, "/v1/tenant/members", nil),
				authctx.Principal{UserInternal: 1, IsSuperAdmin: tc.superAdmin})
			if tc.scope != nil {
				req = req.WithContext(orgctx.WithScope(req.Context(), *tc.scope))
			}
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rr.Code, tc.wantStatus)
			}
			if tc.wantCode != "" {
				if code := errorCode(t, rr); code != tc.wantCode {
					t.Fatalf("code = %q, want %q", code, tc.wantCode)
				}
			}
		})
	}
}

// branchResolver is a resolver with X-Branch lookup.
type branchResolver struct {
	resolverFunc
	branches map[uuid.UUID]int64
}

func (b branchResolver) BranchIDByUUID(_ context.Context, _ int64, id uuid.UUID) (int64, error) {
	if v, ok := b.branches[id]; ok {
		return v, nil
	}
	return 0, orgctx.ErrBranchNotAllowed
}

func TestRequireOrganizationBranchHeader(t *testing.T) {
	t.Parallel()
	mine, other, foreign := uuid.New(), uuid.New(), uuid.New()
	// The member works in branches 5 and 9 only; branch 11 exists but is
	// outside its scope, foreign belongs to no branch of the organization.
	scope := orgctx.Scope{ID: 7, Slug: "acme", Status: orgctx.StatusActive, BranchIDs: []int64{5, 9}}
	resolver := branchResolver{
		resolverFunc: func(context.Context, string, int64) (orgctx.Scope, error) { return scope, nil },
		branches:     map[uuid.UUID]int64{mine: 9, other: 11},
	}
	cases := []struct {
		name       string
		header     string
		wantStatus int
		wantScope  []int64
	}{
		{name: "no header", header: "", wantStatus: http.StatusOK, wantScope: []int64{5, 9}},
		{name: "own branch", header: mine.String(), wantStatus: http.StatusOK, wantScope: []int64{9}},
		{name: "outside scope", header: other.String(), wantStatus: http.StatusForbidden},
		{name: "unknown", header: foreign.String(), wantStatus: http.StatusForbidden},
		{name: "not a uuid", header: "nope", wantStatus: http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var got orgctx.Scope
			var ctxScope []int64
			h := RequireOrganization(resolver)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got, _ = orgctx.From(r.Context())
				ctxScope = orgctx.BranchScope(r.Context())
				w.WriteHeader(http.StatusOK)
			}))
			req := httptest.NewRequest(http.MethodGet, "/v1/tenant/branches", nil)
			req.Header.Set(orgctx.HeaderName, "acme")
			if tc.header != "" {
				req.Header.Set(orgctx.BranchHeaderName, tc.header)
			}
			req = withPrincipal(req, authctx.Principal{UserInternal: 42})
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rr.Code, tc.wantStatus, rr.Body.String())
			}
			if tc.wantStatus != http.StatusOK {
				if code := errorCode(t, rr); code != response.CodeBranchNotAllowed {
					t.Fatalf("code = %q", code)
				}
				return
			}
			if fmt.Sprint(ctxScope) != fmt.Sprint(tc.wantScope) {
				t.Fatalf("BranchScope = %v, want %v", ctxScope, tc.wantScope)
			}
			// Access checks keep the member's full scope.
			if fmt.Sprint(got.BranchIDs) != "[5 9]" {
				t.Fatalf("scope.BranchIDs = %v", got.BranchIDs)
			}
		})
	}

	// A resolver without branch lookup ignores the header.
	plain := resolverFunc(func(context.Context, string, int64) (orgctx.Scope, error) { return scope, nil })
	h := RequireOrganization(plain)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	req := httptest.NewRequest(http.MethodGet, "/v1/tenant/branches", nil)
	req.Header.Set(orgctx.HeaderName, "acme")
	req.Header.Set(orgctx.BranchHeaderName, "nope")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, withPrincipal(req, authctx.Principal{UserInternal: 42}))
	if rr.Code != http.StatusOK {
		t.Fatalf("plain resolver status = %d", rr.Code)
	}
}

func TestRequireAnyTenantPermission(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		scope      *orgctx.Scope
		wantStatus int
	}{
		{name: "first", scope: &orgctx.Scope{Permissions: []string{"tenant.reservations.read"}}, wantStatus: http.StatusOK},
		{name: "second", scope: &orgctx.Scope{Permissions: []string{"tenant.reservations.own"}}, wantStatus: http.StatusOK},
		{name: "none", scope: &orgctx.Scope{Permissions: []string{"tenant.settings.read"}}, wantStatus: http.StatusForbidden},
		{name: "no scope", wantStatus: http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := RequireAnyTenantPermission("tenant.reservations.read", "tenant.reservations.own")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			req := withPrincipal(httptest.NewRequest(http.MethodPost, "/v1/tenant/availability/search", nil), authctx.Principal{UserInternal: 1})
			if tc.scope != nil {
				req = req.WithContext(orgctx.WithScope(req.Context(), *tc.scope))
			}
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rr.Code, tc.wantStatus)
			}
		})
	}
}

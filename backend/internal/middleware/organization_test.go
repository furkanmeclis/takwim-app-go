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

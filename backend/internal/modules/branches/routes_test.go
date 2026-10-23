package branches

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	branchhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

type stubLoader struct{}

func (stubLoader) LoadPrincipal(_ *http.Request, _ jwt.Claims) (authctx.Principal, error) {
	// super_admin on the platform: tenant routes must still ignore it.
	return authctx.Principal{UserID: uuid.New(), UserInternal: 42, IsSuperAdmin: true}, nil
}

type stubResolver map[string]orgctx.Scope

func (s stubResolver) Resolve(_ context.Context, ref string, _ int64) (orgctx.Scope, error) {
	scope, ok := s[ref]
	if !ok {
		return orgctx.Scope{}, orgctx.ErrNotMember
	}
	return scope, nil
}

func TestTenantPermissionGates(t *testing.T) {
	t.Parallel()
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	access, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	resolver := stubResolver{
		"owner-org":  {ID: 1, Slug: "owner-org", MemberRole: "owner", Permissions: []string{rbac.PermTenantBranchesRead, rbac.PermTenantBranchesWrite}},
		"member-org": {ID: 2, Slug: "member-org", MemberRole: "member", Permissions: []string{rbac.PermTenantBranchesRead}},
	}
	mux := http.NewServeMux()
	// The use case is never reached in these cases (gates or body decoding fail first).
	RegisterRoutes(mux, branchhandler.New(usecase.New(nil, nil), nil), EngineHandlers{}, tokens, stubLoader{}, resolver)

	do := func(method, path, org, body string) int {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+access)
		if org != "" {
			req.Header.Set(orgctx.HeaderName, org)
		}
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		return rr.Code
	}

	cases := []struct {
		name, method, path, org, body string
		want                          int
	}{
		{"missing organization header", http.MethodGet, "/v1/tenant/branches", "", "", http.StatusBadRequest},
		{"not a member", http.MethodGet, "/v1/tenant/branches", "other-org", "", http.StatusForbidden},
		{"member cannot create", http.MethodPost, "/v1/tenant/branches", "member-org", "{}", http.StatusForbidden},
		{"member cannot delete", http.MethodDelete, "/v1/tenant/branches/" + uuid.NewString(), "member-org", "", http.StatusForbidden},
		{"member reads meta", http.MethodGet, "/v1/tenant/branches/meta", "member-org", "", http.StatusOK},
		{"owner passes the write gate", http.MethodPost, "/v1/tenant/branches", "owner-org", "{not json", http.StatusBadRequest},
		{"export route not mounted without engine", http.MethodPost, "/v1/tenant/branches/export", "owner-org", "{}", http.StatusMethodNotAllowed},
	}
	for _, tc := range cases {
		if got := do(tc.method, tc.path, tc.org, tc.body); got != tc.want {
			t.Errorf("%s: status = %d, want %d", tc.name, got, tc.want)
		}
	}
}

package tenant

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
)

type stubLoader struct{}

func (stubLoader) LoadPrincipal(_ *http.Request, _ jwt.Claims) (authctx.Principal, error) {
	return authctx.Principal{UserID: uuid.New(), UserInternal: 42}, nil
}

type stubResolver struct{ scope orgctx.Scope }

func (s stubResolver) Resolve(_ context.Context, ref string, userID int64) (orgctx.Scope, error) {
	if ref != s.scope.Slug || userID != 42 {
		return orgctx.Scope{}, orgctx.ErrNotMember
	}
	return s.scope, nil
}

func TestContextRoute(t *testing.T) {
	t.Parallel()
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	access, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	ends := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	scope := orgctx.Scope{
		ID: 3, UUID: uuid.New(), Slug: "acme", Name: "Acme", Status: orgctx.StatusActive,
		AccessEndsAt: &ends, Sector: "restaurant", Timezone: "Europe/Istanbul", Currency: "TRY", MemberRole: orgctx.RoleOwner, Permissions: []string{"tenant.settings.read"},
	}

	mux := http.NewServeMux()
	RegisterRoutes(mux, tokens, stubLoader{}, stubResolver{scope: scope})

	do := func(org string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/v1/tenant/context", nil)
		req.Header.Set("Authorization", "Bearer "+access)
		if org != "" {
			req.Header.Set(orgctx.HeaderName, org)
		}
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		return rr
	}

	rr := do("acme")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Data struct {
			Organization struct {
				UUID         string     `json:"uuid"`
				Slug         string     `json:"slug"`
				AccessEndsAt *time.Time `json:"access_ends_at"`
				Sector       string     `json:"sector"`
				Timezone     string     `json:"timezone"`
			} `json:"organization"`
			Sector struct {
				Code     string   `json:"code"`
				Modules  []string `json:"modules"`
				Capacity struct {
					PartySize bool `json:"party_size"`
				} `json:"capacity"`
				Terms struct {
					Resource string `json:"resource"`
				} `json:"terms"`
			} `json:"sector"`
			Role        string   `json:"role"`
			Permissions []string `json:"permissions"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	d := body.Data
	if d.Organization.UUID != scope.UUID.String() || d.Organization.Slug != "acme" || d.Role != orgctx.RoleOwner ||
		len(d.Permissions) != 1 || d.Organization.AccessEndsAt == nil || !d.Organization.AccessEndsAt.Equal(ends) {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
	if d.Organization.Sector != "restaurant" || d.Organization.Timezone != "Europe/Istanbul" ||
		d.Sector.Code != "restaurant" || !d.Sector.Capacity.PartySize || d.Sector.Terms.Resource != "table" || len(d.Sector.Modules) == 0 {
		t.Fatalf("unexpected sector: %s", rr.Body.String())
	}

	sectors := httptest.NewRequest(http.MethodGet, "/v1/sectors", nil)
	sectors.Header.Set("Authorization", "Bearer "+access)
	src := httptest.NewRecorder()
	mux.ServeHTTP(src, sectors)
	var catalog struct {
		Data struct {
			Items []struct {
				Code string `json:"code"`
			} `json:"items"`
		} `json:"data"`
	}
	if src.Code != http.StatusOK || json.Unmarshal(src.Body.Bytes(), &catalog) != nil || len(catalog.Data.Items) != 6 {
		t.Fatalf("sectors: %d %s", src.Code, src.Body.String())
	}
	anon := httptest.NewRecorder()
	mux.ServeHTTP(anon, httptest.NewRequest(http.MethodGet, "/v1/sectors", nil))
	if anon.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous sectors status = %d", anon.Code)
	}

	if rr := do(""); rr.Code != http.StatusBadRequest {
		t.Fatalf("missing header status = %d", rr.Code)
	}
	if rr := do("other"); rr.Code != http.StatusForbidden {
		t.Fatalf("non-member status = %d", rr.Code)
	}
}

func TestNilResolverRegistersNothing(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	RegisterRoutes(mux, nil, nil, nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/tenant/context", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sector"
)

func TestRequireModule(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	do := func(module sector.Module, scope orgctx.Scope) int {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req = req.WithContext(orgctx.WithScope(req.Context(), scope))
		rr := httptest.NewRecorder()
		RequireModule(module)(ok).ServeHTTP(rr, req)
		return rr.Code
	}
	salon := orgctx.Scope{ID: 1, Sector: "hairdresser"}
	if got := do(sector.ModuleStaffShares, salon); got != http.StatusNoContent {
		t.Fatalf("sector module = %d", got)
	}
	if got := do(sector.ModuleClasses, salon); got != http.StatusForbidden {
		t.Fatalf("hidden sector module = %d", got)
	}
	if got := do(sector.ModuleConfirmation, salon); got != http.StatusForbidden {
		t.Fatalf("add-on without grant = %d", got)
	}
	salon.Addons = []string{"confirmation"}
	if got := do(sector.ModuleConfirmation, salon); got != http.StatusNoContent {
		t.Fatalf("granted add-on = %d", got)
	}
}

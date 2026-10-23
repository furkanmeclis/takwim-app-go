package organizations

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	orghandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
)

func RegisterRoutes(
	mux *http.ServeMux,
	h *orghandler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
) {
	authn := middleware.Authenticate(tokens, loader)
	read := middleware.RequirePermission(rbac.PermPlatformOrganizationsRead)
	write := middleware.RequirePermission(rbac.PermPlatformOrganizationsWrite)

	mux.Handle("GET /v1/platform/organizations/meta", middleware.Chain(
		http.HandlerFunc(h.Meta), authn, read,
	))
	mux.Handle("GET /v1/platform/organizations", middleware.Chain(
		http.HandlerFunc(h.List), authn, read,
	))
	mux.Handle("POST /v1/platform/organizations", middleware.Chain(
		http.HandlerFunc(h.Create), authn, write,
	))
	mux.Handle("GET /v1/platform/organizations/{uuid}", middleware.Chain(
		http.HandlerFunc(h.Get), authn, read,
	))
	mux.Handle("PATCH /v1/platform/organizations/{uuid}", middleware.Chain(
		http.HandlerFunc(h.Patch), authn, write,
	))
	mux.Handle("DELETE /v1/platform/organizations/{uuid}", middleware.Chain(
		http.HandlerFunc(h.Delete), authn, write,
	))
	mux.Handle("POST /v1/platform/organizations/{uuid}/status", middleware.Chain(
		http.HandlerFunc(h.SetStatus), authn, write,
	))
	mux.Handle("POST /v1/platform/organizations/{uuid}/access", middleware.Chain(
		http.HandlerFunc(h.SetAccess), authn, write,
	))
	mux.Handle("GET /v1/platform/organizations/{uuid}/members", middleware.Chain(
		http.HandlerFunc(h.ListMembers), authn, read,
	))
	mux.Handle("POST /v1/platform/organizations/{uuid}/members", middleware.Chain(
		http.HandlerFunc(h.AddMember), authn, write,
	))
	mux.Handle("PATCH /v1/platform/organizations/{uuid}/members/{userUuid}", middleware.Chain(
		http.HandlerFunc(h.PatchMember), authn, write,
	))
	mux.Handle("DELETE /v1/platform/organizations/{uuid}/members/{userUuid}", middleware.Chain(
		http.HandlerFunc(h.DeleteMember), authn, write,
	))
}

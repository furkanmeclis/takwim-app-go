package middleware

import (
	"errors"
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sector"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// RequireModule answers 403 MODULE_NOT_ENABLED when the organization may
// not use a panel module: its sector does not show it, or it is an add-on
// (Konfirmasyon) that is not granted (TEC-883). Must run after
// RequireOrganization; the panel hides the same screens.
func RequireModule(module sector.Module) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			scope, ok := orgctx.From(r.Context())
			if !ok {
				response.InternalErr(w, r, errors.New("RequireModule without RequireOrganization"), "")
				return
			}
			if !entitlement.ForScope(scope).Allows(module) {
				response.Error(w, r, http.StatusForbidden, response.CodeModuleNotEnabled, "This module is not enabled for the organization")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

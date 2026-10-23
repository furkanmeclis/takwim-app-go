package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/observability"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/getsentry/sentry-go"
	"github.com/google/uuid"
)

// maxOrganizationRefLen bounds the X-Organization value: slugs are at most
// 64 characters and uuids 36. Longer values cannot match an organization, so
// they are answered like any other unknown reference without a lookup.
const maxOrganizationRefLen = 64

// RequireOrganization resolves the current organization from the
// X-Organization header (slug or uuid) and the caller's membership in it, and
// stores the orgctx.Scope on the request context. It must run after
// Authenticate. super_admin is not exempt: tenant routes need a membership.
//
// Unknown organization and missing membership get the same answer
// (403 NO_TENANT_MEMBERSHIP) so the header cannot be used to probe which
// organizations exist.
func RequireOrganization(resolver orgctx.Resolver) func(http.Handler) http.Handler {
	if resolver == nil {
		panic("middleware: RequireOrganization needs a resolver")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := authctx.PrincipalFrom(r.Context())
			if !ok {
				response.Unauthorized(w, r, "Authentication is required")
				return
			}
			ref := strings.TrimSpace(r.Header.Get(orgctx.HeaderName))
			if ref == "" {
				response.BadRequest(w, r, response.CodeOrganizationRequired, orgctx.HeaderName+" header is required")
				return
			}
			if len(ref) > maxOrganizationRefLen {
				writeNoMembership(w, r, errors.New("organization reference too long"))
				return
			}
			scope, err := resolver.Resolve(r.Context(), ref, p.UserInternal)
			if err != nil {
				writeOrganizationError(w, r, err)
				return
			}
			if scope, ok = selectBranch(w, r, resolver, scope); !ok {
				return
			}
			tagOrganization(r, scope)
			next.ServeHTTP(w, r.WithContext(orgctx.WithScope(r.Context(), scope)))
		})
	}
}

// selectBranch applies the optional X-Branch header (branch uuid). A branch
// that does not exist, belongs to another organization or is outside the
// caller's branch scope answers 403 BRANCH_NOT_ALLOWED, so the panel can
// drop a stale selection. Resolvers without branch lookup ignore the header.
func selectBranch(w http.ResponseWriter, r *http.Request, resolver orgctx.Resolver, scope orgctx.Scope) (orgctx.Scope, bool) {
	raw := strings.TrimSpace(r.Header.Get(orgctx.BranchHeaderName))
	selector, ok := resolver.(orgctx.BranchSelector)
	if raw == "" || !ok {
		return scope, true
	}
	branchUUID, err := uuid.Parse(raw)
	if err != nil {
		writeBranchNotAllowed(w, r, err)
		return scope, false
	}
	branchID, err := selector.BranchIDByUUID(r.Context(), scope.ID, branchUUID)
	if err == nil {
		scope, err = scope.SelectBranch(branchID)
	}
	if err != nil {
		if errors.Is(err, orgctx.ErrBranchNotAllowed) {
			writeBranchNotAllowed(w, r, err)
		} else {
			response.InternalErr(w, r, err, "")
		}
		return scope, false
	}
	return scope, true
}

func writeBranchNotAllowed(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	response.Error(w, r, http.StatusForbidden, response.CodeBranchNotAllowed, "The selected branch is not available")
}

// RequireTenantPermission requires a tenant permission slug in the current
// organization scope. Tenant permissions come from the membership only:
// there is no super_admin bypass. Must run after RequireOrganization.
func RequireTenantPermission(slug string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			scope, ok := orgctx.From(r.Context())
			if !ok {
				// Route wiring bug, not a client error.
				response.InternalErr(w, r, errors.New("RequireTenantPermission without RequireOrganization"), "")
				return
			}
			if !scope.Has(slug) {
				response.Forbidden(w, r, "Missing permission: "+slug)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeOrganizationError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, orgctx.ErrOrganizationNotFound), errors.Is(err, orgctx.ErrNotMember):
		writeNoMembership(w, r, err)
	case errors.Is(err, orgctx.ErrSuspended):
		response.RecordFailure(w, err)
		response.Error(w, r, http.StatusForbidden, response.CodeOrganizationSuspended, "Organization is suspended")
	case errors.Is(err, orgctx.ErrAccessExpired):
		response.RecordFailure(w, err)
		response.Error(w, r, http.StatusForbidden, response.CodeOrganizationAccessExpired, "Organization access has expired")
	default:
		response.InternalErr(w, r, err, "")
	}
}

func writeNoMembership(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	response.Error(w, r, http.StatusForbidden, response.CodeNoTenantMembership, "You are not a member of this organization")
}

// tagOrganization tags the request-scoped error tracker hub (set up by
// ServerErrors) with the organization uuid.
func tagOrganization(r *http.Request, scope orgctx.Scope) {
	if !observability.Enabled() {
		return
	}
	if hub := sentry.GetHubFromContext(r.Context()); hub != nil {
		hub.Scope().SetTag("organization_uuid", scope.UUID.String())
	}
}

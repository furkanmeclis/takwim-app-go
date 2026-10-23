package usecase

import (
	"context"
	"log/slog"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
)

// Service coordinates search queries and spec listing.
type Service struct {
	client *searchengine.Client
	reg    *searchengine.Registry
	log    *slog.Logger
}

// New creates a search service.
func New(client *searchengine.Client, reg *searchengine.Registry, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{client: client, reg: reg, log: log}
}

// Enabled reports whether remote search is available.
func (s *Service) Enabled() bool {
	return s != nil && s.client != nil && s.client.Enabled()
}

// ListSpecs returns specs visible to the principal.
func (s *Service) ListSpecs(ctx context.Context) []searchengine.Spec {
	if s == nil || s.reg == nil {
		return nil
	}
	p, ok := authctx.PrincipalFrom(ctx)
	if !ok {
		return nil
	}
	out := make([]searchengine.Spec, 0, len(s.reg.Specs()))
	for _, spec := range s.reg.Specs() {
		// Organization-owned records are searched through the tenant shell only.
		if spec.Scope == searchengine.ScopeTenant {
			continue
		}
		if spec.Permission == "" || p.HasPermission(spec.Permission) {
			out = append(out, spec)
		}
	}
	return out
}

// Search queries indexed records for allowed specs.
func (s *Service) Search(ctx context.Context, q, spec string, limit int) []searchengine.Hit {
	if !s.Enabled() {
		return nil
	}
	q = strings.TrimSpace(q)
	if q == "" {
		return nil
	}
	specs := s.allowedSpecs(ctx, spec)
	if len(specs) == 0 {
		return nil
	}
	hits, err := s.client.Search(ctx, specs, q, limit)
	if err != nil {
		s.log.Warn("search_query_failed", "error", err)
		return nil
	}
	return hits
}

func (s *Service) allowedSpecs(ctx context.Context, requested string) []string {
	requested = strings.TrimSpace(requested)
	all := s.ListSpecs(ctx)
	if requested != "" {
		for _, spec := range all {
			if spec.ID == requested {
				return []string{requested}
			}
		}
		return nil
	}
	out := make([]string, 0, len(all))
	for _, spec := range all {
		out = append(out, spec.ID)
	}
	return out
}

// ListTenantSpecs returns the tenant specs the caller may search in the
// current organization (permissions from orgctx.Scope, never platform roles).
func (s *Service) ListTenantSpecs(ctx context.Context) []searchengine.Spec {
	if s == nil || s.reg == nil {
		return nil
	}
	scope, ok := orgctx.From(ctx)
	if !ok {
		return nil
	}
	out := make([]searchengine.Spec, 0)
	for _, spec := range s.reg.Specs() {
		if spec.Scope != searchengine.ScopeTenant {
			continue
		}
		switch tenantReach(scope, spec) {
		case orgctx.ReachAll:
			out = append(out, spec)
		case orgctx.ReachOwn:
			// The client checks the slug it is given: an own-records member
			// holds the own slug only.
			own := spec
			own.Permission = spec.OwnPermission
			out = append(out, own)
		}
	}
	return out
}

// tenantReach is how far the caller reaches into a tenant spec: every record
// (its permission, or none needed), its own records (OwnPermission only) or
// nothing.
func tenantReach(scope orgctx.Scope, spec searchengine.Spec) orgctx.Reach {
	if spec.Permission == "" {
		return orgctx.ReachAll
	}
	return scope.Reach(spec.Permission, spec.OwnPermission)
}

// SearchTenant queries tenant specs, filtered to the current organization.
func (s *Service) SearchTenant(ctx context.Context, q, spec string, limit int) []searchengine.Hit {
	if !s.Enabled() {
		return nil
	}
	q = strings.TrimSpace(q)
	if q == "" {
		return nil
	}
	orgID, ok := orgctx.OrganizationID(ctx)
	if !ok {
		return nil
	}
	requested := strings.TrimSpace(spec)
	branchIDs := orgctx.BranchScope(ctx)
	scope, _ := orgctx.From(ctx)
	var userID int64
	if p, ok := authctx.PrincipalFrom(ctx); ok {
		userID = p.UserInternal
	}
	specs := make([]string, 0)
	filters := make([]string, 0)
	for _, sp := range s.reg.Specs() {
		if sp.Scope != searchengine.ScopeTenant || (requested != "" && sp.ID != requested) {
			continue
		}
		var own int64
		switch tenantReach(scope, sp) {
		case orgctx.ReachNone:
			continue
		case orgctx.ReachOwn:
			if userID <= 0 {
				continue
			}
			own = userID
		}
		filter, ok := searchengine.TenantOwnFilter(sp, orgID, branchIDs, own)
		if !ok {
			continue
		}
		specs = append(specs, sp.ID)
		filters = append(filters, filter)
	}
	if len(specs) == 0 {
		return nil
	}
	hits, err := s.client.SearchPerSpec(ctx, specs, filters, q, limit)
	if err != nil {
		s.log.Warn("search_tenant_query_failed", "error", err)
		return nil
	}
	return hits
}

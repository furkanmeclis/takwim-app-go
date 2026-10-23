package searchengine

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// Spec scopes.
const (
	// ScopePlatform specs are searched from the platform shell (/v1/search).
	ScopePlatform = ""
	// ScopeTenant specs hold organization-owned records. They are searched
	// only through /v1/tenant/search, filtered by organization_id.
	ScopeTenant = "tenant"
)

// Spec describes one searchable resource catalog entry.
type Spec struct {
	ID         string   `json:"id"`
	LabelKey   string   `json:"label_key"`
	Permission string   `json:"permission,omitempty"`
	Icon       string   `json:"icon,omitempty"`
	Searchable []string `json:"searchable_fields,omitempty"`
	Filterable []string `json:"filterable_fields,omitempty"`
	// Scope is ScopePlatform (empty) or ScopeTenant.
	Scope string `json:"scope,omitempty"`
	// BranchScoped tenant specs index Document.BranchIDs; a tenant search
	// by a branch-restricted member only matches its branches.
	BranchScoped bool `json:"-"`
	// OwnPermission is the ".own" slug of a tenant spec whose records belong
	// to members (own reservations). A member holding only this slug sees the
	// spec and matches only documents whose OwnerUserIDs hold its user id.
	OwnPermission string `json:"-"`
}

// Document is the Meilisearch index payload for one entity.
type Document struct {
	ID       string   `json:"id"`
	Spec     string   `json:"spec"`
	Title    string   `json:"title"`
	Subtitle string   `json:"subtitle,omitempty"`
	Keywords []string `json:"keywords,omitempty"`
	Href     string   `json:"href"`
	Icon     string   `json:"icon,omitempty"`
	// OrganizationID is set on tenant documents; tenant searches filter on it.
	OrganizationID int64 `json:"organization_id,omitempty"`
	// BranchIDs are the branches a branch-bound tenant document belongs to
	// (BranchScoped specs).
	BranchIDs []int64 `json:"branch_ids,omitempty"`
	// OwnerUserIDs are the users who own the document (specs with an
	// OwnPermission): an own-records member matches only these.
	OwnerUserIDs []int64 `json:"owner_user_ids,omitempty"`
}

// Hit is a normalized search result returned to clients.
type Hit struct {
	Spec     string  `json:"spec"`
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Subtitle string  `json:"subtitle,omitempty"`
	Href     string  `json:"href"`
	Icon     string  `json:"icon,omitempty"`
	Score    float64 `json:"score,omitempty"`
}

// Adapter connects searchengine to a list resource.
type Adapter interface {
	Spec() Spec
	ListAll(ctx context.Context) ([]Document, error)
	Document(ctx context.Context, id string) (Document, error)
}

// TenantDocumentID builds the document id of an organization-owned record:
// "<organization id>_<record id>". The indexer passes it back to
// Adapter.Document, so tenant adapters can look the record up by
// organization (every tenant query filters by organization_id).
func TenantDocumentID(organizationID int64, id string) string {
	return strconv.FormatInt(organizationID, 10) + "_" + id
}

// ParseTenantDocumentID splits a TenantDocumentID.
func ParseTenantDocumentID(docID string) (int64, string, error) {
	org, id, ok := strings.Cut(strings.TrimSpace(docID), "_")
	if !ok || id == "" {
		return 0, "", fmt.Errorf("searchengine: invalid tenant document id %q", docID)
	}
	orgID, err := strconv.ParseInt(org, 10, 64)
	if err != nil || orgID <= 0 {
		return 0, "", fmt.Errorf("searchengine: invalid tenant document id %q", docID)
	}
	return orgID, id, nil
}

// TenantFilter builds the trusted Meilisearch filter of a tenant search in
// spec for one organization and the caller's branch scope (nil = every
// branch). ok is false when the spec must be skipped: a branch-scoped spec
// for a member without any branch.
func TenantFilter(spec Spec, organizationID int64, branchIDs []int64) (filter string, ok bool) {
	return TenantOwnFilter(spec, organizationID, branchIDs, 0)
}

// TenantOwnFilter is TenantFilter for a caller whose reach over the spec is
// its own records only (ownUserID > 0): documents must also list the user in
// owner_user_ids. ownUserID 0 = every record inside the branch scope.
func TenantOwnFilter(spec Spec, organizationID int64, branchIDs []int64, ownUserID int64) (filter string, ok bool) {
	filter = "organization_id = " + strconv.FormatInt(organizationID, 10)
	if ownUserID > 0 {
		if spec.OwnPermission == "" {
			return "", false
		}
		filter += " AND owner_user_ids = " + strconv.FormatInt(ownUserID, 10)
	}
	if !spec.BranchScoped || branchIDs == nil {
		return filter, true
	}
	if len(branchIDs) == 0 {
		return "", false
	}
	ids := make([]string, 0, len(branchIDs))
	for _, id := range branchIDs {
		ids = append(ids, strconv.FormatInt(id, 10))
	}
	return filter + " AND branch_ids IN [" + strings.Join(ids, ", ") + "]", true
}

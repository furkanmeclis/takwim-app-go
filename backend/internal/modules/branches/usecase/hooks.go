package usecase

import (
	"context"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/repository"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
	"github.com/jackc/pgx/v5"
)

// OrganizationHooks seeds the default branch ("Merkez", code MRK) when an
// organization is created. It implements organizations/usecase.OrganizationHooks.
type OrganizationHooks struct {
	indexer SearchIndexer
}

// NewOrganizationHooks builds the hooks. indexer may be nil.
func NewOrganizationHooks(indexer SearchIndexer) *OrganizationHooks {
	return &OrganizationHooks{indexer: indexer}
}

// OnCreated runs inside the organization-create transaction.
func (h *OrganizationHooks) OnCreated(ctx context.Context, tx pgx.Tx, org orgmodel.Organization, _ orgmodel.User) error {
	repo := repository.NewPostgres(db.New(tx))
	row, err := repo.CreateBranch(ctx, org.ID, DefaultBranchFields())
	if err != nil {
		return err
	}
	if h.indexer != nil {
		h.indexer.EnqueueUpsert(ctx, SearchSpec, searchengine.TenantDocumentID(org.ID, row.Uuid.String()))
	}
	return nil
}

// OnDeleted keeps branches: organizations are soft-deleted and a hard delete
// cascades through the foreign key.
func (h *OrganizationHooks) OnDeleted(context.Context, orgmodel.Organization) error {
	return nil
}

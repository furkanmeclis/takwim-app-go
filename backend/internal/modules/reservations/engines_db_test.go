package reservations_test

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"testing"

	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
)

// engineScope is a request context of a member: the scope (permissions,
// branch scope) and the principal.
func engineScope(f *fixture, userID int64, branchIDs []int64, perms ...string) context.Context {
	ctx := authctx.WithPrincipal(context.Background(), authctx.Principal{UserInternal: userID})
	return orgctx.WithScope(ctx, orgctx.Scope{
		ID: f.org.ID, UUID: f.org.UUID, Slug: f.org.Slug, Timezone: "Europe/Istanbul",
		Permissions: perms, BranchIDs: branchIDs,
	})
}

func exportUUIDs(t *testing.T, io *adapters.IO, ctx context.Context, query ioengine.ExportQuery) []string {
	t.Helper()
	ds, err := io.Export(ctx, query, i18n.LocaleTR)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	out := make([]string, 0, len(ds.Rows))
	for _, row := range ds.Rows {
		out = append(out, row["uuid"].(string))
	}
	return out
}

// applyAll runs a bulk action like the bulk service does (resolve, then
// each item) and returns the summary.
func applyAll(t *testing.T, bulk *adapters.Bulk, ctx context.Context, action string, target bulkengine.BulkTarget) (bulkengine.BulkSummary, error) {
	t.Helper()
	ids, err := bulk.ResolveTargets(ctx, action, target)
	if err != nil {
		return bulkengine.BulkSummary{}, err
	}
	summary := bulkengine.BulkSummary{Total: len(ids)}
	ctx = bulkengine.WithParams(ctx, target.Params)
	for _, id := range ids {
		res, err := bulk.ApplyItem(ctx, action, id)
		if err != nil {
			t.Fatalf("apply %s %s: %v", action, id, err)
		}
		summary.Add(res)
	}
	return summary, nil
}

// TestEngineReach (TEC-963): export, bulk and search documents stay inside
// the caller's reach: another branch, another member's own calendar and a
// caller without the slug see nothing; a job runs with its requester.
func TestEngineReach(t *testing.T) {
	f := newFixture(t)
	bg := context.Background()
	r1, err := f.svc.Create(bg, f.org.ID, f.input(at(9, 0), "", "", f.cut), none, f.owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := f.svc.Create(bg, f.org.ID, f.input(at(10, 0), "", "", f.wash), none, f.owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	var branchID, otherBranch int64
	if err := f.pool.QueryRow(bg, "SELECT id FROM branches WHERE organization_id = $1 AND uuid = $2", f.org.ID, f.branch).Scan(&branchID); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(bg, "INSERT INTO branches (organization_id, code, name) VALUES ($1, 'B2', 'Diğer') RETURNING id", f.org.ID).Scan(&otherBranch); err != nil {
		t.Fatal(err)
	}
	io, bulk := adapters.NewIO(f.svc), adapters.NewBulk(f.svc)
	read := []string{rbac.PermTenantReservationsRead, rbac.PermTenantReservationsWrite}
	both := []string{r1.UUID.String(), r2.UUID.String()}

	// Every branch: both rows, labels in the business language and timezone.
	all := engineScope(f, f.owner.ID, nil, read...)
	if got := exportUUIDs(t, io, all, nil); len(got) != 2 {
		t.Fatalf("full export = %v", got)
	}
	ds, err := io.Export(all, ioengine.ExportQuery{"q": strconv.FormatInt(r1.Number, 10)}, i18n.LocaleTR)
	if err != nil || len(ds.Rows) != 1 {
		t.Fatalf("filtered export = %v %v", ds.Rows, err)
	}
	if row := ds.Rows[0]; row["status"] != "Aktif" || row["starts_at"] != "09:00" || row["date"] != "2030-01-07" {
		t.Fatalf("export row = %v", row)
	}
	if en, _ := io.Export(all, ioengine.ExportQuery{"q": strconv.FormatInt(r1.Number, 10)}, i18n.LocaleEN); en.Rows[0]["status"] != "Active" {
		t.Fatalf("en status = %v", en.Rows[0]["status"])
	}

	// Another branch only: nothing exported, nothing changed in bulk.
	elsewhere := engineScope(f, f.owner.ID, []int64{otherBranch}, read...)
	if got := exportUUIDs(t, io, elsewhere, nil); len(got) != 0 {
		t.Fatalf("other branch export = %v", got)
	}
	s, err := applyAll(t, bulk, elsewhere, "cancel", bulkengine.BulkTarget{Scope: "ids", IDs: both})
	if err != nil || s.Succeeded != 0 || s.Failed != 2 {
		t.Fatalf("other branch bulk = %+v %v", s, err)
	}
	if s, err := applyAll(t, bulk, elsewhere, "cancel", bulkengine.BulkTarget{Scope: "query"}); err != nil || s.Total != 0 {
		t.Fatalf("other branch query bulk = %+v %v", s, err)
	}

	// Own calendar: fatma neither booked nor serves them; emre serves both.
	fatma := engineScope(f, f.fatma.ID, nil, rbac.PermTenantReservationsOwn)
	if got := exportUUIDs(t, io, fatma, nil); len(got) != 0 {
		t.Fatalf("fatma export = %v", got)
	}
	if s, _ := applyAll(t, bulk, fatma, "cancel", bulkengine.BulkTarget{Scope: "ids", IDs: both}); s.Succeeded != 0 {
		t.Fatalf("fatma bulk = %+v", s)
	}
	// A worker context: emre's membership re-resolved from the job actor
	// (template "member" role: own reservations only).
	orgSvc := orgusecase.New(orgrepository.NewPostgres(f.pool, f.q), branchusecase.NewOrganizationHooks(nil))
	job, err := orgctx.WithJobScope(bg, orgSvc, f.org.ID, f.emre.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := exportUUIDs(t, io, job, nil); len(got) != 2 {
		t.Fatalf("emre job export = %v", got)
	}

	// No slug at all, or no organization: refused.
	if _, err := io.Export(engineScope(f, f.owner.ID, nil), nil, i18n.LocaleTR); err == nil {
		t.Fatal("export without permission")
	}
	if _, err := io.Export(bg, nil, i18n.LocaleTR); err == nil {
		t.Fatal("export without organization")
	}
	if _, err := bulk.ResolveTargets(engineScope(f, f.owner.ID, nil, rbac.PermTenantReservationsRead), "cancel",
		bulkengine.BulkTarget{Scope: "ids", IDs: both}); err == nil {
		t.Fatal("bulk without write")
	}

	// Search documents: the branch and the owners (booker, served by).
	doc, err := adapters.NewSearch(f.q).Document(bg, searchengine.TenantDocumentID(f.org.ID, r1.UUID.String()))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(doc.BranchIDs, []int64{branchID}) || !slices.Contains(doc.OwnerUserIDs, f.owner.ID) ||
		!slices.Contains(doc.OwnerUserIDs, f.emre.ID) || slices.Contains(doc.OwnerUserIDs, f.fatma.ID) {
		t.Fatalf("search document = %+v", doc)
	}
	if !strings.HasPrefix(doc.Title, "#"+strconv.FormatInt(r1.Number, 10)+" · Zeynep Kaya") ||
		!strings.Contains(doc.Subtitle, "07.01.2030 09:00") || !strings.HasSuffix(doc.Href, "/reservations/"+r1.UUID.String()) {
		t.Fatalf("search document = %+v", doc)
	}
	if _, err := adapters.NewSearch(f.q).Document(bg, searchengine.TenantDocumentID(f.org2.ID, r1.UUID.String())); err == nil {
		t.Fatal("another business's document id resolved")
	}
	docs, err := adapters.NewSearch(f.q).ListAll(bg)
	if err != nil {
		t.Fatal(err)
	}
	mine := 0
	for _, d := range docs {
		if d.OrganizationID == f.org.ID {
			mine++
		}
	}
	if mine != 2 {
		t.Fatalf("indexed %d documents of the business, want 2", mine)
	}
}

// TestBulkCancel (TEC-963): a bulk cancel frees the booked times; records
// the action does not apply to, or that changed since they were read, are
// skipped and reported with their numbers.
func TestBulkCancel(t *testing.T) {
	f := newFixture(t)
	bg := context.Background()
	r1, err := f.svc.Create(bg, f.org.ID, f.input(at(9, 0), "", "", f.cut), none, f.owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := f.svc.Create(bg, f.org.ID, f.input(at(10, 0), "", "", f.cut), none, f.owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	ctx := engineScope(f, f.owner.ID, nil, rbac.PermTenantReservationsRead, rbac.PermTenantReservationsWrite)
	bulk := adapters.NewBulk(f.svc)

	if _, err := bulk.ResolveTargets(ctx, "cancel", bulkengine.BulkTarget{Scope: "ids", IDs: []string{r1.UUID.String()},
		Params: map[string]string{"reason": strings.Repeat("x", 33)}}); err == nil {
		t.Fatal("long reason accepted")
	}
	if _, err := bulk.ResolveTargets(ctx, "cancel", bulkengine.BulkTarget{Scope: "ids", IDs: []string{r1.UUID.String()},
		Params: map[string]string{"versions": "nope"}}); err == nil {
		t.Fatal("bad versions accepted")
	}

	// r2 was read at an older version: skipped as stale.
	stale := strconv.Itoa(r2.Version - 1)
	s, err := applyAll(t, bulk, ctx, "cancel", bulkengine.BulkTarget{
		Scope: "ids", IDs: []string{r1.UUID.String(), r2.UUID.String()},
		Params: map[string]string{"reason": "business", "versions": r1.UUID.String() + ":" + strconv.Itoa(r1.Version) + "," + r2.UUID.String() + ":" + stale},
	})
	if err != nil || s.Succeeded != 1 || s.Skipped != 1 || s.Failed != 0 {
		t.Fatalf("bulk cancel = %+v %v", s, err)
	}
	if len(s.Issues) != 1 || s.Issues[0].Error != "stale" || s.Issues[0].Label != "#"+strconv.FormatInt(r2.Number, 10) {
		t.Fatalf("issues = %+v", s.Issues)
	}
	got, err := f.svc.Get(bg, f.org.ID, r1.UUID, none)
	if err != nil || got.Status != model.StatusCancelled || got.CancelReason == nil || *got.CancelReason != "business" {
		t.Fatalf("r1 = %+v %v", got, err)
	}
	if _, status, _ := f.hold(t, got); status == "active" {
		t.Fatal("cancelled booking still holds its time")
	}
	// The freed time can be booked again.
	if _, err := f.svc.Create(bg, f.org.ID, f.input(at(9, 0), "", "", f.cut), none, f.owner.ID); err != nil {
		t.Fatalf("rebook freed time: %v", err)
	}

	// All matching cancelled requests and actives: the cancelled one and
	// accept on an active booking are skipped (invalid transition).
	s, err = applyAll(t, bulk, ctx, "accept", bulkengine.BulkTarget{Scope: "query", Query: map[string]string{"status": "active,cancelled"}})
	if err != nil || s.Total != 3 || s.Succeeded != 0 || s.Skipped != 3 {
		t.Fatalf("bulk accept = %+v %v", s, err)
	}
	for _, issue := range s.Issues {
		if issue.Error != "invalid_transition" || !issue.Skipped {
			t.Fatalf("issue = %+v", issue)
		}
	}
	if _, err := applyAll(t, bulk, ctx, "cancel", bulkengine.BulkTarget{Scope: "query", Query: map[string]string{"status": "bogus"}}); err == nil {
		t.Fatal("invalid filter accepted")
	}
}

func TestTenantOwnFilter(t *testing.T) {
	spec := adapters.NewSearch(nil).Spec()
	filter, ok := searchengine.TenantOwnFilter(spec, 7, []int64{3}, 42)
	if !ok || filter != "organization_id = 7 AND owner_user_ids = 42 AND branch_ids IN [3]" {
		t.Fatalf("own filter = %q %v", filter, ok)
	}
	if _, ok := searchengine.TenantOwnFilter(spec, 7, []int64{}, 42); ok {
		t.Fatal("no branch searched")
	}
	other := spec
	other.OwnPermission = ""
	if _, ok := searchengine.TenantOwnFilter(other, 7, nil, 42); ok {
		t.Fatal("own reach on a spec without an own slug")
	}
}

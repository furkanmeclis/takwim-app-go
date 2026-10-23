package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// fakeRepo mirrors the SQL contract: every lookup is keyed by organization.
type fakeRepo struct {
	nextID   int64
	branches map[int64]*db.Branch
	points   map[int64]*db.ServicePoint
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{branches: map[int64]*db.Branch{}, points: map[int64]*db.ServicePoint{}}
}

func (f *fakeRepo) id() int64 { f.nextID++; return f.nextID }

func (f *fakeRepo) live(orgID int64) []*db.Branch {
	out := []*db.Branch{}
	for _, b := range f.branches {
		if b.OrganizationID == orgID && !b.DeletedAt.Valid {
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (f *fakeRepo) find(orgID int64, id uuid.UUID) (*db.Branch, error) {
	for _, b := range f.live(orgID) {
		if b.Uuid == id {
			return b, nil
		}
	}
	return nil, repository.ErrNotFound
}

func (f *fakeRepo) ListBranches(_ context.Context, orgID int64, filter model.ListFilter) ([]repository.BranchRow, int64, error) {
	rows := []repository.BranchRow{}
	for _, b := range f.live(orgID) {
		if filter.Q != "" && !strings.Contains(strings.ToLower(b.Name+b.Code), strings.ToLower(filter.Q)) {
			continue
		}
		if len(filter.Cities) > 0 && !containsString(filter.Cities, b.City) {
			continue
		}
		if filter.IsActive != nil && b.IsActive != *filter.IsActive {
			continue
		}
		if filter.BranchIDs != nil && !slices.Contains(filter.BranchIDs, b.ID) {
			continue
		}
		rows = append(rows, repository.BranchRow{Branch: *b})
	}
	return rows, int64(len(rows)), nil
}

func (f *fakeRepo) CityFacets(_ context.Context, orgID int64, branchIDs []int64) ([]model.Facet, error) {
	counts := map[string]int64{}
	for _, b := range f.live(orgID) {
		if branchIDs != nil && !slices.Contains(branchIDs, b.ID) {
			continue
		}
		if b.City != "" {
			counts[b.City]++
		}
	}
	out := []model.Facet{}
	for city, n := range counts {
		out = append(out, model.Facet{Value: city, Count: n})
	}
	return out, nil
}

func (f *fakeRepo) ListBranchUUIDs(ctx context.Context, orgID int64, filter model.ListFilter, _ int32) ([]uuid.UUID, error) {
	rows, _, _ := f.ListBranches(ctx, orgID, filter)
	out := []uuid.UUID{}
	for _, r := range rows {
		out = append(out, r.Uuid)
	}
	return out, nil
}

func (f *fakeRepo) ListBranchesForExport(ctx context.Context, orgID int64, filter model.ListFilter) ([]db.Branch, error) {
	rows, _, _ := f.ListBranches(ctx, orgID, filter)
	out := []db.Branch{}
	for _, r := range rows {
		out = append(out, r.Branch)
	}
	return out, nil
}

func (f *fakeRepo) GetBranch(_ context.Context, orgID int64, id uuid.UUID) (db.Branch, error) {
	b, err := f.find(orgID, id)
	if err != nil {
		return db.Branch{}, err
	}
	return *b, nil
}

func (f *fakeRepo) GetBranchByCode(_ context.Context, orgID int64, code string) (db.Branch, error) {
	for _, b := range f.live(orgID) {
		if b.Code == code {
			return *b, nil
		}
	}
	return db.Branch{}, repository.ErrNotFound
}

func (f *fakeRepo) CodeTaken(_ context.Context, orgID int64, code string, exclude *uuid.UUID) (bool, error) {
	for _, b := range f.live(orgID) {
		if b.Code == code && (exclude == nil || b.Uuid != *exclude) {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeRepo) SlugTaken(_ context.Context, orgID int64, slug string, exclude *uuid.UUID) (bool, error) {
	for _, b := range f.live(orgID) {
		if b.Slug == slug && (exclude == nil || b.Uuid != *exclude) {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeRepo) apply(b *db.Branch, fields repository.BranchFields) {
	b.Code, b.Name, b.Address, b.City, b.District = fields.Code, fields.Name, fields.Address, fields.City, fields.District
	b.Latitude, b.Longitude, b.Phone, b.Email = fields.Latitude, fields.Longitude, fields.Phone, fields.Email
	b.WorkingHours, b.HolidayExceptions = fields.WorkingHours, fields.HolidayExceptions
	b.PrepMinutes, b.IsActive = fields.PrepMinutes, fields.IsActive
	b.Slug, b.Description, b.Timezone, b.ShowOnline = fields.Slug, fields.Description, fields.Timezone, fields.ShowOnline
	b.UpdatedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
}

func (f *fakeRepo) CreateBranch(_ context.Context, orgID int64, fields repository.BranchFields) (db.Branch, error) {
	b := &db.Branch{ID: f.id(), Uuid: uuid.New(), OrganizationID: orgID, CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}}
	f.apply(b, fields)
	f.branches[b.ID] = b
	return *b, nil
}

func (f *fakeRepo) UpdateBranch(_ context.Context, orgID int64, id uuid.UUID, fields repository.BranchFields) (db.Branch, error) {
	b, err := f.find(orgID, id)
	if err != nil {
		return db.Branch{}, err
	}
	f.apply(b, fields)
	return *b, nil
}

func (f *fakeRepo) SetBranchActive(_ context.Context, orgID int64, id uuid.UUID, active bool) (db.Branch, error) {
	b, err := f.find(orgID, id)
	if err != nil {
		return db.Branch{}, err
	}
	b.IsActive = active
	return *b, nil
}

func (f *fakeRepo) DeleteBranch(_ context.Context, orgID int64, id uuid.UUID) error {
	b, err := f.find(orgID, id)
	if err != nil {
		return err
	}
	b.DeletedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	return nil
}

func (f *fakeRepo) ListServicePoints(_ context.Context, orgID, branchID int64) ([]db.ServicePoint, error) {
	out := []db.ServicePoint{}
	for _, p := range f.points {
		if p.OrganizationID == orgID && p.BranchID == branchID {
			out = append(out, *p)
		}
	}
	return out, nil
}

func (f *fakeRepo) findPoint(orgID, branchID int64, id uuid.UUID) (*db.ServicePoint, error) {
	for _, p := range f.points {
		if p.OrganizationID == orgID && p.BranchID == branchID && p.Uuid == id {
			return p, nil
		}
	}
	return nil, repository.ErrNotFound
}

func (f *fakeRepo) GetServicePoint(_ context.Context, orgID, branchID int64, id uuid.UUID) (db.ServicePoint, error) {
	p, err := f.findPoint(orgID, branchID, id)
	if err != nil {
		return db.ServicePoint{}, err
	}
	return *p, nil
}

func (f *fakeRepo) CreateServicePoint(_ context.Context, orgID, branchID int64, fields repository.ServicePointFields) (db.ServicePoint, error) {
	p := &db.ServicePoint{ID: f.id(), Uuid: uuid.New(), OrganizationID: orgID, BranchID: branchID}
	p.Type, p.Name, p.Description, p.ExtraFee = fields.Type, fields.Name, fields.Description, fields.ExtraFee
	p.WorkingHours, p.OnlineSelectable, p.IsActive = fields.WorkingHours, fields.OnlineSelectable, fields.IsActive
	f.points[p.ID] = p
	return *p, nil
}

func (f *fakeRepo) UpdateServicePoint(_ context.Context, orgID, branchID int64, id uuid.UUID, fields repository.ServicePointFields) (db.ServicePoint, error) {
	p, err := f.findPoint(orgID, branchID, id)
	if err != nil {
		return db.ServicePoint{}, err
	}
	p.Type, p.Name, p.Description, p.ExtraFee = fields.Type, fields.Name, fields.Description, fields.ExtraFee
	p.WorkingHours, p.OnlineSelectable, p.IsActive = fields.WorkingHours, fields.OnlineSelectable, fields.IsActive
	return *p, nil
}

func (f *fakeRepo) DeleteServicePoint(_ context.Context, orgID, branchID int64, id uuid.UUID) error {
	p, err := f.findPoint(orgID, branchID, id)
	if err != nil {
		return err
	}
	delete(f.points, p.ID)
	return nil
}

func (f *fakeRepo) ListActivity(context.Context, int64, uuid.UUID, int32, int32) ([]db.ListOrganizationResourceActivityRow, int64, error) {
	return nil, 0, nil
}

type fakeIndexer struct{ upserts, deletes []string }

func (i *fakeIndexer) EnqueueUpsert(_ context.Context, spec, id string) {
	i.upserts = append(i.upserts, spec+":"+id)
}

func (i *fakeIndexer) EnqueueDelete(_ context.Context, spec, id string) {
	i.deletes = append(i.deletes, spec+":"+id)
}

func strp(v string) *string { return &v }

const (
	orgA int64 = 1
	orgB int64 = 2
)

func TestOrganizationIsolation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := New(newFakeRepo(), nil)

	a, err := svc.Create(ctx, orgA, model.BranchInput{Code: strp("ist"), Name: strp("İstanbul")})
	if err != nil {
		t.Fatalf("create in A: %v", err)
	}
	if a.Code != "IST" {
		t.Fatalf("code not normalized: %q", a.Code)
	}

	if _, err := svc.Get(ctx, orgB, a.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("org B get A branch: err = %v, want ErrNotFound", err)
	}
	if _, err := svc.Patch(ctx, orgB, a.UUID, model.BranchInput{Name: strp("hijack")}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("org B patch A branch: err = %v, want ErrNotFound", err)
	}
	if err := svc.Delete(ctx, orgB, a.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("org B delete A branch: err = %v, want ErrNotFound", err)
	}
	if _, err := svc.SetActive(ctx, orgB, a.UUID, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("org B deactivate A branch: err = %v, want ErrNotFound", err)
	}
	if _, err := svc.ListServicePoints(ctx, orgB, a.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("org B list A service points: err = %v, want ErrNotFound", err)
	}
	if _, err := svc.CreateServicePoint(ctx, orgB, a.UUID, model.ServicePointInput{Type: strp("office"), Name: strp("x")}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("org B add point to A branch: err = %v, want ErrNotFound", err)
	}
	if items, total, err := svc.List(ctx, orgB, model.ListFilter{Limit: 20}); err != nil || total != 0 || len(items) != 0 {
		t.Fatalf("org B list = %d items (total %d), err %v; want empty", len(items), total, err)
	}

	got, err := svc.Get(ctx, orgA, a.UUID)
	if err != nil || got.Name != "İstanbul" {
		t.Fatalf("org A get = %+v, %v", got, err)
	}
}

func TestCodeUniquePerOrganization(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := New(newFakeRepo(), nil)

	if _, err := svc.Create(ctx, orgA, model.BranchInput{Code: strp("MRK"), Name: strp("Merkez")}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, orgA, model.BranchInput{Code: strp(" mrk "), Name: strp("Copy")}); !errors.Is(err, ErrCodeTaken) {
		t.Fatalf("duplicate code in same org: err = %v, want ErrCodeTaken", err)
	}
	if _, err := svc.Create(ctx, orgB, model.BranchInput{Code: strp("MRK"), Name: strp("Merkez B")}); err != nil {
		t.Fatalf("same code in another org must be allowed: %v", err)
	}
	other, err := svc.Create(ctx, orgA, model.BranchInput{Code: strp("ANK"), Name: strp("Ankara")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Patch(ctx, orgA, other.UUID, model.BranchInput{Code: strp("mrk")}); !errors.Is(err, ErrCodeTaken) {
		t.Fatalf("patch to taken code: err = %v, want ErrCodeTaken", err)
	}
	if _, err := svc.Patch(ctx, orgA, other.UUID, model.BranchInput{Code: strp("ANK"), Name: strp("Ankara Merkez")}); err != nil {
		t.Fatalf("patch keeping own code: %v", err)
	}
	// A deleted branch frees its code.
	if err := svc.Delete(ctx, orgA, other.UUID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, orgA, model.BranchInput{Code: strp("ANK"), Name: strp("Ankara 2")}); err != nil {
		t.Fatalf("code of a deleted branch must be reusable: %v", err)
	}
}

func TestServicePointBelongsToBranch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := New(newFakeRepo(), nil)

	b1, _ := svc.Create(ctx, orgA, model.BranchInput{Code: strp("B1"), Name: strp("One")})
	b2, _ := svc.Create(ctx, orgA, model.BranchInput{Code: strp("B2"), Name: strp("Two")})
	fee := json.Number("150.5")
	p, err := svc.CreateServicePoint(ctx, orgA, b1.UUID, model.ServicePointInput{
		Type: strp("airport"), Name: strp("SAW"), ExtraFee: &fee,
	})
	if err != nil {
		t.Fatalf("create point: %v", err)
	}
	if p.ExtraFee != "150.50" || p.BranchUUID != b1.UUID || p.WorkingHours != nil || !p.OnlineSelectable {
		t.Fatalf("unexpected point: %+v", p)
	}

	if _, err := svc.PatchServicePoint(ctx, orgA, b2.UUID, p.UUID, model.ServicePointInput{Name: strp("moved")}); !errors.Is(err, ErrServicePointNotFound) {
		t.Fatalf("patch point through another branch: err = %v, want ErrServicePointNotFound", err)
	}
	if err := svc.DeleteServicePoint(ctx, orgA, b2.UUID, p.UUID); !errors.Is(err, ErrServicePointNotFound) {
		t.Fatalf("delete point through another branch: err = %v, want ErrServicePointNotFound", err)
	}
	if err := svc.DeleteServicePoint(ctx, orgB, b1.UUID, p.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete point from another org: err = %v, want ErrNotFound", err)
	}
	if points, _ := svc.ListServicePoints(ctx, orgA, b2.UUID); len(points) != 0 {
		t.Fatalf("branch two must have no points, got %d", len(points))
	}

	hours := model.DefaultWeeklyHours()
	updated, err := svc.PatchServicePoint(ctx, orgA, b1.UUID, p.UUID, model.ServicePointInput{
		WorkingHours: model.OptionalHours{Set: true, Value: &hours},
	})
	if err != nil || updated.WorkingHours == nil || updated.Name != "SAW" {
		t.Fatalf("patch own hours = %+v, %v", updated, err)
	}
	cleared, err := svc.PatchServicePoint(ctx, orgA, b1.UUID, p.UUID, model.ServicePointInput{
		WorkingHours: model.OptionalHours{Set: true},
	})
	if err != nil || cleared.WorkingHours != nil {
		t.Fatalf("clearing hours must fall back to branch hours: %+v, %v", cleared, err)
	}
	got, _ := svc.Get(ctx, orgA, b1.UUID)
	if got.ServicePointsCount != 1 {
		t.Fatalf("service point count = %d, want 1", got.ServicePointsCount)
	}
}

func TestValidation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := New(newFakeRepo(), nil)

	bad := model.DefaultWeeklyHours()
	bad.Mon = model.DayHours{Open: "18:00", Close: "09:00"}
	bad.Tue = model.DayHours{Open: "9am", Close: "17:00"}
	lat := 120.0
	holidays := []model.HolidayException{{Date: "2026-13-01", Closed: true}, {Date: "2026-01-01", Closed: true}, {Date: "2026-01-01", Closed: true}}
	email := "not-an-email"
	_, err := svc.Create(ctx, orgA, model.BranchInput{
		Code: strp("bad code!"), WorkingHours: &bad, HolidayExceptions: &holidays,
		Latitude: model.OptionalFloat{Set: true, Value: &lat}, Email: &email,
	})
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v, want ValidationError", err)
	}
	fields := map[string]bool{}
	for _, d := range ve.Details {
		fields[d.Field] = true
	}
	for _, want := range []string{
		"code", "name", "email", "latitude", "working_hours.mon.close", "working_hours.tue.open",
		"holiday_exceptions.0.date", "holiday_exceptions.2.date",
	} {
		if !fields[want] {
			t.Errorf("missing validation detail for %s (got %v)", want, fields)
		}
	}

	b, _ := svc.Create(ctx, orgA, model.BranchInput{Code: strp("OK"), Name: strp("Ok")})
	if b.WorkingHours != model.DefaultWeeklyHours() || b.PrepMinutes != 60 || !b.IsActive {
		t.Fatalf("defaults not applied: %+v", b)
	}
	neg := json.Number("-1")
	if _, err := svc.CreateServicePoint(ctx, orgA, b.UUID, model.ServicePointInput{Type: strp("ship"), Name: strp("x"), ExtraFee: &neg}); !errors.As(err, &ve) || len(ve.Details) != 2 {
		t.Fatalf("service point validation = %v", err)
	}
}

func TestSearchIndexing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	idx := &fakeIndexer{}
	svc := New(newFakeRepo(), idx)
	b, _ := svc.Create(ctx, orgA, model.BranchInput{Code: strp("S1"), Name: strp("Search")})
	if err := svc.Delete(ctx, orgA, b.UUID); err != nil {
		t.Fatal(err)
	}
	want := SearchSpec + ":1_" + b.UUID.String()
	if len(idx.upserts) != 1 || idx.upserts[0] != want || len(idx.deletes) != 1 || idx.deletes[0] != want {
		t.Fatalf("index calls = %v / %v, want %s", idx.upserts, idx.deletes, want)
	}
}

func TestImportRowCreatesThenUpdatesByCode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := New(newFakeRepo(), nil)
	id, created, prev, err := svc.ImportRow(ctx, orgA, model.BranchInput{Code: strp("imp"), Name: strp("Import")})
	if err != nil || !created || prev != nil {
		t.Fatalf("first import = %v %v %v", created, prev, err)
	}
	id2, created, prev, err := svc.ImportRow(ctx, orgA, model.BranchInput{Code: strp("IMP"), Name: strp("Import 2")})
	if err != nil || created || prev == nil || prev.Name != "Import" || id2 != id {
		t.Fatalf("second import = %v %v %v", created, prev, err)
	}
	// Same code in another organization creates a separate branch.
	id3, created, _, err := svc.ImportRow(ctx, orgB, model.BranchInput{Code: strp("IMP"), Name: strp("Other")})
	if err != nil || !created || id3 == id {
		t.Fatalf("import in org B = %v %v", created, err)
	}
}

func TestFormatMoney(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{"0": "0.00", "150.5": "150.50", "0.05": "0.05", "1234567.89": "1234567.89", "12": "12.00"} {
		var n pgtype.Numeric
		if err := n.Scan(in); err != nil {
			t.Fatal(err)
		}
		if got := FormatMoney(n); got != want {
			t.Errorf("FormatMoney(%s) = %s, want %s", in, got, want)
		}
	}
}

func containsString(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

package orgctx

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestBranchScopeHelpers(t *testing.T) {
	t.Parallel()
	bg := context.Background()
	if BranchScope(bg) != nil || CheckBranch(bg, 7) != nil || RequireAllBranches(bg) != nil {
		t.Fatal("a context without scope must not be restricted")
	}

	all := WithScope(bg, Scope{ID: 1, Slug: "acme"})
	if BranchScope(all) != nil || CheckBranch(all, 7) != nil || RequireAllBranches(all) != nil {
		t.Fatal("branch scope 'all' must not be restricted")
	}

	selected := WithScope(bg, Scope{ID: 1, Slug: "acme", BranchIDs: []int64{3}})
	if got := BranchScope(selected); len(got) != 1 || got[0] != 3 {
		t.Fatalf("BranchScope = %v", got)
	}
	if err := CheckBranch(selected, 3); err != nil {
		t.Fatalf("own branch: %v", err)
	}
	if err := CheckBranch(selected, 4); !errors.Is(err, ErrBranchNotAllowed) {
		t.Fatalf("other branch err = %v", err)
	}
	if err := RequireAllBranches(selected); !errors.Is(err, ErrAllBranchesRequired) {
		t.Fatalf("RequireAllBranches err = %v", err)
	}

	none := WithScope(bg, Scope{ID: 1, Slug: "acme", BranchIDs: []int64{}})
	if got := BranchScope(none); got == nil || len(got) != 0 {
		t.Fatalf("selected scope without branches must stay non-nil and empty, got %v", got)
	}
	if err := CheckBranch(none, 3); !errors.Is(err, ErrBranchNotAllowed) {
		t.Fatalf("no branches err = %v", err)
	}
}

// The Redis cache stores scopes as JSON: nil (every branch) and empty
// (no branch) must survive the round trip.
func TestScopeJSONKeepsBranchScope(t *testing.T) {
	t.Parallel()
	for _, in := range []Scope{
		{ID: 1, UUID: uuid.New(), Slug: "a"},
		{ID: 1, UUID: uuid.New(), Slug: "a", BranchIDs: []int64{}},
		{ID: 1, UUID: uuid.New(), Slug: "a", BranchIDs: []int64{5, 9}, MemberRoles: []string{"resepsiyon", "member"}},
	} {
		raw, err := json.Marshal(in)
		if err != nil {
			t.Fatal(err)
		}
		var out Scope
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
		if (in.BranchIDs == nil) != (out.BranchIDs == nil) || len(in.BranchIDs) != len(out.BranchIDs) {
			t.Fatalf("branch ids %v -> %v", in.BranchIDs, out.BranchIDs)
		}
		if len(out.MemberRoles) != len(in.MemberRoles) {
			t.Fatalf("roles %v -> %v", in.MemberRoles, out.MemberRoles)
		}
	}
}

type stubJobLoader struct {
	calls int
	scope Scope
}

func (s *stubJobLoader) ResolveMember(_ context.Context, orgID, userID int64) (Scope, error) {
	s.calls++
	if userID != 42 {
		return Scope{}, ErrNotMember
	}
	sc := s.scope
	sc.ID = orgID
	return sc, nil
}

func TestWithJobScope(t *testing.T) {
	t.Parallel()
	loader := &stubJobLoader{scope: Scope{Slug: "acme", BranchIDs: []int64{3}}}

	// Worker: the actor's membership is re-resolved.
	ctx, err := WithJobScope(context.Background(), loader, 9, 42)
	if err != nil {
		t.Fatal(err)
	}
	if s := Must(ctx); s.ID != 9 || len(s.BranchIDs) != 1 || s.BranchIDs[0] != 3 {
		t.Fatalf("job scope = %+v", s)
	}
	// The actor left the organization: the job fails.
	if _, err := WithJobScope(context.Background(), loader, 9, 7); !errors.Is(err, ErrNotMember) {
		t.Fatalf("former member err = %v", err)
	}
	// Synchronous processing keeps the request scope.
	req := WithScope(context.Background(), Scope{ID: 9, Slug: "acme", BranchIDs: []int64{4}})
	calls := loader.calls
	ctx, err = WithJobScope(req, loader, 9, 42)
	if err != nil || loader.calls != calls || Must(ctx).BranchIDs[0] != 4 {
		t.Fatalf("request scope not kept: %v %+v", err, Must(ctx))
	}
	// No loader: organization only.
	ctx, err = WithJobScope(context.Background(), nil, 9, 42)
	if err != nil || Must(ctx).ID != 9 || Must(ctx).BranchIDs != nil {
		t.Fatalf("nil loader scope = %+v %v", Must(ctx), err)
	}
}

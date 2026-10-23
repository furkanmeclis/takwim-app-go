package orgctx

// Reach is how far a member's permission over a kind of record goes.
type Reach int

const (
	// ReachNone: no access.
	ReachNone Reach = iota
	// ReachOwn: only the member's own records (own reservations, own
	// schedule, own earnings).
	ReachOwn
	// ReachAll: every record inside the member's branch scope.
	ReachAll
)

// Reach resolves a full / own permission pair (for example
// tenant.earnings.read and tenant.earnings.own). The full slug wins; the
// branch scope (BranchIDs) still limits ReachAll. Modules call it before
// they query, and filter by the member's staff record on ReachOwn, so a
// staff member never reads another member's records (requirements §4).
func (s Scope) Reach(full, own string) Reach {
	switch {
	case full != "" && s.Has(full):
		return ReachAll
	case own != "" && s.Has(own):
		return ReachOwn
	}
	return ReachNone
}

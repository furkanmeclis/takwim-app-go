package orgctx

import "testing"

func TestReach(t *testing.T) {
	t.Parallel()
	const full, own = "tenant.earnings.read", "tenant.earnings.own"
	cases := []struct {
		perms []string
		want  Reach
	}{
		{nil, ReachNone},
		{[]string{own}, ReachOwn},
		{[]string{full}, ReachAll},
		{[]string{own, full}, ReachAll},
		{[]string{"tenant.reservations.own"}, ReachNone},
	}
	for _, c := range cases {
		if got := (Scope{Permissions: c.perms}).Reach(full, own); got != c.want {
			t.Errorf("Reach(%v) = %v, want %v", c.perms, got, c.want)
		}
	}
	if got := (Scope{Permissions: []string{own}}).Reach(full, ""); got != ReachNone {
		t.Errorf("no own slug: %v", got)
	}
}

package model

import (
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

func TestDistribute(t *testing.T) {
	m := money.MustParse
	cases := []struct {
		name  string
		price string
		lists []string
		want  []string
	}{
		// A16: 800 + 400 sold at 1000.
		{"proportional", "1000", []string{"800", "400"}, []string{"666.67", "333.33"}},
		{"remainder on the last line", "100", []string{"2", "1"}, []string{"66.67", "33.33"}},
		{"three lines remainder last", "100", []string{"1", "1", "1"}, []string{"33.33", "33.33", "33.34"}},
		{"free services share equally", "50", []string{"0", "0"}, []string{"25.00", "25.00"}},
		{"one free line takes nothing", "90", []string{"0", "300"}, []string{"0.00", "90.00"}},
		{"bundle above list", "1500", []string{"800", "400"}, []string{"1000.00", "500.00"}},
	}
	for _, c := range cases {
		lists := make([]money.Money, len(c.lists))
		for i, l := range c.lists {
			lists[i] = m(l)
		}
		got := Distribute(m(c.price), lists)
		sum := money.Zero()
		for i, g := range got {
			sum = sum.Add(g)
			if !g.Equal(m(c.want[i])) {
				t.Errorf("%s: part %d = %s, want %s", c.name, i, g, c.want[i])
			}
		}
		if !sum.Equal(m(c.price)) {
			t.Errorf("%s: sum = %s, want %s", c.name, sum, c.price)
		}
	}
}

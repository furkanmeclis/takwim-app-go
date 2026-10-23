package phone

import "testing"

func TestNormalize(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"0532 123 45 67", "+905321234567", true},
		{"532 123 45 67", "+905321234567", true},
		{"+90 (532) 123-45-67", "+905321234567", true},
		{"00905321234567", "+905321234567", true},
		{"+49 30 901820", "+4930901820", true},
		{"", "", true},
		{"12", "", false},
		{"abc", "", false},
	}
	for _, c := range cases {
		got, err := Normalize(c.in, "")
		if (err == nil) != c.ok || got != c.want {
			t.Errorf("Normalize(%q) = %q, %v; want %q ok=%v", c.in, got, err, c.want, c.ok)
		}
	}
}

func TestDisplay(t *testing.T) {
	t.Parallel()
	if got := Display("+905321234567", ""); got != "0532 123 45 67" {
		t.Errorf("Display TR = %q", got)
	}
	if got := Display("+4930901820", ""); got != "+49 30 901820" {
		t.Errorf("Display DE = %q", got)
	}
}

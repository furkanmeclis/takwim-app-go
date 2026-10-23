package slug

import "testing"

func TestFromNameFoldsTurkish(t *testing.T) {
	cases := map[string]string{
		"İdes Car Rental":         "ides-car-rental",
		"Güneş Oto Kiralama":      "gunes-oto-kiralama",
		"ÇAĞDAŞ IŞIK Ölçü Şirket": "cagdas-isik-olcu-sirket",
		"  --  ":                  "item",
		"Acme, Inc.":              "acme-inc",
	}
	for in, want := range cases {
		if got := FromName(in); got != want {
			t.Errorf("FromName(%q) = %q, want %q", in, got, want)
		}
	}
}

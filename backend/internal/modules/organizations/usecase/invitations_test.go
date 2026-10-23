package usecase

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestInvitationTokenAndHash(t *testing.T) {
	a, err := NewInvitationToken()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := NewInvitationToken()
	if len(a) != 43 || a == b {
		t.Fatalf("tokens %q %q: want 32 random bytes in base64url", a, b)
	}
	sum := sha256.Sum256([]byte(a))
	if got := HashInvitationToken(a); got != hex.EncodeToString(sum[:]) || len(got) != 64 {
		t.Fatalf("hash = %q", got)
	}
	if HashInvitationToken(" "+a+"\n") != HashInvitationToken(a) {
		t.Fatal("hash must ignore surrounding whitespace from copied links")
	}
}

func TestMaskEmail(t *testing.T) {
	cases := map[string]string{
		"ayse.kaya@example.com": "a********@e******.com",
		"a@b.co":                "*@*.co",
		"ömer@şirket.com.tr":    "ö***@ş*********.tr",
		"broken":                "***",
	}
	for in, want := range cases {
		if got := MaskEmail(in); got != want {
			t.Errorf("MaskEmail(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeEmail(t *testing.T) {
	if v, ok := normalizeEmail("  Ayse@Example.COM "); !ok || v != "ayse@example.com" {
		t.Fatalf("normalize = %q %v", v, ok)
	}
	for _, bad := range []string{"", "no-at", "Ayşe <a@b.co>", "a@b.co, c@d.co"} {
		if _, ok := normalizeEmail(bad); ok {
			t.Errorf("%q must be rejected", bad)
		}
	}
}

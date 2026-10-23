package crypto

import (
	"strings"
	"testing"
)

func TestSealerPurposeSeparation(t *testing.T) {
	c, err := NewFieldCipher(strings.Repeat("k", 32))
	if err != nil {
		t.Fatal(err)
	}
	creds, err := c.Sealer("integration.credentials")
	if err != nil {
		t.Fatal(err)
	}
	other, err := c.Sealer("other.purpose")
	if err != nil {
		t.Fatal(err)
	}
	ct, err := creds.Seal(`{"secret_key":"s"}`, "acct|org:1")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := creds.Open(ct, "acct|org:1"); err != nil || got != `{"secret_key":"s"}` {
		t.Fatalf("open: %q %v", got, err)
	}
	if _, err := creds.Open(ct, "acct|org:2"); err == nil {
		t.Fatal("another owner's associated data opened the value")
	}
	if _, err := other.Open(ct, "acct|org:1"); err == nil {
		t.Fatal("another purpose opened the value")
	}
	if _, err := c.Decrypt(ct, "acct|org:1"); err == nil {
		t.Fatal("the personal data key opened a purpose value")
	}
	if _, err := c.Sealer("bad/purpose"); err == nil {
		t.Fatal("invalid purpose accepted")
	}
	// Rotation: a value sealed under v1 still opens after v2 becomes current.
	c2, err := NewFieldCipherVersion(strings.Repeat("n", 32), 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := c2.AddKey(1, strings.Repeat("k", 32)); err != nil {
		t.Fatal(err)
	}
	creds2, err := c2.Sealer("integration.credentials")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := creds2.Open(ct, "acct|org:1"); err != nil || got != `{"secret_key":"s"}` {
		t.Fatalf("rotated open: %q %v", got, err)
	}
	if ct2, _ := creds2.Seal("x", "a"); !strings.HasPrefix(ct2, "v2:") {
		t.Fatalf("new value not on the current version: %s", ct2)
	}
}

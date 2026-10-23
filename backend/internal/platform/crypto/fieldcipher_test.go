package crypto

import (
	"strings"
	"testing"
)

func TestFieldCipherRoundTripAndAAD(t *testing.T) {
	c, err := NewFieldCipher(testKey(t))
	if err != nil {
		t.Fatal(err)
	}
	enc, err := c.Encrypt("10000000146", "customers.identity|org:1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(enc, "v1:") || strings.Contains(enc, "10000000146") {
		t.Fatalf("ciphertext %q", enc)
	}
	if KeyVersion(enc) != 1 {
		t.Fatalf("version = %d", KeyVersion(enc))
	}
	got, err := c.Decrypt(enc, "customers.identity|org:1")
	if err != nil || got != "10000000146" {
		t.Fatalf("decrypt = %q, %v", got, err)
	}
	// Another column or organization does not open it.
	if _, err := c.Decrypt(enc, "customers.identity|org:2"); err == nil {
		t.Fatal("ciphertext opened with another organization's aad")
	}
	if _, err := c.Decrypt(enc, "drivers.licence|org:1"); err == nil {
		t.Fatal("ciphertext opened with another column's aad")
	}
	if e, _ := c.Encrypt("", "x"); e != "" {
		t.Fatal("empty plaintext must stay empty")
	}
}

func TestFieldCipherKeyRotation(t *testing.T) {
	oldKey := testKey(t)
	old, _ := NewFieldCipher(oldKey)
	enc, _ := old.Encrypt("secret", "a")
	newKey := strings.Repeat("n", 32)
	c, err := NewFieldCipherVersion(newKey, 2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Decrypt(enc, "a"); err != ErrUnknownKeyVersion {
		t.Fatalf("unknown version err = %v", err)
	}
	if err := c.AddKey(1, oldKey); err != nil {
		t.Fatal(err)
	}
	if got, err := c.Decrypt(enc, "a"); err != nil || got != "secret" {
		t.Fatalf("old version decrypt = %q, %v", got, err)
	}
	enc2, _ := c.Encrypt("secret", "a")
	if KeyVersion(enc2) != 2 {
		t.Fatalf("new ciphertext version = %d", KeyVersion(enc2))
	}
}

func TestFieldCipherBlindIndex(t *testing.T) {
	c, _ := NewFieldCipher(testKey(t))
	a := c.BlindIndex("customer.identity", "org:1", "tckn:10000000146")
	if a == "" || a != c.BlindIndex("customer.identity", "org:1", "tckn:10000000146") {
		t.Fatal("blind index must be deterministic")
	}
	if a == c.BlindIndex("customer.identity", "org:2", "tckn:10000000146") {
		t.Fatal("blind index must differ per organization")
	}
	if a == c.BlindIndex("driver.identity", "org:1", "tckn:10000000146") {
		t.Fatal("blind index must differ per purpose")
	}
	if strings.Contains(a, "10000000146") || len(a) != 64 {
		t.Fatalf("blind index %q", a)
	}
	if c.BlindIndex("p", "s", "") != "" {
		t.Fatal("empty value has no index")
	}
	other, _ := NewFieldCipher(strings.Repeat("z", 32))
	if other.BlindIndex("customer.identity", "org:1", "tckn:10000000146") == a {
		t.Fatal("blind index must depend on the key")
	}
}

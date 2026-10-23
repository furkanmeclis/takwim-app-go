package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

// FieldCipher encrypts personal data columns (column-level encryption) and
// computes keyed blind indexes for exact-match lookups on them.
//
// Keys are derived from APP_ENCRYPTION_KEY with HKDF-SHA256 and a purpose
// label, so the column keys, the blind index keys and the SecretBox key of
// the integrations never coincide:
//
//   - encryption key version N: HKDF(master_N, info "pii/enc/v<N>")
//   - blind index key of purpose P: HKDF(master_current, info "pii/bidx/<P>")
//
// Ciphertexts are "v<N>:" + base64(nonce | AES-256-GCM sealed). The
// associated data names the column and the owner (for example
// "customers.identity|org:42"), so a ciphertext copied into another column
// or another organization's row does not decrypt.
//
// Rotation: NewFieldCipher takes the current key; AddKey registers older
// versions that still decrypt. Re-encrypting rows with the current version
// is a background job (not built yet); blind indexes are bound to the
// current master key and must be recomputed when it changes.
type FieldCipher struct {
	current  int
	aeads    map[int]cipher.AEAD
	master   []byte
	masters  map[int][]byte
	mu       sync.Mutex
	bidxKeys map[string][]byte
}

// ErrUnknownKeyVersion is returned for a ciphertext of an unregistered key.
var ErrUnknownKeyVersion = errors.New("fieldcipher: unknown key version")

// NewFieldCipher builds a cipher whose current key version is 1.
func NewFieldCipher(keyMaterial string) (*FieldCipher, error) {
	return NewFieldCipherVersion(keyMaterial, 1)
}

// NewFieldCipherVersion builds a cipher with the given current version.
func NewFieldCipherVersion(keyMaterial string, version int) (*FieldCipher, error) {
	if version < 1 {
		return nil, fmt.Errorf("fieldcipher: version must be >= 1")
	}
	master, err := decodeKey(keyMaterial)
	if err != nil {
		return nil, err
	}
	c := &FieldCipher{
		current: version, aeads: map[int]cipher.AEAD{}, master: master, masters: map[int][]byte{},
		bidxKeys: map[string][]byte{},
	}
	if err := c.addKey(version, master); err != nil {
		return nil, err
	}
	return c, nil
}

// AddKey registers an older key version that can still decrypt.
func (c *FieldCipher) AddKey(version int, keyMaterial string) error {
	if version < 1 || version == c.current {
		return fmt.Errorf("fieldcipher: invalid previous version %d", version)
	}
	master, err := decodeKey(keyMaterial)
	if err != nil {
		return err
	}
	return c.addKey(version, master)
}

func (c *FieldCipher) addKey(version int, master []byte) error {
	gcm, err := deriveAEAD(master, "pii/enc/v"+strconv.Itoa(version))
	if err != nil {
		return err
	}
	c.aeads[version] = gcm
	c.masters[version] = master
	return nil
}

func deriveAEAD(master []byte, info string) (cipher.AEAD, error) {
	key, err := hkdf.Key(sha256.New, master, nil, info, 32)
	if err != nil {
		return nil, fmt.Errorf("fieldcipher: derive: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("fieldcipher: cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("fieldcipher: gcm: %w", err)
	}
	return gcm, nil
}

// CurrentVersion is the key version new ciphertexts use.
func (c *FieldCipher) CurrentVersion() int { return c.current }

// Encrypt seals plaintext for the column / owner named by aad. An empty
// plaintext stays empty (no value).
func (c *FieldCipher) Encrypt(plaintext, aad string) (string, error) {
	return seal(c.aeads, c.current, plaintext, aad)
}

func seal(aeads map[int]cipher.AEAD, current int, plaintext, aad string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	gcm := aeads[current]
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("fieldcipher: nonce: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), []byte(aad))
	return "v" + strconv.Itoa(current) + ":" + base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt opens a ciphertext of Encrypt with the same aad.
func (c *FieldCipher) Decrypt(encoded, aad string) (string, error) {
	return open(c.aeads, encoded, aad)
}

func open(aeads map[int]cipher.AEAD, encoded, aad string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	head, body, ok := strings.Cut(encoded, ":")
	if !ok || !strings.HasPrefix(head, "v") {
		return "", ErrInvalidInput
	}
	version, err := strconv.Atoi(head[1:])
	if err != nil {
		return "", ErrInvalidInput
	}
	gcm, ok := aeads[version]
	if !ok {
		return "", ErrUnknownKeyVersion
	}
	sealed, err := base64.StdEncoding.DecodeString(body)
	if err != nil || len(sealed) < gcm.NonceSize() {
		return "", ErrInvalidInput
	}
	nonce, ciphertext := sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, []byte(aad))
	if err != nil {
		return "", ErrInvalidInput
	}
	return string(plain), nil
}

// KeyVersion returns the key version of a ciphertext (0 when it is not one).
func KeyVersion(encoded string) int {
	head, _, ok := strings.Cut(encoded, ":")
	if !ok || !strings.HasPrefix(head, "v") {
		return 0
	}
	v, err := strconv.Atoi(head[1:])
	if err != nil {
		return 0
	}
	return v
}

// BlindIndex is the keyed hash (HMAC-SHA256, hex) of a normalized value for
// exact-match lookups and uniqueness. purpose separates the key per kind of
// value ("customer.identity", "driver.licence"); scope (the organization)
// is part of the input, so the same number in two organizations gets
// unrelated indexes and rows cannot be linked across organizations. An
// empty value has no index ("").
func (c *FieldCipher) BlindIndex(purpose, scope, normalized string) string {
	if normalized == "" {
		return ""
	}
	c.mu.Lock()
	key, ok := c.bidxKeys[purpose]
	if !ok {
		derived, err := hkdf.Key(sha256.New, c.master, nil, "pii/bidx/"+purpose, 32)
		if err != nil {
			// sha256 with a 32-byte output cannot fail.
			panic(err)
		}
		key = derived
		c.bidxKeys[purpose] = key
	}
	c.mu.Unlock()
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(scope))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(normalized))
	return hex.EncodeToString(mac.Sum(nil))
}

// Sealer encrypts the values of one purpose (for example the credentials of
// integration accounts) with keys of their own: HKDF(master_N, info
// "purpose/<name>/enc/v<N>"), separate from the personal data column keys,
// the blind index keys and every other purpose. The ciphertext format and
// key rotation are the FieldCipher's; the associated data still binds a
// value to its row and owner.
type Sealer struct {
	purpose string
	current int
	aeads   map[int]cipher.AEAD
}

// Sealer returns the purpose-separated cipher for name (every registered
// key version decrypts; new values use the current version).
func (c *FieldCipher) Sealer(purpose string) (*Sealer, error) {
	if strings.TrimSpace(purpose) == "" || strings.ContainsAny(purpose, "/ ") {
		return nil, fmt.Errorf("fieldcipher: invalid purpose %q", purpose)
	}
	s := &Sealer{purpose: purpose, current: c.current, aeads: map[int]cipher.AEAD{}}
	for v, master := range c.masters {
		gcm, err := deriveAEAD(master, "purpose/"+purpose+"/enc/v"+strconv.Itoa(v))
		if err != nil {
			return nil, err
		}
		s.aeads[v] = gcm
	}
	return s, nil
}

// Purpose names the sealer's purpose.
func (s *Sealer) Purpose() string { return s.purpose }

// Seal encrypts plaintext bound to aad ("" stays "").
func (s *Sealer) Seal(plaintext, aad string) (string, error) {
	return seal(s.aeads, s.current, plaintext, aad)
}

// Open decrypts a Seal ciphertext with the same aad.
func (s *Sealer) Open(encoded, aad string) (string, error) {
	return open(s.aeads, encoded, aad)
}

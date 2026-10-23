package jwt

import (
	"errors"
	"testing"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func TestAccessTokenKind(t *testing.T) {
	t.Parallel()
	m, err := NewManager("secret", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	user := uuid.New()
	tok, _, err := m.IssueAccess(AccessInput{UserID: user})
	if err != nil {
		t.Fatal(err)
	}
	claims, err := m.ParseAccess(tok)
	if err != nil || claims.Kind != KindStaff || claims.Subject != user.String() {
		t.Fatalf("staff token = %+v %v", claims, err)
	}

	sign := func(kind string) string {
		c := Claims{Kind: kind, RegisteredClaims: jwtlib.RegisteredClaims{
			Subject:   user.String(),
			ExpiresAt: jwtlib.NewNumericDate(time.Now().Add(time.Minute)),
		}}
		s, err := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, c).SignedString([]byte("secret"))
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	if c, err := m.ParseAccess(sign("")); err != nil || c.Kind != KindStaff {
		t.Fatalf("legacy token = %+v %v", c, err)
	}
	if _, err := m.ParseAccess(sign("customer")); !errors.Is(err, ErrWrongKind) {
		t.Fatalf("customer token err = %v", err)
	}
}

func TestCustomerTokenKind(t *testing.T) {
	t.Parallel()
	m, err := NewManager("secret", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	identity, session := uuid.New(), uuid.New()
	tok, err := m.IssueCustomer(CustomerInput{IdentityID: identity, SessionID: session, ExpiresAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	claims, err := m.ParseCustomer(tok)
	if err != nil || claims.Kind != KindCustomer || claims.Subject != identity.String() || claims.SessionUUID() != session {
		t.Fatalf("customer token = %+v %v", claims, err)
	}
	// A customer token never reaches a staff route, and a staff token never
	// opens the customer panel.
	if _, err := m.ParseAccess(tok); !errors.Is(err, ErrWrongKind) {
		t.Fatalf("staff parse of a customer token = %v", err)
	}
	staff, _, err := m.IssueAccess(AccessInput{UserID: identity, SessionID: session})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.ParseCustomer(staff); !errors.Is(err, ErrWrongKind) {
		t.Fatalf("customer parse of a staff token = %v", err)
	}
	if _, err := m.IssueCustomer(CustomerInput{IdentityID: identity, SessionID: session, ExpiresAt: time.Now().Add(-time.Minute)}); err == nil {
		t.Fatal("expired customer token issued")
	}
	other, _ := NewManager("other", time.Minute, time.Hour)
	if _, err := other.ParseCustomer(tok); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("foreign signature = %v", err)
	}
}

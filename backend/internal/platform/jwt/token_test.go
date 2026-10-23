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

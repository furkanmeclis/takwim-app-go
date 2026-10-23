// Package money is the one money type of the backend (TEC-878).
//
// Representation: an int64 count of minor units (kuruş for TRY) plus a
// currency code. Every amount the product stores is a NUMERIC(14,2) column:
// at most 12 integer digits and 2 fraction digits, i.e. |x| < 10^14 kuruş,
// which fits an int64 (9.2 * 10^18) with a wide margin. Integer minor units
// keep addition, comparison and equality exact without a dependency on a
// decimal library; multiplication by a rate (VAT, percentage rules,
// prepayment) goes through math/big so the intermediate product cannot
// overflow, and is then rounded once.
//
// Rounding rule (the only one): half away from zero to the minor unit
// ("half-up" for the positive amounts prices are). 0.005 TRY → 0.01 TRY,
// -0.005 TRY → -0.01 TRY. Every operation that can produce a fraction of a
// kuruş (MulRate, Ratio, Parse with more than two decimals via ParseRound,
// FromNumeric of a wider NUMERIC) applies it exactly once.
//
// JSON: a decimal string with two fraction digits ("1300.00"), never a
// float. Unmarshal also accepts a JSON number or a string with a comma
// decimal separator.
//
// Postgres: FromNumeric / Numeric convert to and from pgtype.Numeric (sqlc
// columns), and Money implements pgtype.NumericScanner / NumericValuer so it
// can be passed to pgx directly.
package money

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

// Currency is an ISO 4217 code. TRY is the default; the field exists so
// amounts never silently change meaning when a second currency arrives.
type Currency string

// TRY is the default currency.
const TRY Currency = "TRY"

// MaxMinor bounds amounts to NUMERIC(14,2): |minor| < 10^14.
const MaxMinor int64 = 100_000_000_000_000 - 1

// minorPerUnit is the number of minor units in one currency unit (2 decimals).
const minorPerUnit = 100

var (
	// ErrInvalid is returned for a string that is not a decimal amount.
	ErrInvalid = errors.New("money: invalid amount")
	// ErrPrecision is returned by Parse for more than two decimals.
	ErrPrecision = errors.New("money: at most 2 decimals are allowed")
	// ErrRange is returned for an amount outside NUMERIC(14,2).
	ErrRange = errors.New("money: amount out of range")
)

// Money is an amount in minor units of Currency. The zero value is 0 TRY
// (an empty currency reads as TRY).
type Money struct {
	minor    int64
	currency Currency
}

// New returns minor units of TRY (New(130000) = 1300.00 TRY).
func New(minor int64) Money { return Money{minor: minor, currency: TRY} }

// NewIn returns minor units of the given currency.
func NewIn(minor int64, c Currency) Money { return Money{minor: minor, currency: c} }

// Zero is 0 TRY.
func Zero() Money { return New(0) }

// FromUnits returns whole currency units (FromUnits(1300) = 1300.00 TRY).
func FromUnits(units int64) Money { return New(units * minorPerUnit) }

// Minor returns the amount in minor units.
func (m Money) Minor() int64 { return m.minor }

// Currency returns the currency (TRY when unset).
func (m Money) Currency() Currency {
	if m.currency == "" {
		return TRY
	}
	return m.currency
}

// IsZero, IsNegative and IsPositive compare with zero.
func (m Money) IsZero() bool     { return m.minor == 0 }
func (m Money) IsNegative() bool { return m.minor < 0 }
func (m Money) IsPositive() bool { return m.minor > 0 }

func (m Money) same(o Money) {
	if m.Currency() != o.Currency() {
		// Mixing currencies is a programming error, not an input error:
		// every amount of one calculation comes from one organization.
		panic(fmt.Sprintf("money: currency mismatch %s vs %s", m.Currency(), o.Currency()))
	}
}

// Add returns m + o (same currency; panics otherwise).
func (m Money) Add(o Money) Money {
	m.same(o)
	return Money{minor: m.minor + o.minor, currency: m.Currency()}
}

// Sub returns m - o (same currency; panics otherwise).
func (m Money) Sub(o Money) Money {
	m.same(o)
	return Money{minor: m.minor - o.minor, currency: m.Currency()}
}

// Neg returns -m.
func (m Money) Neg() Money { return Money{minor: -m.minor, currency: m.Currency()} }

// Mul returns m * n (an exact integer multiple, e.g. days or quantity).
func (m Money) Mul(n int64) Money { return Money{minor: m.minor * n, currency: m.Currency()} }

// Cmp returns -1, 0 or 1 (same currency; panics otherwise).
func (m Money) Cmp(o Money) int {
	m.same(o)
	switch {
	case m.minor < o.minor:
		return -1
	case m.minor > o.minor:
		return 1
	}
	return 0
}

// Equal reports whether both amount and currency match.
func (m Money) Equal(o Money) bool { return m.minor == o.minor && m.Currency() == o.Currency() }

// ClampZero returns m, or 0 when m is negative ("never negative" results).
func (m Money) ClampZero() Money {
	if m.minor < 0 {
		return Money{currency: m.Currency()}
	}
	return m
}

// Min returns the smaller amount.
func (m Money) Min(o Money) Money {
	if m.Cmp(o) <= 0 {
		return m
	}
	return o
}

// Ratio returns m * num / den, rounded half away from zero to the minor
// unit. den must not be zero.
func (m Money) Ratio(num, den int64) Money {
	if den == 0 {
		panic("money: zero denominator")
	}
	p := new(big.Int).Mul(big.NewInt(m.minor), big.NewInt(num))
	return Money{minor: divRound(p, big.NewInt(den)), currency: m.Currency()}
}

// MulRate returns m * rate, rate being a Rate (basis points, 1/10000), rounded
// half away from zero. MulRate(Percent(20)) of 1300.00 is 260.00.
func (m Money) MulRate(r Rate) Money { return m.Ratio(int64(r), RateScale) }

// divRound divides p by d rounding half away from zero.
func divRound(p, d *big.Int) int64 {
	neg := (p.Sign() < 0) != (d.Sign() < 0)
	pa := new(big.Int).Abs(p)
	da := new(big.Int).Abs(d)
	q, r := new(big.Int).QuoRem(pa, da, new(big.Int))
	if new(big.Int).Mul(r, big.NewInt(2)).Cmp(da) >= 0 {
		q.Add(q, big.NewInt(1))
	}
	if neg {
		q.Neg(q)
	}
	return q.Int64()
}

// String renders "1300.00" (no currency, no grouping): the API format.
func (m Money) String() string {
	v := m.minor
	sign := ""
	if v < 0 {
		sign = "-"
		v = -v
	}
	return fmt.Sprintf("%s%d.%02d", sign, v/minorPerUnit, v%minorPerUnit)
}

// Parse reads a decimal amount: "1300", "1300.5", "1300.50", "-20",
// "1300,50" (comma decimal separator). Thousands separators are not
// accepted (ambiguous between tr and en). More than two decimals is
// ErrPrecision; use ParseRound to round instead.
func Parse(s string) (Money, error) {
	return parse(s, false)
}

// ParseRound is Parse that rounds extra decimals half away from zero.
func ParseRound(s string) (Money, error) {
	return parse(s, true)
}

func parse(s string, round bool) (Money, error) {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", "."))
	if s == "" {
		return Money{}, ErrInvalid
	}
	neg := false
	switch s[0] {
	case '-':
		neg = true
		s = s[1:]
	case '+':
		s = s[1:]
	}
	intPart, frac, hasDot := strings.Cut(s, ".")
	if intPart == "" && frac == "" || strings.Contains(frac, ".") || hasDot && frac == "" && intPart == "" {
		return Money{}, ErrInvalid
	}
	if intPart == "" {
		intPart = "0"
	}
	if !digits(intPart) || frac != "" && !digits(frac) {
		return Money{}, ErrInvalid
	}
	intPart = strings.TrimLeft(intPart, "0")
	if intPart == "" {
		intPart = "0"
	}
	if len(intPart) > 12 {
		return Money{}, ErrRange
	}
	roundUp := false
	if len(frac) > 2 {
		if !round {
			return Money{}, ErrPrecision
		}
		roundUp = frac[2] >= '5'
		frac = frac[:2]
	}
	for len(frac) < 2 {
		frac += "0"
	}
	units, _ := strconv.ParseInt(intPart, 10, 64)
	cents, _ := strconv.ParseInt(frac, 10, 64)
	minor := units*minorPerUnit + cents
	if roundUp {
		minor++
	}
	if minor > MaxMinor {
		return Money{}, ErrRange
	}
	if neg {
		minor = -minor
	}
	return New(minor), nil
}

func digits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return s != ""
}

// MustParse is Parse that panics (tests and constants).
func MustParse(s string) Money {
	m, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return m
}

// MarshalJSON renders the amount as a JSON string ("1300.00").
func (m Money) MarshalJSON() ([]byte, error) {
	return []byte(`"` + m.String() + `"`), nil
}

// UnmarshalJSON accepts "1300.00", "1300,5" or 1300.5 (at most 2 decimals).
func (m *Money) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if bytes.Equal(b, []byte("null")) {
		*m = Money{}
		return nil
	}
	var s string
	if len(b) > 0 && b[0] == '"' {
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
	} else {
		s = string(b)
	}
	v, err := Parse(s)
	if err != nil {
		return err
	}
	*m = v
	return nil
}

// FromNumeric converts a pgtype.Numeric (NUMERIC column) to TRY, rounding
// extra fraction digits half away from zero. A NULL numeric is ErrInvalid.
func FromNumeric(n pgtype.Numeric) (Money, error) {
	if !n.Valid || n.Int == nil {
		return Money{}, ErrInvalid
	}
	if n.NaN || n.InfinityModifier != pgtype.Finite {
		return Money{}, ErrInvalid
	}
	v := new(big.Int).Set(n.Int)
	shift := int64(n.Exp) + 2
	var minor int64
	if shift >= 0 {
		v.Mul(v, new(big.Int).Exp(big.NewInt(10), big.NewInt(shift), nil))
		if !v.IsInt64() {
			return Money{}, ErrRange
		}
		minor = v.Int64()
	} else {
		minor = divRound(v, new(big.Int).Exp(big.NewInt(10), big.NewInt(-shift), nil))
	}
	if minor > MaxMinor || minor < -MaxMinor {
		return Money{}, ErrRange
	}
	return New(minor), nil
}

// FromNumericOrZero is FromNumeric that reads NULL / invalid as 0.
func FromNumericOrZero(n pgtype.Numeric) Money {
	m, err := FromNumeric(n)
	if err != nil {
		return Zero()
	}
	return m
}

// Numeric converts to a pgtype.Numeric with exponent -2 (NUMERIC(14,2)).
func (m Money) Numeric() pgtype.Numeric {
	return pgtype.Numeric{Int: big.NewInt(m.minor), Exp: -2, Valid: true}
}

// ScanNumeric implements pgtype.NumericScanner.
func (m *Money) ScanNumeric(n pgtype.Numeric) error {
	v, err := FromNumeric(n)
	if err != nil {
		return err
	}
	*m = v
	return nil
}

// NumericValue implements pgtype.NumericValuer.
func (m Money) NumericValue() (pgtype.Numeric, error) {
	return m.Numeric(), nil
}

// FormatNumeric renders a NUMERIC column as "1300.00" ("0.00" for NULL).
func FormatNumeric(n pgtype.Numeric) string {
	return FromNumericOrZero(n).String()
}

// --------------------------------------------------------------------- rates

// Rate is a ratio in basis points (1/10000): Percent(20) = 2000. Two
// decimals of a percentage are representable exactly ("18.25" %).
type Rate int64

// RateScale is the denominator of a Rate.
const RateScale = 10_000

// Percent returns a whole percentage as a Rate.
func Percent(p int64) Rate { return Rate(p * 100) }

// ParseRate reads a percentage with at most two decimals ("20", "18.25",
// "-10", "7,5").
func ParseRate(s string) (Rate, error) {
	m, err := Parse(s) // same grammar: two decimals
	if err != nil {
		return 0, err
	}
	return Rate(m.minor), nil
}

// String renders the percentage ("20.00").
func (r Rate) String() string { return New(int64(r)).String() }

// MarshalJSON renders the percentage as a string ("20.00").
func (r Rate) MarshalJSON() ([]byte, error) { return []byte(`"` + r.String() + `"`), nil }

// UnmarshalJSON accepts "20", "18.25" or 20.
func (r *Rate) UnmarshalJSON(b []byte) error {
	var m Money
	if err := m.UnmarshalJSON(b); err != nil {
		return err
	}
	*r = Rate(m.minor)
	return nil
}

// RateFromNumeric converts a NUMERIC percentage column to a Rate.
func RateFromNumeric(n pgtype.Numeric) (Rate, error) {
	m, err := FromNumeric(n)
	return Rate(m.minor), err
}

// Numeric converts the percentage to a NUMERIC with exponent -2.
func (r Rate) Numeric() pgtype.Numeric { return New(int64(r)).Numeric() }

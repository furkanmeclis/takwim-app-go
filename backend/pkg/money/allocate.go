package money

import (
	"errors"
	"math/big"
	"regexp"
)

// ErrAllocate is returned for an empty or non-positive weight list.
var ErrAllocate = errors.New("money: weights must be non-negative with a positive sum")

// ErrCurrency is returned for a code that is not three upper-case letters.
var ErrCurrency = errors.New("money: currency must be a 3-letter ISO 4217 code")

var currencyRe = regexp.MustCompile(`^[A-Z]{3}$`)

// ParseCurrency validates an ISO 4217 code ("TRY", "EUR").
func ParseCurrency(s string) (Currency, error) {
	if !currencyRe.MatchString(s) {
		return "", ErrCurrency
	}
	return Currency(s), nil
}

// Allocate splits m over weights in proportion. Every part is rounded down
// toward zero and the rounding remainder goes to the last part, so the
// parts always add up to m exactly ("son satıra fark verme").
func (m Money) Allocate(weights ...int64) ([]Money, error) {
	if len(weights) == 0 {
		return nil, ErrAllocate
	}
	var total int64
	for _, w := range weights {
		if w < 0 {
			return nil, ErrAllocate
		}
		total += w
	}
	if total <= 0 {
		return nil, ErrAllocate
	}
	c := m.Currency()
	out := make([]Money, len(weights))
	var used int64
	for i, w := range weights[:len(weights)-1] {
		part := truncRatio(m.minor, w, total)
		out[i] = Money{minor: part, currency: c}
		used += part
	}
	out[len(weights)-1] = Money{minor: m.minor - used, currency: c}
	return out, nil
}

// Split divides m into n equal parts; the remainder goes to the last part.
func (m Money) Split(n int) ([]Money, error) {
	if n <= 0 {
		return nil, ErrAllocate
	}
	w := make([]int64, n)
	for i := range w {
		w[i] = 1
	}
	return m.Allocate(w...)
}

// truncRatio returns minor * num / den rounded toward zero; the product
// goes through math/big so it cannot overflow.
func truncRatio(minor, num, den int64) int64 {
	p := new(big.Int).Mul(big.NewInt(minor), big.NewInt(num))
	return p.Quo(p, big.NewInt(den)).Int64()
}

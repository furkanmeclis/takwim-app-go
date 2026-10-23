// Package phone normalizes phone numbers to E.164 ("+905321234567"), the
// one form the API stores and compares. A number without a country code is
// read in the default region (Turkey unless the caller says otherwise).
package phone

import (
	"errors"
	"strings"

	"github.com/nyaruka/phonenumbers"
)

// DefaultRegion is the region of numbers written without a country code.
const DefaultRegion = "TR"

// ErrInvalid is returned for input that is not a valid phone number.
var ErrInvalid = errors.New("invalid phone number")

// Normalize returns the E.164 form of raw. Empty input returns "" and no
// error; region "" means DefaultRegion.
func Normalize(raw, region string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if region == "" {
		region = DefaultRegion
	}
	// "00" international prefix written without "+".
	if strings.HasPrefix(raw, "00") {
		raw = "+" + raw[2:]
	}
	num, err := phonenumbers.Parse(raw, strings.ToUpper(region))
	if err != nil || !phonenumbers.IsValidNumber(num) {
		return "", ErrInvalid
	}
	return phonenumbers.Format(num, phonenumbers.E164), nil
}

// Display formats an E.164 number for people: national format in the
// default region ("0532 123 45 67"), international otherwise.
func Display(e164, region string) string {
	if e164 == "" {
		return ""
	}
	if region == "" {
		region = DefaultRegion
	}
	num, err := phonenumbers.Parse(e164, strings.ToUpper(region))
	if err != nil {
		return e164
	}
	if phonenumbers.GetRegionCodeForNumber(num) == strings.ToUpper(region) {
		return phonenumbers.Format(num, phonenumbers.NATIONAL)
	}
	return phonenumbers.Format(num, phonenumbers.INTERNATIONAL)
}

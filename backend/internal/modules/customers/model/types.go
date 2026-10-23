// Package model holds the API shapes of the customers tenant resource.
package model

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Slug is the stable slug of the customers resource (activity, io, bulk).
const Slug = "tenant.customers"

// Sources lists where a customer record came from.
var Sources = []string{"panel", "import", "online"}

// LinkSources lists how a phone was proven before a record was linked to
// the shared identity.
var LinkSources = []string{"otp", "panel_login"}

// Verification tells that the record is linked to a person who proved the
// phone. The shared identity itself never leaves the platform.
type Verification struct {
	Source     string    `json:"source"`
	VerifiedAt time.Time `json:"verified_at"`
}

// Customer is the business-level customer record.
type Customer struct {
	UUID             uuid.UUID     `json:"uuid"`
	FirstName        string        `json:"first_name"`
	LastName         string        `json:"last_name"`
	FullName         string        `json:"full_name"`
	Phone            *string       `json:"phone"`
	PhoneDisplay     string        `json:"phone_display"`
	Email            string        `json:"email"`
	BirthDate        *string       `json:"birth_date"`
	Notes            string        `json:"notes"`
	Tags             []string      `json:"tags"`
	SMSConsent       bool          `json:"sms_consent"`
	EmailConsent     bool          `json:"email_consent"`
	ConsentUpdatedAt *time.Time    `json:"consent_updated_at"`
	Source           string        `json:"source"`
	Verification     *Verification `json:"verification"`
	CreatedAt        time.Time     `json:"created_at"`
	UpdatedAt        time.Time     `json:"updated_at"`
}

// OptionalString distinguishes an absent field from null (null clears).
type OptionalString struct {
	Set   bool
	Value *string
}

func (o *OptionalString) UnmarshalJSON(b []byte) error {
	o.Set = true
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		o.Value = nil
		return nil
	}
	var v string
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}

// Input is the create / patch body. Absent fields keep their value on patch
// and take their defaults on create; phone and birth_date take null to clear.
type Input struct {
	FirstName    *string        `json:"first_name"`
	LastName     *string        `json:"last_name"`
	Phone        OptionalString `json:"phone"`
	Email        *string        `json:"email"`
	BirthDate    OptionalString `json:"birth_date"`
	Notes        *string        `json:"notes"`
	Tags         *[]string      `json:"tags"`
	SMSConsent   *bool          `json:"sms_consent"`
	EmailConsent *bool          `json:"email_consent"`
}

// ListFilter selects customers of one organization.
type ListFilter struct {
	Limit        int32
	Offset       int32
	Q            string
	Tags         []string
	Sources      []string
	SMSConsent   *bool
	EmailConsent *bool
	Verified     *bool
	SortField    string
	SortDesc     bool
}

// Facet is one option of a faceted filter with its row count.
type Facet struct {
	Value string `json:"value"`
	Label string `json:"label,omitempty"`
	Count int64  `json:"count"`
}

// Facets are the dynamic filter options of the list.
type Facets struct {
	Tag    []Facet `json:"tag"`
	Source []Facet `json:"source"`
}

// ActivityActor is the user behind an activity entry.
type ActivityActor struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   string    `json:"email"`
}

// ActivityEntry is one audit event of a customer.
type ActivityEntry struct {
	UUID      uuid.UUID       `json:"uuid"`
	Action    string          `json:"action"`
	Payload   json.RawMessage `json:"payload"`
	Actor     *ActivityActor  `json:"actor"`
	CreatedAt time.Time       `json:"created_at"`
}

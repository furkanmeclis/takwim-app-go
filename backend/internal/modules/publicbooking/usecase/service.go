// Package usecase is the public booking API of Takwim (TEC-906): the
// business page, the widget and the mobile app resolve a business by its
// slug, read its public catalog, search slots on the online channel, hold
// the picked slot during checkout and book it for a customer identified by
// phone. Availability, holds and reservations are the panel's own use
// cases (channel online), so a slot means the same thing everywhere.
package usecase

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availabilitymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	customersusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/usecase"
	orgsettings "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	paymentsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/model"
	paymentsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicbooking/model"
	reservationsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	reservationsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sector"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/phone"
)

// Field limits of the contact form.
const (
	maxNameLength  = 80
	maxEmailLength = 254
)

// accessTokenPurpose separates the reservation access tokens from every
// other keyed hash of the field cipher.
const accessTokenPurpose = "public.reservation_access"

var (
	// ErrBusinessNotFound: no live business with the slug, or it is
	// suspended or outside its access window (the same answer for all, so
	// the route does not tell which businesses exist).
	ErrBusinessNotFound = errors.New("business not found")
	// ErrCheckoutNotFound: no checkout of the business with the token.
	ErrCheckoutNotFound = errors.New("checkout not found")
	// ErrReservationNotFound: unknown reservation, another business's, or
	// a missing / wrong access token.
	ErrReservationNotFound = errors.New("reservation not found")
	// ErrLogoNotFound: the business has no logo.
	ErrLogoNotFound = errors.New("logo not found")
	// ErrCoverNotFound: the business has no cover image.
	ErrCoverNotFound = errors.New("cover not found")
)

// UnavailableError: the business does not take public bookings on the
// channel (Reason is a model.Unavailable* code).
type UnavailableError struct {
	Reason string
}

func (e *UnavailableError) Error() string {
	return "online booking is not available: " + e.Reason
}

// Detail is one field-level validation problem.
type Detail struct {
	Field   string
	Message string
	Code    string
}

// ValidationError carries field details; handlers answer 400.
type ValidationError struct {
	Details []Detail
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Details) == 0 {
		return "validation failed"
	}
	return e.Details[0].Field + ": " + e.Details[0].Message
}

func (e *ValidationError) add(field, code, msg string) {
	e.Details = append(e.Details, Detail{Field: field, Code: code, Message: msg})
}

func (e *ValidationError) orNil() error {
	if len(e.Details) == 0 {
		return nil
	}
	return e
}

func invalid(field, code, msg string) error {
	return &ValidationError{Details: []Detail{{Field: field, Code: code, Message: msg}}}
}

// SettingsReader returns the current business settings.
type SettingsReader interface {
	Current(ctx context.Context, orgID int64) (orgsettings.Version, error)
}

// TokenSigner computes keyed hashes (crypto.FieldCipher.BlindIndex).
type TokenSigner interface {
	BlindIndex(purpose, scope, normalized string) string
}

// SearchIndexer schedules Cmd+K index updates of new customers.
type SearchIndexer interface {
	EnqueueUpsert(ctx context.Context, spec, id string)
}

// Payments is the online payment layer (TEC-912): whether the business
// takes online payments and the start of one.
type Payments interface {
	OnlineReady(ctx context.Context, orgID int64) bool
	StartOnline(ctx context.Context, in paymentsusecase.StartInput) (paymentsmodel.OnlineStart, error)
}

// Service implements the public booking API.
type Service struct {
	payments     Payments
	q            *db.Queries
	settings     SettingsReader
	availability *availabilityusecase.Service
	holds        *availabilityusecase.Holds
	reservations *reservationsusecase.Service
	signer       TokenSigner
	indexer      SearchIndexer
	classes      ClassSessions
	now          func() time.Time
}

// New builds the service.
func New(q *db.Queries, settings SettingsReader, availability *availabilityusecase.Service, holds *availabilityusecase.Holds, reservations *reservationsusecase.Service, signer TokenSigner) *Service {
	return &Service{
		q: q, settings: settings, availability: availability, holds: holds,
		reservations: reservations, signer: signer, now: time.Now,
	}
}

// SetIndexer indexes customers created by bookings (optional).
func (s *Service) SetIndexer(ix SearchIndexer) {
	s.indexer = ix
}

// SetPayments plugs in the online payment layer (nil: online payment is
// never offered as ready).
func (s *Service) SetPayments(p Payments) {
	s.payments = p
}

// SetClock replaces time.Now (tests).
func (s *Service) SetClock(now func() time.Time) {
	s.now = now
}

// Org is a business resolved from its slug.
type Org struct {
	ID             int64
	UUID           uuid.UUID
	Slug           string
	Name           string
	Sector         string
	Timezone       string
	Currency       string
	LogoObjectKey  string
	CoverObjectKey string
	modules        entitlement.Effective
}

// Scope is a minimal organization scope for activity and live events.
func (o Org) Scope() orgctx.Scope {
	return orgctx.Scope{ID: o.ID, UUID: o.UUID, Slug: o.Slug, Name: o.Name, Status: orgctx.StatusActive, Sector: o.Sector, Timezone: o.Timezone, Currency: o.Currency}
}

// Resolve loads a live business by slug. Suspended businesses and those
// outside their access window answer ErrBusinessNotFound.
func (s *Service) Resolve(ctx context.Context, slug string) (Org, error) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if slug == "" || len(slug) > 64 {
		return Org{}, ErrBusinessNotFound
	}
	row, err := s.q.GetPublicOrganizationBySlug(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return Org{}, ErrBusinessNotFound
	}
	if err != nil {
		return Org{}, err
	}
	now := s.now()
	if row.Status != orgctx.StatusActive ||
		(row.AccessStartsAt.Valid && row.AccessStartsAt.Time.After(now)) ||
		(row.AccessEndsAt.Valid && !row.AccessEndsAt.Time.After(now)) {
		return Org{}, ErrBusinessNotFound
	}
	st := entitlement.Stored{Plan: row.Plan, Addons: row.Addons}
	if row.BranchLimit.Valid {
		st.BranchLimit = &row.BranchLimit.Int32
	}
	if row.StaffLimit.Valid {
		st.StaffLimit = &row.StaffLimit.Int32
	}
	return Org{
		ID: row.ID, UUID: row.Uuid, Slug: row.Slug, Name: row.Name, Sector: row.Sector,
		Timezone: row.Timezone, Currency: row.Currency, LogoObjectKey: row.LogoObjectKey.String,
		CoverObjectKey: row.CoverObjectKey.String,
		modules:        entitlement.Resolve(row.Sector, st),
	}, nil
}

// LogoURL is the public logo stream of the business (versioned by key).
func LogoURL(o Org) *string {
	return mediaURL(o.Slug, "logo", o.LogoObjectKey)
}

// CoverURL is the public cover image stream of the business page
// (TEC-966), versioned by key.
func CoverURL(o Org) *string {
	return mediaURL(o.Slug, "cover", o.CoverObjectKey)
}

// mediaURL builds /v1/public/businesses/{slug}/{name}?v= with a version
// derived from the object key (FNV-1a), nil without a key.
func mediaURL(slug, name, key string) *string {
	if key == "" {
		return nil
	}
	h := uint32(2166136261)
	for i := 0; i < len(key); i++ {
		h ^= uint32(key[i])
		h *= 16777619
	}
	u := fmt.Sprintf("/v1/public/businesses/%s/%s?v=%x", slug, name, h)
	return &u
}

// Logo returns the object key of the business logo.
func (s *Service) Logo(o Org) (string, error) {
	if o.LogoObjectKey == "" {
		return "", ErrLogoNotFound
	}
	return o.LogoObjectKey, nil
}

// Cover returns the object key of the business page cover. Only a key under
// the business's own storage prefix is served.
func (s *Service) Cover(o Org) (string, error) {
	if o.CoverObjectKey == "" || !storage.OrganizationOwnsKey(o.CoverObjectKey, o.UUID.String()) {
		return "", ErrCoverNotFound
	}
	return o.CoverObjectKey, nil
}

// booking computes whether the business takes public bookings.
func (s *Service) booking(ctx context.Context, o Org) (model.Booking, error) {
	v, err := s.settings.Current(ctx, o.ID)
	if err != nil {
		return model.Booking{}, err
	}
	set := v.Settings
	out := model.Booking{
		Channels: map[string]bool{}, Unavailable: []string{}, ApprovalMode: set.Reservation.ApprovalMode,
		MinLeadMinutes: set.Reservation.MinLeadMinutes, MaxAdvanceDays: set.Reservation.MaxAdvanceDays,
		CancelCutoffMinutes: set.Reservation.CancelCutoffMinutes, ChangeCutoffMinutes: set.Reservation.ChangeCutoffMinutes,
		PaymentMethods: []string{}, CheckoutMinutes: availabilityusecase.DefaultCheckoutMinutes,
		SlotStepMinutes: set.Reservation.SlotStepMinutes, MaxOpenPerCustomer: set.Reservation.MaxOpenPerCustomer,
		HiddenSteps: append([]string{}, set.Booking.HiddenSteps...), Fields: []model.Field{},
	}
	for _, f := range set.Booking.Fields {
		if f.Online {
			out.Fields = append(out.Fields, model.Field{
				Key: f.Key, Label: f.Label, Help: f.Help, Type: f.Type, Options: append([]string{}, f.Options...), Required: f.Required,
			})
		}
	}
	for _, c := range model.Channels {
		out.Channels[c] = false
	}
	for _, c := range set.SalesChannels {
		if _, ok := out.Channels[c.Code]; ok {
			out.Channels[c.Code] = c.IsActive
		}
	}
	if set.Payment.PayAtVenue {
		out.PaymentMethods = append(out.PaymentMethods, reservationsmodel.MethodOnSite)
	}
	if set.Payment.PayOnline {
		out.PaymentMethods = append(out.PaymentMethods, reservationsmodel.MethodOnline)
		out.OnlinePaymentReady = s.onlineReady(ctx, o)
	}
	if !o.modules.Allows(sector.ModuleReservations) || !o.modules.Allows(sector.ModuleBookingPage) {
		out.Unavailable = append(out.Unavailable, model.UnavailableModule)
	}
	if !v.ReadyForOnlineSales {
		out.Unavailable = append(out.Unavailable, model.UnavailableNotReady)
	}
	out.Ready = len(out.Unavailable) == 0
	return out, nil
}

// gate refuses bookings when the business does not take them on channel.
func (s *Service) gate(ctx context.Context, o Org, channel string) error {
	b, err := s.booking(ctx, o)
	if err != nil {
		return err
	}
	if len(b.Unavailable) > 0 {
		return &UnavailableError{Reason: b.Unavailable[0]}
	}
	if !b.Channels[channel] {
		return &UnavailableError{Reason: model.UnavailableChannel}
	}
	return nil
}

// channelOf validates the channel of a request (empty = web).
func channelOf(raw string) (string, error) {
	if raw == "" {
		return model.ChannelWeb, nil
	}
	if !slices.Contains(model.Channels, raw) {
		return "", invalid("channel", "invalid", "channel must be web, widget or api")
	}
	return raw, nil
}

// Business is the profile and page context.
func (s *Service) Business(ctx context.Context, o Org) (model.Business, error) {
	b, err := s.booking(ctx, o)
	if err != nil {
		return model.Business{}, err
	}
	v, err := s.settings.Current(ctx, o.ID)
	if err != nil {
		return model.Business{}, err
	}
	branches, err := s.q.ListPublicBranches(ctx, o.ID)
	if err != nil {
		return model.Business{}, err
	}
	def := sector.Of(o.Sector)
	flow := make([]string, 0, len(def.Flow))
	for _, st := range def.Flow {
		if !v.Settings.Booking.Hides(string(st)) {
			flow = append(flow, string(st))
		}
	}
	c := v.Settings.Company
	p := v.Settings.Policies
	return model.Business{
		UUID: o.UUID, Slug: o.Slug, Name: o.Name, LogoURL: LogoURL(o), Sector: o.Sector,
		Timezone: o.Timezone, Currency: o.Currency, Flow: flow,
		Terms:       map[string]string{"staff": def.Terms.Staff, "resource": def.Terms.Resource, "booking": def.Terms.Booking},
		Contact:     model.Contact{Phone: c.Phone, Email: c.Email, Website: c.Website, Address: c.Address},
		Policies:    model.Policies{Cancellation: p.Cancellation, Change: p.Change, NoShow: p.NoShow},
		Booking:     b,
		BranchCount: len(branches),
		Embed:       model.Embed{AllowedOrigins: append([]string{}, v.Settings.Embed.AllowedOrigins...)},
		Theme:       theme(o, v.Settings.Theme),
	}, nil
}

// theme is the customer-facing theme of the page (TEC-966). A version saved
// before the theme decodes with an empty font: the page's default.
func theme(o Org, t orgsettings.Theme) model.Theme {
	font := t.Font
	if font == "" {
		font = orgsettings.FontDefault
	}
	return model.Theme{
		PrimaryColor: t.PrimaryColor, AccentColor: t.AccentColor, Font: font,
		Headline: t.Headline, Description: t.Description, CoverURL: CoverURL(o),
	}
}

// Branches lists the branches shown on the customer screens.
func (s *Service) Branches(ctx context.Context, o Org) ([]model.Branch, error) {
	rows, err := s.q.ListPublicBranches(ctx, o.ID)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	images, err := s.q.ListPublicBranchImages(ctx, db.ListPublicBranchImagesParams{OrganizationID: o.ID, BranchIds: ids})
	if err != nil {
		return nil, err
	}
	zones, err := s.q.ListPublicBranchZones(ctx, db.ListPublicBranchZonesParams{OrganizationID: o.ID, BranchIds: ids})
	if err != nil {
		return nil, err
	}
	zonesOf := map[int64][]string{}
	for _, z := range zones {
		zonesOf[z.BranchID] = append(zonesOf[z.BranchID], z.Zone)
	}
	byBranch := map[int64][]model.Image{}
	for _, img := range images {
		byBranch[img.BranchID] = append(byBranch[img.BranchID], model.Image{
			UUID: img.Uuid, URL: "/v1/public/media/branches/" + o.UUID.String() + "/images/" + img.Uuid.String(),
		})
	}
	out := make([]model.Branch, 0, len(rows))
	for _, r := range rows {
		b := model.Branch{
			UUID: r.Uuid, Slug: r.Slug, Name: r.Name, Description: r.Description, Address: r.Address,
			City: r.City, District: r.District, Latitude: float(r.Latitude), Longitude: float(r.Longitude),
			Phone: r.Phone, Email: r.Email, Timezone: r.Timezone,
			WorkingHours: map[string]model.Hours{}, SpecialDays: []model.SpecialDay{}, Images: byBranch[r.ID],
			Zones: zonesOf[r.ID],
		}
		if b.Zones == nil {
			b.Zones = []string{}
		}
		if b.Images == nil {
			b.Images = []model.Image{}
		}
		if err := json.Unmarshal(r.WorkingHours, &b.WorkingHours); err != nil {
			return nil, fmt.Errorf("branch hours: %w", err)
		}
		var days []model.SpecialDay
		if err := json.Unmarshal(r.HolidayExceptions, &days); err != nil {
			return nil, fmt.Errorf("branch special days: %w", err)
		}
		loc, err := time.LoadLocation(r.Timezone)
		if err != nil {
			loc = time.UTC
		}
		today := s.now().In(loc).Format(time.DateOnly)
		for _, d := range days {
			if d.Date >= today {
				b.SpecialDays = append(b.SpecialDays, d)
			}
		}
		slices.SortFunc(b.SpecialDays, func(a, c model.SpecialDay) int { return strings.Compare(a.Date, c.Date) })
		out = append(out, b)
	}
	return out, nil
}

func float(n pgtype.Numeric) *float64 {
	if !n.Valid {
		return nil
	}
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return nil
	}
	v := f.Float64
	return &v
}

// Products lists what the customer may add to a booking in a public
// branch (TEC-919): active products shown online, sold there, with units
// available (on hand - reserved). Without the products module the list
// is empty.
func (s *Service) Products(ctx context.Context, o Org, branchUUID *uuid.UUID) ([]model.Product, error) {
	id := uuid.Nil
	if branchUUID != nil {
		id = *branchUUID
	}
	branchID, err := s.publicBranchID(ctx, o, id, "branch_uuid")
	if err != nil {
		return nil, err
	}
	out := []model.Product{}
	if !o.modules.Allows(sector.ModuleProducts) {
		return out, nil
	}
	if hidden, err := s.productsHidden(ctx, o); err != nil || hidden {
		return out, err
	}
	items, err := reservationsusecase.ListProductOptions(ctx, s.q, o.ID, branchID, "", true, 100)
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		p := model.Product{
			ProductUUID: it.ProductUUID, Name: it.ProductName, VariantUUID: it.VariantUUID, VariantName: it.VariantName,
			Price: it.Price, MaxQuantity: int(min(it.Available, model.MaxProductQuantity)),
		}
		if it.ImageUUID != nil {
			u := fmt.Sprintf("/v1/public/media/products/%s/images/%s", o.UUID, *it.ImageUUID)
			p.ImageURL = &u
		}
		out = append(out, p)
	}
	return out, nil
}

// publicBranchID returns the id of a public branch, or a validation error
// on field (hidden, inactive, deleted and other businesses' branches).
func (s *Service) publicBranchID(ctx context.Context, o Org, id uuid.UUID, field string) (int64, error) {
	if id == uuid.Nil {
		return 0, invalid(field, "required", "branch is required")
	}
	rows, err := s.q.ListPublicBranches(ctx, o.ID)
	if err != nil {
		return 0, err
	}
	for _, r := range rows {
		if r.Uuid == id {
			return r.ID, nil
		}
	}
	return 0, invalid(field, "not_found", "branch not found")
}

// catalog is the public catalog of a business, keyed by id.
type catalog struct {
	branches    map[int64]uuid.UUID
	branchIDs   map[uuid.UUID]int64
	services    []db.ListPublicServicesRow
	serviceUUID map[int64]uuid.UUID
	staff       []db.ListPublicStaffRow
	staffUUID   map[int64]uuid.UUID
	offers      map[int64][]db.ListPublicServiceOffersRow
	performers  map[int64][]db.ListPublicServiceStaffRow
	staffSvcs   map[int64][]int64
	staffBranch map[int64][]int64
}

func (s *Service) loadCatalog(ctx context.Context, o Org) (*catalog, error) {
	c := &catalog{
		branches: map[int64]uuid.UUID{}, branchIDs: map[uuid.UUID]int64{}, serviceUUID: map[int64]uuid.UUID{},
		staffUUID: map[int64]uuid.UUID{}, offers: map[int64][]db.ListPublicServiceOffersRow{},
		performers: map[int64][]db.ListPublicServiceStaffRow{}, staffSvcs: map[int64][]int64{}, staffBranch: map[int64][]int64{},
	}
	branches, err := s.q.ListPublicBranches(ctx, o.ID)
	if err != nil {
		return nil, err
	}
	for _, b := range branches {
		c.branches[b.ID] = b.Uuid
		c.branchIDs[b.Uuid] = b.ID
	}
	if c.services, err = s.q.ListPublicServices(ctx, o.ID); err != nil {
		return nil, err
	}
	for _, sv := range c.services {
		c.serviceUUID[sv.ID] = sv.Uuid
	}
	if c.staff, err = s.q.ListPublicStaff(ctx, o.ID); err != nil {
		return nil, err
	}
	for _, st := range c.staff {
		c.staffUUID[st.ID] = st.Uuid
	}
	offers, err := s.q.ListPublicServiceOffers(ctx, o.ID)
	if err != nil {
		return nil, err
	}
	for _, of := range offers {
		if _, ok := c.branches[of.BranchID]; ok {
			c.offers[of.ServiceID] = append(c.offers[of.ServiceID], of)
		}
	}
	staffBranches, err := s.q.ListPublicStaffBranches(ctx, o.ID)
	if err != nil {
		return nil, err
	}
	for _, sb := range staffBranches {
		if _, ok := c.branches[sb.BranchID]; ok {
			c.staffBranch[sb.StaffID] = append(c.staffBranch[sb.StaffID], sb.BranchID)
		}
	}
	performers, err := s.q.ListPublicServiceStaff(ctx, o.ID)
	if err != nil {
		return nil, err
	}
	for _, p := range performers {
		_, staffOK := c.staffUUID[p.StaffID]
		_, serviceOK := c.serviceUUID[p.ServiceID]
		if staffOK && serviceOK {
			c.performers[p.ServiceID] = append(c.performers[p.ServiceID], p)
			c.staffSvcs[p.StaffID] = append(c.staffSvcs[p.StaffID], p.ServiceID)
		}
	}
	return c, nil
}

// branchFilter resolves an optional branch filter (nil = none).
func branchFilter(c *catalog, id *uuid.UUID) (int64, error) {
	if id == nil {
		return 0, nil
	}
	bid, ok := c.branchIDs[*id]
	if !ok {
		return 0, invalid("branch_uuid", "not_found", "branch not found")
	}
	return bid, nil
}

// Services lists the bookable services; with branchUUID only the services
// the branch offers, priced for it.
func (s *Service) Services(ctx context.Context, o Org, branchUUID *uuid.UUID) ([]model.Service, error) {
	c, err := s.loadCatalog(ctx, o)
	if err != nil {
		return nil, err
	}
	branchID, err := branchFilter(c, branchUUID)
	if err != nil {
		return nil, err
	}
	out := []model.Service{}
	for _, sv := range c.services {
		offers := c.offers[sv.ID]
		if len(offers) == 0 {
			continue
		}
		item := model.Service{
			UUID: sv.Uuid, Name: sv.Name, Description: sv.Description, DurationMinutes: int(sv.DurationMinutes),
			Price: money.FromNumericOrZero(sv.Price), Currency: o.Currency, PaymentMode: sv.PaymentMode,
			StaffMode: sv.StaffMode, Branches: []model.ServiceOffer{}, Staff: []model.ServiceStaff{},
		}
		if sv.CategoryUuid.Valid {
			item.Category = &model.Ref{UUID: uuid.UUID(sv.CategoryUuid.Bytes), Name: sv.CategoryName.String}
		}
		if sv.GroupCapacity.Valid {
			seats := int(sv.GroupCapacity.Int32)
			item.GroupCapacity = &seats
		}
		if sv.ImageUuid.Valid {
			u := "/v1/public/media/services/" + o.UUID.String() + "/images/" + uuid.UUID(sv.ImageUuid.Bytes).String()
			item.ImageURL = &u
		}
		offered := false
		for _, of := range offers {
			offer := model.ServiceOffer{BranchUUID: c.branches[of.BranchID], Price: item.Price, DurationMinutes: item.DurationMinutes}
			if of.Price.Valid {
				offer.Price = money.FromNumericOrZero(of.Price)
			}
			if of.DurationMinutes.Valid {
				offer.DurationMinutes = int(of.DurationMinutes.Int32)
			}
			if branchID != 0 && of.BranchID != branchID {
				continue
			}
			if branchID != 0 {
				offered = true
				item.Price, item.DurationMinutes = offer.Price, offer.DurationMinutes
			}
			item.Branches = append(item.Branches, offer)
		}
		if branchID != 0 && !offered {
			continue
		}
		if sv.StaffMode != "none" {
			for _, p := range c.performers[sv.ID] {
				if branchID != 0 && !slices.Contains(c.staffBranch[p.StaffID], branchID) {
					continue
				}
				ps := model.ServiceStaff{StaffUUID: c.staffUUID[p.StaffID]}
				if p.Price.Valid {
					m := money.FromNumericOrZero(p.Price)
					ps.Price = &m
				}
				if p.DurationMinutes.Valid {
					d := int(p.DurationMinutes.Int32)
					ps.DurationMinutes = &d
				}
				item.Staff = append(item.Staff, ps)
			}
		}
		out = append(out, item)
	}
	return out, nil
}

// Staff lists the people shown online, optionally those working in a
// branch and / or performing a service.
func (s *Service) Staff(ctx context.Context, o Org, branchUUID, serviceUUID *uuid.UUID) ([]model.Staff, error) {
	c, err := s.loadCatalog(ctx, o)
	if err != nil {
		return nil, err
	}
	branchID, err := branchFilter(c, branchUUID)
	if err != nil {
		return nil, err
	}
	var serviceID int64
	if serviceUUID != nil {
		for id, u := range c.serviceUUID {
			if u == *serviceUUID {
				serviceID = id
			}
		}
		if serviceID == 0 {
			return nil, invalid("service_uuid", "not_found", "service not found")
		}
	}
	out := []model.Staff{}
	for _, st := range c.staff {
		branches := c.staffBranch[st.ID]
		if len(branches) == 0 || (branchID != 0 && !slices.Contains(branches, branchID)) {
			continue
		}
		services := c.staffSvcs[st.ID]
		if serviceID != 0 && !slices.Contains(services, serviceID) {
			continue
		}
		item := model.Staff{UUID: st.Uuid, Name: st.Name, Title: st.Title, Bio: st.Bio, BranchUUIDs: []uuid.UUID{}, ServiceUUIDs: []uuid.UUID{}}
		for _, b := range branches {
			item.BranchUUIDs = append(item.BranchUUIDs, c.branches[b])
		}
		for _, sv := range services {
			if len(c.offers[sv]) > 0 {
				item.ServiceUUIDs = append(item.ServiceUUIDs, c.serviceUUID[sv])
			}
		}
		out = append(out, item)
	}
	return out, nil
}

// checkChoices refuses the choices of the optional steps the business
// hides (TEC-905): a person when the staff step is hidden (the engine
// assigns a free one), an area when the area step is hidden.
func (s *Service) checkChoices(ctx context.Context, o Org, lines []availabilitymodel.LineInput, zone string) error {
	v, err := s.settings.Current(ctx, o.ID)
	if err != nil {
		return err
	}
	b := v.Settings.Booking
	ve := &ValidationError{}
	if b.Hides(orgsettings.StepStaff) {
		for i, l := range lines {
			if l.StaffUUID != nil {
				ve.add(fmt.Sprintf("lines.%d.staff_uuid", i), "not_offered", "the business assigns the staff member")
			}
		}
	}
	if b.Hides(orgsettings.StepArea) && strings.TrimSpace(zone) != "" {
		ve.add("zone", "not_offered", "the business assigns the area")
	}
	return ve.orNil()
}

// productsHidden reports whether the business hides the products step.
func (s *Service) productsHidden(ctx context.Context, o Org) (bool, error) {
	v, err := s.settings.Current(ctx, o.ID)
	if err != nil {
		return false, err
	}
	return v.Settings.Booking.Hides(orgsettings.StepProducts), nil
}

// Availability searches slots on the online channel.
func (s *Service) Availability(ctx context.Context, o Org, in model.AvailabilityInput) (model.Availability, error) {
	channel, err := channelOf(in.Channel)
	if err != nil {
		return model.Availability{}, err
	}
	if err := s.gate(ctx, o, channel); err != nil {
		return model.Availability{}, err
	}
	if _, err := s.publicBranchID(ctx, o, in.BranchUUID, "branch_uuid"); err != nil {
		return model.Availability{}, err
	}
	if err := s.checkChoices(ctx, o, in.Lines, in.Zone); err != nil {
		return model.Availability{}, err
	}
	res, err := s.availability.Search(ctx, o.ID, availabilitymodel.SearchInput{
		BranchUUID: in.BranchUUID, Lines: in.Lines, From: in.From, To: in.To,
		StepMinutes: in.StepMinutes, Channel: availabilitymodel.ChannelOnline, PartySize: in.PartySize, Zone: in.Zone,
	}, availabilityusecase.Caller{})
	if err != nil {
		return model.Availability{}, err
	}
	out := model.Availability{
		Branch: model.Ref{UUID: res.Branch.UUID, Name: res.Branch.Name}, Timezone: res.Timezone,
		EarliestStart: res.EarliestStart, LatestDate: res.LatestDate, StepMinutes: res.StepMinutes,
		Lines: make([]model.LineInfo, 0, len(res.Lines)), Days: make([]model.Day, 0, len(res.Days)),
	}
	for _, l := range res.Lines {
		info := model.LineInfo{ServiceUUID: l.ServiceUUID, ServiceName: l.ServiceName, DurationMinutes: l.DurationMinutes, StaffMode: l.StaffMode, Staff: make([]model.Ref, 0, len(l.Staff)), GroupCapacity: l.GroupCapacity}
		for _, st := range l.Staff {
			info.Staff = append(info.Staff, model.Ref{UUID: st.UUID, Name: st.Name})
		}
		out.Lines = append(out.Lines, info)
	}
	for _, d := range res.Days {
		day := model.Day{Date: d.Date, Status: d.Status, Slots: make([]model.Slot, 0, len(d.Slots))}
		for _, sl := range d.Slots {
			day.Slots = append(day.Slots, model.Slot{StartsAt: sl.StartsAt, EndsAt: sl.EndsAt, Lines: slotLines(sl.Lines)})
		}
		out.Days = append(out.Days, day)
	}
	return out, nil
}

func slotLines(lines []availabilitymodel.SlotLine) []model.SlotLine {
	out := make([]model.SlotLine, 0, len(lines))
	for _, l := range lines {
		item := model.SlotLine{ServiceUUID: l.ServiceUUID, StartsAt: l.StartsAt, EndsAt: l.EndsAt, SeatsLeft: l.SeatsLeft}
		if l.Staff != nil {
			item.Staff = &model.Ref{UUID: l.Staff.UUID, Name: l.Staff.Name}
		}
		out = append(out, item)
	}
	return out
}

// CreateCheckout holds the picked slot for the customer while they fill in
// the contact form (a checkout hold of the online channel).
func (s *Service) CreateCheckout(ctx context.Context, o Org, in model.CheckoutInput) (model.Checkout, error) {
	channel, err := channelOf(in.Channel)
	if err != nil {
		return model.Checkout{}, err
	}
	if err := s.gate(ctx, o, channel); err != nil {
		return model.Checkout{}, err
	}
	if _, err := s.publicBranchID(ctx, o, in.BranchUUID, "branch_uuid"); err != nil {
		return model.Checkout{}, err
	}
	if err := s.checkChoices(ctx, o, in.Lines, in.Zone); err != nil {
		return model.Checkout{}, err
	}
	set, err := s.settings.Current(ctx, o.ID)
	if err != nil {
		return model.Checkout{}, err
	}
	var token uuid.UUID
	err = s.holds.Run(ctx, o.ID, func(tx *availabilityusecase.Tx) error {
		created, err := s.holds.CreateIn(ctx, tx, availabilitymodel.HoldInput{
			BranchUUID: in.BranchUUID, Lines: in.Lines, Channel: availabilitymodel.ChannelOnline,
			StartsAt: in.StartsAt, Reason: availabilitymodel.ReasonCheckout, PartySize: in.PartySize, Zone: in.Zone,
		}, availabilityusecase.Caller{}, 0)
		if err != nil {
			return err
		}
		// The branch's online bookings of the day (the booking checks again).
		if err := reservationsusecase.CheckDailyLimit(ctx, tx.Q, o.ID, &set, created.Slot.BranchID, created.Slot.StartsAt); err != nil {
			return err
		}
		raw, err := json.Marshal(slotCheckoutLines(created.Slot.Lines))
		if err != nil {
			return err
		}
		row, err := tx.Q.CreatePublicCheckout(ctx, db.CreatePublicCheckoutParams{
			OrganizationID: o.ID, BranchID: created.Slot.BranchID, HoldID: created.ID, Channel: channel,
			Lines: raw, StartsAt: pgtype.Timestamptz{Time: created.Slot.StartsAt, Valid: true},
			PartySize: partyParam(in.PartySize), Zone: strings.TrimSpace(in.Zone),
		})
		if err != nil {
			return err
		}
		token = row.Uuid
		return nil
	})
	if err != nil {
		return model.Checkout{}, err
	}
	return s.Checkout(ctx, o, token)
}

func slotCheckoutLines(lines []availabilitymodel.SlotLine) []model.CheckoutLine {
	out := make([]model.CheckoutLine, 0, len(lines))
	for _, l := range lines {
		item := model.CheckoutLine{ServiceUUID: l.ServiceUUID, StartsAt: l.StartsAt, EndsAt: l.EndsAt}
		if l.Staff != nil {
			item.Staff = &model.Ref{UUID: l.Staff.UUID, Name: l.Staff.Name}
		}
		out = append(out, item)
	}
	return out
}

// Checkout returns a checkout by its token.
func (s *Service) Checkout(ctx context.Context, o Org, token uuid.UUID) (model.Checkout, error) {
	row, err := s.q.GetPublicCheckout(ctx, db.GetPublicCheckoutParams{OrganizationID: o.ID, Uuid: token})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Checkout{}, ErrCheckoutNotFound
	}
	if err != nil {
		return model.Checkout{}, err
	}
	out := model.Checkout{
		Token: row.Uuid, Channel: row.Channel, Branch: model.Ref{UUID: row.BranchUuid, Name: row.BranchName},
		StartsAt: row.StartsAt.Time.UTC(), EndsAt: row.HoldEndsAt.Time.UTC(), Lines: []model.CheckoutLine{},
		Zone: row.Zone,
	}
	if row.PartySize.Valid {
		n := int(row.PartySize.Int32)
		out.PartySize = &n
	}
	if err := json.Unmarshal(row.Lines, &out.Lines); err != nil {
		return model.Checkout{}, err
	}
	if row.HoldExpiresAt.Valid {
		t := row.HoldExpiresAt.Time.UTC()
		out.ExpiresAt = &t
	}
	switch {
	case row.ReservationUuid.Valid:
		u := uuid.UUID(row.ReservationUuid.Bytes)
		out.Status, out.ReservationUUID, out.ExpiresAt = model.CheckoutBooked, &u, nil
	case row.HoldStatus == availabilitymodel.StatusActive && out.ExpiresAt != nil && !out.ExpiresAt.After(s.now()):
		out.Status = model.CheckoutExpired
	case row.HoldStatus == availabilitymodel.StatusActive:
		out.Status = model.CheckoutActive
	case row.HoldStatus == availabilitymodel.StatusExpired:
		out.Status = model.CheckoutExpired
	default:
		out.Status = model.CheckoutReleased
	}
	return out, nil
}

// ReleaseCheckout frees the time of a checkout the customer abandons. A
// booked, expired or already released checkout is answered as it is.
func (s *Service) ReleaseCheckout(ctx context.Context, o Org, token uuid.UUID) (model.Checkout, error) {
	row, err := s.q.GetPublicCheckout(ctx, db.GetPublicCheckoutParams{OrganizationID: o.ID, Uuid: token})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Checkout{}, ErrCheckoutNotFound
	}
	if err != nil {
		return model.Checkout{}, err
	}
	if !row.ReservationID.Valid {
		err := s.holds.Run(ctx, o.ID, func(tx *availabilityusecase.Tx) error {
			return ignoreInactive(s.holds.ReleaseIn(ctx, tx, row.HoldID, "checkout_released"))
		})
		if err != nil {
			return model.Checkout{}, err
		}
	}
	return s.Checkout(ctx, o, token)
}

func ignoreInactive(err error) error {
	if errors.Is(err, availabilityusecase.ErrHoldExpired) || errors.Is(err, availabilityusecase.ErrNotActive) {
		return nil
	}
	return err
}

// alreadyBooked: the checkout was booked before (a retried request).
type alreadyBooked struct {
	reservationID int64
}

func (e *alreadyBooked) Error() string { return "the checkout was already booked" }

// contact is the validated contact form.
type contact struct {
	firstName, lastName, phone, email string
	smsConsent, emailConsent          bool
}

func validateContact(ve *ValidationError, in model.CustomerInput) contact {
	c := contact{smsConsent: in.SMSConsent, emailConsent: in.EmailConsent}
	c.firstName = strings.TrimSpace(in.FirstName)
	c.lastName = strings.TrimSpace(in.LastName)
	switch {
	case c.firstName == "":
		ve.add("customer.first_name", "required", "first name is required")
	case utf8.RuneCountInString(c.firstName) > maxNameLength:
		ve.add("customer.first_name", "too_long", fmt.Sprintf("first name must be at most %d characters", maxNameLength))
	}
	if utf8.RuneCountInString(c.lastName) > maxNameLength {
		ve.add("customer.last_name", "too_long", fmt.Sprintf("last name must be at most %d characters", maxNameLength))
	}
	normalized, err := phone.Normalize(in.Phone, "")
	switch {
	case strings.TrimSpace(in.Phone) == "":
		ve.add("customer.phone", "required", "phone is required")
	case err != nil || normalized == "":
		ve.add("customer.phone", "invalid", "phone is not a valid number")
	default:
		c.phone = normalized
	}
	if email := strings.TrimSpace(in.Email); email != "" {
		addr, err := mail.ParseAddress(email)
		switch {
		case len(email) > maxEmailLength:
			ve.add("customer.email", "too_long", fmt.Sprintf("email must be at most %d characters", maxEmailLength))
		case err != nil || addr.Address != email || !strings.Contains(email, "@"):
			ve.add("customer.email", "invalid", "email is not a valid address")
		default:
			c.email = strings.ToLower(email)
		}
	}
	return c
}

// NormalizePhone returns the E.164 form of a phone ("" when invalid); the
// handler limits bookings per phone with it.
func NormalizePhone(raw string) string {
	n, err := phone.Normalize(raw, "")
	if err != nil {
		return ""
	}
	return n
}

// Book turns a checkout into a reservation for the customer of the phone.
// The checkout's time is freed and booked again in one transaction (an
// expired checkout still books when the time is free). The customer is the
// business's live record with the phone, else a new record (source online);
// an existing record is not changed. A checkout books once: a retry answers
// the same reservation with replayed = true.
func (s *Service) Book(ctx context.Context, o Org, in model.BookingInput) (model.Reservation, bool, error) {
	if len(in.PackageSessions) > 0 {
		// Using a package needs a proven phone: the customer panel route.
		return model.Reservation{}, false, invalid("package_sessions", "sign_in_required", "sign in to use a package")
	}
	return s.book(ctx, o, in, nil)
}

// CustomerOf returns the business's customer record of a signed-in
// customer panel session; uuid.Nil when the business does not know the
// person.
type CustomerOf func(ctx context.Context, orgID int64) (uuid.UUID, error)

// BookAs books a checkout for the business's customer record of a
// signed-in customer panel session (TEC-984): the person proved the phone,
// so the booking may use sessions of the person's packages. The contact
// form is not used (the record stays as the business keeps it); a business
// that does not know the person answers customer / not_linked.
func (s *Service) BookAs(ctx context.Context, o Org, in model.BookingInput, customer CustomerOf) (model.Reservation, bool, error) {
	if len(in.PackageSessions) > 0 && !o.modules.Allows(sector.ModulePackages) {
		return model.Reservation{}, false, invalid("package_sessions", "not_allowed", "the business does not sell packages")
	}
	return s.book(ctx, o, in, customer)
}

func (s *Service) book(ctx context.Context, o Org, in model.BookingInput, known CustomerOf) (model.Reservation, bool, error) {
	ve := &ValidationError{}
	if strings.TrimSpace(in.Website) != "" {
		ve.add("website", "rejected", "the request was rejected")
		return model.Reservation{}, false, ve
	}
	if in.CheckoutToken == uuid.Nil {
		ve.add("checkout_token", "required", "checkout token is required")
	}
	var c contact
	if known == nil {
		c = validateContact(ve, in.Customer)
	}
	if err := ve.orNil(); err != nil {
		return model.Reservation{}, false, err
	}
	checkout, err := s.q.GetPublicCheckout(ctx, db.GetPublicCheckoutParams{OrganizationID: o.ID, Uuid: in.CheckoutToken})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Reservation{}, false, invalid("checkout_token", "not_found", "checkout not found")
	}
	if err != nil {
		return model.Reservation{}, false, err
	}
	if checkout.ReservationUuid.Valid {
		out, err := s.reservation(ctx, o, uuid.UUID(checkout.ReservationUuid.Bytes))
		return out, true, err
	}
	if err := s.gate(ctx, o, checkout.Channel); err != nil {
		return model.Reservation{}, false, err
	}
	if _, err := s.publicBranchID(ctx, o, checkout.BranchUuid, "checkout_token"); err != nil {
		return model.Reservation{}, false, err
	}
	var lines []model.CheckoutLine
	if err := json.Unmarshal(checkout.Lines, &lines); err != nil {
		return model.Reservation{}, false, err
	}
	input := reservationsmodel.CreateInput{
		BranchUUID: checkout.BranchUuid, StartsAt: &checkout.StartsAt.Time, Source: reservationsmodel.SourceOnline,
		Payment: in.Payment, CustomFields: in.CustomFields, CustomerNote: strings.TrimSpace(in.CustomerNote),
		Zone: checkout.Zone, Vehicle: in.Vehicle, PackageSessions: in.PackageSessions,
	}
	if checkout.PartySize.Valid {
		n := int(checkout.PartySize.Int32)
		input.PartySize = &n
	}
	if checkout.Channel == model.ChannelWidget {
		input.Source = reservationsmodel.SourceEmbed
	}
	for _, l := range lines {
		line := availabilitymodel.LineInput{ServiceUUID: l.ServiceUUID}
		if l.Staff != nil {
			staff := l.Staff.UUID
			line.StaffUUID = &staff
		}
		input.Lines = append(input.Lines, line)
	}
	if len(in.Products) > 0 && !o.modules.Allows(sector.ModuleProducts) {
		return model.Reservation{}, false, invalid("products", "not_allowed", "the business does not sell products online")
	}
	if len(in.Products) > 0 {
		hidden, err := s.productsHidden(ctx, o)
		if err != nil {
			return model.Reservation{}, false, err
		}
		if hidden {
			return model.Reservation{}, false, invalid("products", "not_offered", "the business does not offer products with a booking")
		}
	}
	for i, p := range in.Products {
		if p.Quantity < 1 || p.Quantity > model.MaxProductQuantity {
			return model.Reservation{}, false, invalid(fmt.Sprintf("products[%d].quantity", i), "invalid",
				fmt.Sprintf("quantity must be between 1 and %d", model.MaxProductQuantity))
		}
		input.Products = append(input.Products, reservationsmodel.ProductInput{
			ProductUUID: p.ProductUUID, VariantUUID: p.VariantUUID, Quantity: p.Quantity,
		})
	}
	var createdCustomer uuid.UUID
	res, err := s.reservations.CreateWith(ctx, o.ID, input, availabilityusecase.Caller{}, 0, reservationsusecase.CreateHooks{
		Before: func(ctx context.Context, tx *availabilityusecase.Tx) (uuid.UUID, error) {
			locked, err := tx.Q.GetPublicCheckoutForUpdate(ctx, db.GetPublicCheckoutForUpdateParams{OrganizationID: o.ID, Uuid: checkout.Uuid})
			if err != nil {
				return uuid.Nil, err
			}
			if locked.ReservationID.Valid {
				return uuid.Nil, &alreadyBooked{reservationID: locked.ReservationID.Int64}
			}
			if err := ignoreInactive(s.holds.ReleaseIn(ctx, tx, locked.HoldID, "checkout_booked")); err != nil {
				return uuid.Nil, err
			}
			if known != nil {
				id, err := known(ctx, o.ID)
				if err == nil && id == uuid.Nil {
					return uuid.Nil, invalid("customer", "not_linked", "the business does not know the signed-in person")
				}
				return id, err
			}
			id, created, err := findOrCreateCustomer(ctx, tx.Q, o.ID, c)
			if created {
				createdCustomer = id
			}
			return id, err
		},
		After: func(ctx context.Context, tx *availabilityusecase.Tx, reservationID int64) error {
			return tx.Q.SetPublicCheckoutReservation(ctx, db.SetPublicCheckoutReservationParams{
				OrganizationID: o.ID, ID: checkout.ID, ReservationID: pgtype.Int8{Int64: reservationID, Valid: true},
			})
		},
	})
	var booked *alreadyBooked
	if errors.As(err, &booked) {
		id, err := s.q.GetPublicReservationUUID(ctx, db.GetPublicReservationUUIDParams{OrganizationID: o.ID, ID: booked.reservationID})
		if err != nil {
			return model.Reservation{}, false, err
		}
		out, err := s.reservation(ctx, o, id)
		return out, true, err
	}
	if err != nil {
		return model.Reservation{}, false, err
	}
	if createdCustomer != uuid.Nil && s.indexer != nil {
		s.indexer.EnqueueUpsert(ctx, customersusecase.SearchSpec, searchengine.TenantDocumentID(o.ID, createdCustomer.String()))
	}
	// The panel link goes out with the "request received" / "accepted"
	// notice the reservation change wrote (customernotify, TEC-911).
	out, err := s.view(ctx, o, res)
	return out, false, err
}

// findOrCreateCustomer returns the business's live record with the phone,
// or creates one from the contact form (source online, unverified).
func findOrCreateCustomer(ctx context.Context, q *db.Queries, orgID int64, c contact) (uuid.UUID, bool, error) {
	existing, err := q.GetCustomerByPhone(ctx, db.GetCustomerByPhoneParams{OrganizationID: orgID, PhoneE164: pgtype.Text{String: c.phone, Valid: true}})
	if err == nil {
		return existing.Uuid, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, err
	}
	params := db.CreateCustomerParams{
		OrganizationID: orgID, FirstName: c.firstName, LastName: c.lastName,
		PhoneE164: pgtype.Text{String: c.phone, Valid: true}, Email: c.email, Tags: []string{},
		SmsConsent: c.smsConsent, EmailConsent: c.emailConsent, Source: "online",
	}
	if c.smsConsent || c.emailConsent {
		params.ConsentUpdatedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	}
	row, err := q.CreateCustomer(ctx, params)
	if err != nil {
		return uuid.Nil, false, err
	}
	return row.Uuid, true, nil
}

// AccessToken is the reservation's access token for the status route: a
// keyed hash of the business and the reservation, so it needs no storage
// and cannot be derived without the server key.
func (s *Service) AccessToken(o Org, id uuid.UUID) string {
	return s.signer.BlindIndex(accessTokenPurpose, o.UUID.String(), id.String())
}

// Reservation returns the status view of a reservation; token must be its
// access token (anything else answers ErrReservationNotFound).
func (s *Service) Reservation(ctx context.Context, o Org, id uuid.UUID, token string) (model.Reservation, error) {
	want := s.AccessToken(o, id)
	if token == "" || subtle.ConstantTimeCompare([]byte(want), []byte(strings.TrimSpace(token))) != 1 {
		return model.Reservation{}, ErrReservationNotFound
	}
	out, err := s.reservation(ctx, o, id)
	if err != nil {
		return model.Reservation{}, err
	}
	out.AccessToken = ""
	return out, nil
}

func (s *Service) reservation(ctx context.Context, o Org, id uuid.UUID) (model.Reservation, error) {
	res, err := s.reservations.Get(ctx, o.ID, id, availabilityusecase.Caller{})
	if errors.Is(err, reservationsusecase.ErrNotFound) {
		return model.Reservation{}, ErrReservationNotFound
	}
	if err != nil {
		return model.Reservation{}, err
	}
	return s.view(ctx, o, res)
}

// view maps the record to the customer's view (no internal note, history
// or resources) with its access token.
func (s *Service) view(ctx context.Context, o Org, r reservationsmodel.Reservation) (model.Reservation, error) {
	meta, err := s.q.GetPublicReservationMeta(ctx, db.GetPublicReservationMetaParams{OrganizationID: o.ID, Uuid: r.UUID})
	if err != nil {
		return model.Reservation{}, err
	}
	out := model.Reservation{
		UUID: r.UUID, Number: r.Number, Status: r.Status, ApprovalStatus: r.ApprovalStatus,
		ConfirmationStatus: r.ConfirmationStatus, PaymentStatus: r.PaymentStatus,
		Branch: model.Ref{UUID: r.Branch.UUID, Name: r.Branch.Name}, Timezone: meta.Timezone,
		StartsAt: r.StartsAt, EndsAt: r.EndsAt, TotalPrice: r.TotalPrice, Currency: r.Currency,
		CustomerNote: r.CustomerNote, ExpiresAt: r.HoldExpiresAt, CancelReason: r.CancelReason,
		PartySize: r.PartySize, Zone: r.Zone, Vehicle: r.Vehicle,
		Lines: make([]model.Line, 0, len(r.Lines)), Products: make([]model.BookedProduct, 0, len(r.Products)),
		CreatedAt: r.CreatedAt, AccessToken: s.AccessToken(o, r.UUID), Answers: []model.Answer{},
	}
	for _, a := range r.CustomAnswers {
		if a.Online {
			out.Answers = append(out.Answers, model.Answer{Key: a.Key, Label: a.Label, Type: a.Type, Value: a.Value})
		}
	}
	for _, p := range r.Products {
		item := model.BookedProduct{Name: p.Product.Name, Quantity: p.Quantity, UnitPrice: p.UnitPrice, Status: p.Status}
		if p.Variant != nil {
			name := p.Variant.Name
			item.VariantName = &name
		}
		out.Products = append(out.Products, item)
	}
	onPackage := map[int]string{}
	for _, ps := range r.PackageSessions {
		if ps.Status != reservationsmodel.SessionReleased {
			onPackage[ps.LineIndex] = ps.CustomerPackage.Name
		}
	}
	for _, l := range r.Lines {
		line := model.Line{
			ServiceUUID: l.ServiceUUID, ServiceName: l.ServiceName, StartsAt: l.StartsAt, EndsAt: l.EndsAt,
			DurationMinutes: l.DurationMinutes, UnitPrice: l.UnitPrice,
		}
		if name, ok := onPackage[l.Index]; ok {
			line.PackageName = &name
		}
		if l.Staff != nil {
			line.Staff = &model.StaffName{UUID: l.Staff.UUID, Name: l.Staff.Name}
		}
		out.Lines = append(out.Lines, line)
	}
	out.CanPayOnline = r.Status == reservationsmodel.StatusRequested && r.ApprovalStatus == reservationsmodel.ApprovalAccepted &&
		r.PaymentStatus == reservationsmodel.PaymentOnlinePending && r.HoldExpiresAt != nil && r.HoldExpiresAt.After(s.now()) &&
		s.onlineReady(ctx, o)
	return out, nil
}

func (s *Service) onlineReady(ctx context.Context, o Org) bool {
	return s.payments != nil && s.payments.OnlineReady(ctx, o.ID)
}

// StartPayment opens the payment page of a reservation for the holder of
// its access token (ErrReservationNotFound otherwise).
func (s *Service) StartPayment(ctx context.Context, o Org, id uuid.UUID, token, ip, locale string) (paymentsmodel.OnlineStart, error) {
	if _, err := s.Reservation(ctx, o, id, token); err != nil {
		return paymentsmodel.OnlineStart{}, err
	}
	if s.payments == nil {
		return paymentsmodel.OnlineStart{}, paymentsusecase.ErrNotReady
	}
	return s.payments.StartOnline(ctx, paymentsusecase.StartInput{
		OrganizationID: o.ID, Slug: o.Slug, ReservationUUID: id, IP: ip, Locale: locale,
	})
}

// partyParam stores a party size.
func partyParam(v *int) pgtype.Int4 {
	if v == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(*v), Valid: true}
}

// Package model holds the API shapes of the products tenant resource.
package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// Slug is the stable slug of the products resource (activity, io, bulk,
// live lists). Stock movements are recorded under the product.
const Slug = "tenant.products"

// MovementsSlug is the engine slug of the stock ledger (export, count
// import; TEC-964).
const MovementsSlug = "tenant.stock_movements"

// Units a product is counted in.
var Units = []string{"piece", "bottle", "box", "pack", "tube", "set"}

// Movement kinds stored in the ledger.
const (
	KindReceipt     = "receipt"
	KindSale        = "sale"
	KindReturn      = "return"
	KindAdjustment  = "adjustment"
	KindTransferOut = "transfer_out"
	KindTransferIn  = "transfer_in"
)

// MovementKinds are the ledger kinds (list filter values).
var MovementKinds = []string{KindReceipt, KindSale, KindReturn, KindAdjustment, KindTransferOut, KindTransferIn}

// KindTransfer is the input kind of a transfer between branches; it writes
// a transfer_out and a transfer_in row.
const KindTransfer = "transfer"

// InputKinds are the kinds POST /stock-movements accepts.
var InputKinds = []string{KindReceipt, KindSale, KindReturn, KindAdjustment, KindTransfer}

// Stock status values of a product (list filter, row badge).
const (
	StockIn  = "in"
	StockLow = "low"
	StockOut = "out"
)

// StockStatuses lists the stock status filter values.
var StockStatuses = []string{StockIn, StockLow, StockOut}

// Stock is the stock of a product or variant summed over the branches of
// the caller's scope. Available = OnHand - Reserved (reserved units belong
// to appointments, TEC-919).
type Stock struct {
	OnHand    int64  `json:"on_hand"`
	Reserved  int64  `json:"reserved"`
	Available int64  `json:"available"`
	Status    string `json:"status"`
}

// Offer is a branch selling the product. Price is the branch override
// (null = catalog price). Editable is false for branches outside the
// caller's scope.
type Offer struct {
	BranchUUID     uuid.UUID    `json:"branch_uuid"`
	BranchName     string       `json:"branch_name"`
	BranchCode     string       `json:"branch_code"`
	Price          *money.Money `json:"price"`
	EffectivePrice money.Money  `json:"effective_price"`
	IsActive       bool         `json:"is_active"`
	Editable       bool         `json:"editable"`
}

// Variant is a variant with its own stock code. Price / Cost override the
// product values (null = product value).
type Variant struct {
	UUID           uuid.UUID    `json:"uuid"`
	Name           string       `json:"name"`
	SKU            string       `json:"sku"`
	Barcode        string       `json:"barcode"`
	Price          *money.Money `json:"price"`
	Cost           *money.Money `json:"cost"`
	EffectivePrice money.Money  `json:"effective_price"`
	EffectiveCost  money.Money  `json:"effective_cost"`
	IsActive       bool         `json:"is_active"`
	SortOrder      int32        `json:"sort_order"`
	Stock          Stock        `json:"stock"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

// StockLevel is the stock of one stock item (the product, or a variant) in
// one branch of the caller's scope.
type StockLevel struct {
	BranchUUID  uuid.UUID  `json:"branch_uuid"`
	BranchName  string     `json:"branch_name"`
	BranchCode  string     `json:"branch_code"`
	VariantUUID *uuid.UUID `json:"variant_uuid"`
	VariantName string     `json:"variant_name"`
	VariantSKU  string     `json:"variant_sku"`
	OnHand      int64      `json:"on_hand"`
	Reserved    int64      `json:"reserved"`
	Available   int64      `json:"available"`
	Status      string     `json:"status"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// Product is the API projection of a catalog product. Variants and Levels
// are filled on the detail only.
type Product struct {
	UUID              uuid.UUID    `json:"uuid"`
	Name              string       `json:"name"`
	Description       string       `json:"description"`
	SKU               string       `json:"sku"`
	Barcode           string       `json:"barcode"`
	Unit              string       `json:"unit"`
	Price             money.Money  `json:"price"`
	Cost              money.Money  `json:"cost"`
	Currency          string       `json:"currency"`
	LowStockThreshold int32        `json:"low_stock_threshold"`
	ShowOnline        bool         `json:"show_online"`
	IsActive          bool         `json:"is_active"`
	ImageURL          *string      `json:"image_url"`
	VariantsCount     int64        `json:"variants_count"`
	Stock             Stock        `json:"stock"`
	Offers            []Offer      `json:"offers"`
	Variants          []Variant    `json:"variants,omitempty"`
	Levels            []StockLevel `json:"levels,omitempty"`
	CreatedAt         time.Time    `json:"created_at"`
	UpdatedAt         time.Time    `json:"updated_at"`
}

// Input is the create / patch body. Absent fields keep their value on
// patch and take their defaults on create. BranchUUIDs (create only) adds
// offers at the catalog price in those branches.
type Input struct {
	Name              *string      `json:"name"`
	Description       *string      `json:"description"`
	SKU               *string      `json:"sku"`
	Barcode           *string      `json:"barcode"`
	Unit              *string      `json:"unit"`
	Price             *json.Number `json:"price"`
	Cost              *json.Number `json:"cost"`
	LowStockThreshold *int32       `json:"low_stock_threshold"`
	ShowOnline        *bool        `json:"show_online"`
	IsActive          *bool        `json:"is_active"`
	BranchUUIDs       []uuid.UUID  `json:"branch_uuids"`
}

// OptionalNumber distinguishes an absent amount from null (clear).
type OptionalNumber struct {
	Set   bool
	Value *json.Number
}

func (o *OptionalNumber) UnmarshalJSON(b []byte) error {
	o.Set = true
	if string(b) == "null" {
		o.Value = nil
		return nil
	}
	var v json.Number
	if err := json.Unmarshal(b, &v); err != nil {
		// Accept amounts sent as strings ("12.50").
		var s string
		if err2 := json.Unmarshal(b, &s); err2 != nil {
			return err
		}
		v = json.Number(s)
	}
	o.Value = &v
	return nil
}

// VariantInput is the create / patch body of a variant. Price / cost null
// use the product values.
type VariantInput struct {
	Name      *string        `json:"name"`
	SKU       *string        `json:"sku"`
	Barcode   *string        `json:"barcode"`
	Price     OptionalNumber `json:"price"`
	Cost      OptionalNumber `json:"cost"`
	IsActive  *bool          `json:"is_active"`
	SortOrder *int32         `json:"sort_order"`
}

// OfferInput is one branch offer of PUT …/offers.
type OfferInput struct {
	BranchUUID uuid.UUID    `json:"branch_uuid"`
	Price      *json.Number `json:"price"`
	IsActive   *bool        `json:"is_active"`
}

// OffersInput replaces the offers of the branches the caller manages.
type OffersInput struct {
	Offers []OfferInput `json:"offers"`
}

// ListFilter selects products of one organization.
type ListFilter struct {
	Limit       int32
	Offset      int32
	Q           string
	Branches    []uuid.UUID
	StockStatus []string
	IsActive    *bool
	ShowOnline  *bool
	SortField   string
	SortDesc    bool
}

// Facet is one option of a faceted filter with its row count.
type Facet struct {
	Value string `json:"value"`
	Label string `json:"label,omitempty"`
	Count int64  `json:"count"`
}

// Facets are the dynamic filter options of the product list.
type Facets struct {
	Branch []Facet `json:"branch"`
}

// LookupMatch is one product (and variant) whose SKU or barcode is the
// scanned code.
type LookupMatch struct {
	Product Product  `json:"product"`
	Variant *Variant `json:"variant"`
}

// Ref names a related record of a movement.
type Ref struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
	SKU  string    `json:"sku,omitempty"`
}

// Movement is one row of the stock ledger.
type Movement struct {
	UUID              uuid.UUID    `json:"uuid"`
	Kind              string       `json:"kind"`
	Quantity          int32        `json:"quantity"`
	BalanceAfter      int32        `json:"balance_after"`
	Product           Ref          `json:"product"`
	Variant           *Ref         `json:"variant"`
	Branch            Ref          `json:"branch"`
	CounterpartBranch *Ref         `json:"counterpart_branch"`
	TransferUUID      *uuid.UUID   `json:"transfer_uuid"`
	UnitPrice         *money.Money `json:"unit_price"`
	UnitCost          *money.Money `json:"unit_cost"`
	Currency          string       `json:"currency"`
	Staff             *Ref         `json:"staff"`
	Reason            string       `json:"reason"`
	Note              string       `json:"note"`
	Source            string       `json:"source"`
	// Returned: a sale whose product was taken back (TEC-913).
	Returned   bool      `json:"returned"`
	OccurredAt time.Time `json:"occurred_at"`
	CreatedBy  *Ref      `json:"created_by"`
	CreatedAt  time.Time `json:"created_at"`
}

// MovementInput is the body of POST /stock-movements. Quantity is the
// positive number of units (receipt, sale, return, transfer);
// CountedQuantity is the counted stock of an adjustment (sayım).
type MovementInput struct {
	Kind            string       `json:"kind"`
	BranchUUID      uuid.UUID    `json:"branch_uuid"`
	ToBranchUUID    *uuid.UUID   `json:"to_branch_uuid"`
	ProductUUID     uuid.UUID    `json:"product_uuid"`
	VariantUUID     *uuid.UUID   `json:"variant_uuid"`
	Quantity        *int32       `json:"quantity"`
	CountedQuantity *int32       `json:"counted_quantity"`
	UnitPrice       *json.Number `json:"unit_price"`
	UnitCost        *json.Number `json:"unit_cost"`
	StaffUUID       *uuid.UUID   `json:"staff_uuid"`
	Reason          *string      `json:"reason"`
	Note            *string      `json:"note"`
	OccurredAt      *time.Time   `json:"occurred_at"`
	// Source marks what produced the movement (not part of the body):
	// empty is a panel entry ("manual"), SourceImport a count import.
	Source string `json:"-"`
}

// MovementFilter selects ledger rows.
type MovementFilter struct {
	Limit     int32
	Offset    int32
	Q         string
	Branches  []uuid.UUID
	Products  []uuid.UUID
	Staff     []uuid.UUID
	Kinds     []string
	From      *time.Time
	To        *time.Time
	SortField string
	SortDesc  bool
}

// MovementFacets are the dynamic filter options of the movement list.
type MovementFacets struct {
	Kind   []Facet `json:"kind"`
	Branch []Facet `json:"branch"`
	Staff  []Facet `json:"staff"`
}

// ActivityActor is the user behind an activity entry.
type ActivityActor struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   string    `json:"email"`
}

// ActivityEntry is one audit event of a product.
type ActivityEntry struct {
	UUID      uuid.UUID       `json:"uuid"`
	Action    string          `json:"action"`
	Payload   json.RawMessage `json:"payload"`
	Actor     *ActivityActor  `json:"actor"`
	CreatedAt time.Time       `json:"created_at"`
}

// SourceSaleReturn marks the return movement of a sale (its source_uuid is
// the sale movement).
const SourceSaleReturn = "sale_return"

// SourceImport marks a count written by a stock count import (TEC-964);
// SourceImportRollback the adjustment that undid it (its source_uuid is
// the imported count).
const (
	SourceImport         = "import"
	SourceImportRollback = "import_rollback"
)

// ReturnInput takes back the product of a stock sale (a manager's
// operation): the units come back to the branch's stock and the sale's
// revenue line is reversed.
type ReturnInput struct {
	Reason string `json:"reason"`
	Note   string `json:"note"`
}

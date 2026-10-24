package usecase

import (
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerpanel/model"
	reservationsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
)

// ReleaseRemoved is the release reason of a product line the business took
// off the visit; the customer never took it, so the panel does not show it.
const ReleaseRemoved = "removed"

// productsView maps the products of a visit to the customer's lines
// (TEC-977): held and delivered lines, and lines released with the visit
// (cancelled, rejected, expired, no-show) as "released".
func productsView(in []reservationsmodel.Product) []model.Product {
	out := make([]model.Product, 0, len(in))
	for _, p := range in {
		if p.Status == reservationsmodel.ProductReleased && p.ReleaseReason != nil && *p.ReleaseReason == ReleaseRemoved {
			continue
		}
		line := model.Product{
			UUID: p.UUID, Name: p.Product.Name, Quantity: p.Quantity,
			UnitPrice: p.UnitPrice, Total: p.Total, Status: p.Status,
		}
		if p.Variant != nil {
			name := p.Variant.Name
			line.VariantName = &name
		}
		out = append(out, line)
	}
	return out
}

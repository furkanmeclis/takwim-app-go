// Package publicbooking is the public booking API of Takwim (TEC-906):
// unauthenticated /v1/public/businesses/{slug}/... routes for the business
// page (takwim.link/{slug}, TEC-907), the embedded widget (TEC-908) and the
// mobile app. See docs/design/public-booking-api.md.
package publicbooking

import (
	"net/http"

	publicbookinghandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicbooking/handler"
)

// RegisterRoutes mounts /v1/public/businesses/{slug}. The routes are
// public: the business is resolved from the slug on every request.
func RegisterRoutes(mux *http.ServeMux, h *publicbookinghandler.Handler) {
	mux.HandleFunc("GET /v1/public/businesses/{slug}", h.Business)
	mux.HandleFunc("GET /v1/public/businesses/{slug}/logo", h.Logo)
	mux.HandleFunc("GET /v1/public/businesses/{slug}/branches", h.Branches)
	mux.HandleFunc("GET /v1/public/businesses/{slug}/services", h.Services)
	mux.HandleFunc("GET /v1/public/businesses/{slug}/staff", h.Staff)
	mux.HandleFunc("POST /v1/public/businesses/{slug}/availability", h.Availability)
	mux.HandleFunc("POST /v1/public/businesses/{slug}/checkouts", h.CreateCheckout)
	mux.HandleFunc("GET /v1/public/businesses/{slug}/checkouts/{token}", h.GetCheckout)
	mux.HandleFunc("DELETE /v1/public/businesses/{slug}/checkouts/{token}", h.ReleaseCheckout)
	mux.HandleFunc("POST /v1/public/businesses/{slug}/reservations", h.CreateReservation)
	mux.HandleFunc("GET /v1/public/businesses/{slug}/reservations/{uuid}", h.GetReservation)
}

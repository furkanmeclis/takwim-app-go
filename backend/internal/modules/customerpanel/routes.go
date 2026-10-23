// Package customerpanel is the shared customer panel of Takwim (TEC-909,
// takwim.link/panel): sign-in by a code sent to the phone and the person's
// reservations and payments across businesses. See
// docs/design/customer-panel.md.
package customerpanel

import (
	"net/http"

	customerpanelhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerpanel/handler"
)

// RegisterRoutes mounts /v1/customer/*. The sign-in routes are public; the
// rest need a customer session (never a staff token).
func RegisterRoutes(mux *http.ServeMux, h *customerpanelhandler.Handler) {
	mux.HandleFunc("POST /v1/customer/auth/start", h.Start)
	mux.HandleFunc("POST /v1/customer/auth/verify", h.Verify)
	mux.HandleFunc("POST /v1/customer/auth/logout", h.Authenticate(h.Logout))
	mux.HandleFunc("GET /v1/customer/me", h.Authenticate(h.Me))
	mux.HandleFunc("GET /v1/customer/reservations", h.Authenticate(h.Reservations))
	mux.HandleFunc("GET /v1/customer/reservations/{uuid}", h.Authenticate(h.Reservation))
	mux.HandleFunc("POST /v1/customer/reservations/{uuid}/cancel", h.Authenticate(h.Cancel))
	mux.HandleFunc("POST /v1/customer/reservations/{uuid}/availability", h.Authenticate(h.Availability))
	mux.HandleFunc("POST /v1/customer/reservations/{uuid}/reschedule", h.Authenticate(h.Reschedule))
	mux.HandleFunc("GET /v1/customer/payments", h.Authenticate(h.Payments))
	mux.HandleFunc("GET /v1/customer/payments/summary", h.Authenticate(h.PaymentSummary))
}

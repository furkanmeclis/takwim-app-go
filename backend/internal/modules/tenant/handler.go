package tenant

import (
	"net/http"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

type organizationDTO struct {
	UUID         string     `json:"uuid"`
	Slug         string     `json:"slug"`
	Name         string     `json:"name"`
	Status       string     `json:"status"`
	AccessEndsAt *time.Time `json:"access_ends_at"`
}

type contextDTO struct {
	Organization organizationDTO `json:"organization"`
	Role         string          `json:"role"`
	Permissions  []string        `json:"permissions"`
}

// getContext returns the resolved organization scope of the request.
func getContext(w http.ResponseWriter, r *http.Request) {
	s := orgctx.Must(r.Context())
	perms := s.Permissions
	if perms == nil {
		perms = []string{}
	}
	response.JSON(w, r, http.StatusOK, contextDTO{
		Organization: organizationDTO{
			UUID:         s.UUID.String(),
			Slug:         s.Slug,
			Name:         s.Name,
			Status:       s.Status,
			AccessEndsAt: s.AccessEndsAt,
		},
		Role:        s.MemberRole,
		Permissions: perms,
	})
}

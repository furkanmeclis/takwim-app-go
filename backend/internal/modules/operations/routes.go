// Package operations mounts the platform operations routes (TEC-930,
// docs/OPERATIONS.md):
//
//	GET    /v1/platform/operations/summary                       queues, workers, failure counts, error tracking
//	GET    /v1/platform/operations/queue-tasks                   failed queue tasks (state=archived|retry)
//	POST   /v1/platform/operations/queue-tasks/{queue}/{id}/run  run a failed task again now
//	DELETE /v1/platform/operations/queue-tasks/{queue}/{id}      drop a failed task
//	GET    /v1/platform/operations/integration-jobs              provider errors of every organization
//	GET    /v1/platform/operations/integration-jobs/{uuid}       job + provider calls
//	POST   /v1/platform/operations/integration-jobs/{uuid}/retry the runner's retry (CheckStatus first)
//	GET    /v1/platform/operations/notice-failures               failed customer notice sends
//
// Reads need platform.settings.read (like the system health page), actions
// platform.settings.write.
package operations

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/operations/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
)

// RegisterRoutes mounts the routes.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader) {
	authn := middleware.Authenticate(tokens, loader)
	read := middleware.RequirePermission(rbac.PermPlatformSettingsRead)
	write := middleware.RequirePermission(rbac.PermPlatformSettingsWrite)

	mux.Handle("GET /v1/platform/operations/summary", middleware.Chain(http.HandlerFunc(h.Summary), authn, read))
	mux.Handle("GET /v1/platform/operations/queue-tasks", middleware.Chain(http.HandlerFunc(h.QueueTasks), authn, read))
	mux.Handle("POST /v1/platform/operations/queue-tasks/{queue}/{id}/run", middleware.Chain(http.HandlerFunc(h.RunQueueTask), authn, write))
	mux.Handle("DELETE /v1/platform/operations/queue-tasks/{queue}/{id}", middleware.Chain(http.HandlerFunc(h.DeleteQueueTask), authn, write))
	mux.Handle("GET /v1/platform/operations/integration-jobs", middleware.Chain(http.HandlerFunc(h.IntegrationJobs), authn, read))
	mux.Handle("GET /v1/platform/operations/integration-jobs/{uuid}", middleware.Chain(http.HandlerFunc(h.IntegrationJob), authn, read))
	mux.Handle("POST /v1/platform/operations/integration-jobs/{uuid}/retry", middleware.Chain(http.HandlerFunc(h.RetryIntegrationJob), authn, write))
	mux.Handle("GET /v1/platform/operations/notice-failures", middleware.Chain(http.HandlerFunc(h.NoticeFailures), authn, read))
}

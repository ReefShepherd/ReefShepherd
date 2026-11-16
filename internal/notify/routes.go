package notify

import (
	"log/slog"
	"net/http"

	"github.com/reefshepherd/reefshepherd/internal/auth"
	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/settings"
)

// Deps are the dependencies the package's handlers, service and jobs need.
type Deps struct {
	DB       *db.DB
	Settings *settings.Store
	// Logger receives unexpected failures.
	Logger *slog.Logger
	// PublicURL is the external URL without a trailing slash, empty when unset.
	PublicURL string
	// HTTP is the client providers send with; nil means a client with a 10 s timeout.
	HTTP *http.Client
}

// Routes registers the notification endpoints behind the caller's sign-in check, where only an admin can manage channels.
func Routes(mux *http.ServeMux, deps Deps) {
	svc := NewService(deps)
	handle := func(h httpx.HandlerFunc) http.Handler { return httpx.Handle(svc.Logger, h, apiError) }
	mux.Handle("GET /api/v1/notification-channels", auth.RequireAdmin(handle(svc.listChannels)))
	mux.Handle("POST /api/v1/notification-channels", auth.RequireAdmin(handle(svc.createChannel)))
	mux.Handle("PATCH /api/v1/notification-channels/{id}", auth.RequireAdmin(handle(svc.patchChannel)))
	mux.Handle("DELETE /api/v1/notification-channels/{id}", auth.RequireAdmin(handle(svc.deleteChannel)))
	mux.Handle("POST /api/v1/notification-channels/{id}/test", auth.RequireAdmin(handle(svc.testChannel)))
	mux.Handle("GET /api/v1/notifications", handle(svc.listInbox))
	mux.Handle("GET /api/v1/notifications/unread-count", handle(svc.unreadCount))
	mux.Handle("POST /api/v1/notifications/read", handle(svc.markRead))
}

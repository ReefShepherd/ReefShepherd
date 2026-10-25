package auth

import (
	"context"
	"net/http"

	"github.com/reefshepherd/reefshepherd/internal/httpx"
)

// CodeSetupRequired is the error code protected routes answer with while first-run setup is pending.
const CodeSetupRequired = "setup_required"

// sessionInfo identifies the session a request arrived with; the zero value means none, as in disabled mode.
type sessionInfo struct {
	ID     int64
	Method string
}

type sessionKey struct{}

// SessionID returns the id of the session the request is signed in with, or false when the request has none.
func SessionID(ctx context.Context) (int64, bool) {
	s, ok := ctx.Value(sessionKey{}).(sessionInfo)
	return s.ID, ok && s.ID != 0
}

// RequireUser passes only requests from a signed-in user (or, in disabled mode, everyone) to next and puts the user in the context.
func (s *Service) RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, sess, ok, err := s.authenticate(w, r)
		if err != nil {
			httpx.Fail(s.Logger, w, r, err)
			return
		}
		if !ok {
			setup, err := s.SetupRequired(r.Context())
			if err != nil {
				httpx.Fail(s.Logger, w, r, err)
				return
			}
			if setup {
				httpx.WriteError(w, httpx.NewError(http.StatusConflict, CodeSetupRequired, "setup has not been completed"))
				return
			}
			httpx.WriteError(w, httpx.Unauthorized("sign in required"))
			return
		}
		ctx := httpx.WithUser(r.Context(), user)
		ctx = context.WithValue(ctx, sessionKey{}, sess)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireAdmin passes only admins to next; place it inside RequireUser.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := httpx.CurrentUser(r.Context())
		if !ok {
			httpx.WriteError(w, httpx.Unauthorized("sign in required"))
			return
		}
		if !u.IsAdmin() {
			httpx.WriteError(w, httpx.Forbidden("administrators only"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

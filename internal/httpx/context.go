package httpx

import "context"

// Roles a user can have.
const (
	RoleAdmin  = "admin"
	RoleMember = "member"
)

// User is the signed-in user as the HTTP layer sees them.
type User struct {
	ID          int64
	Username    string
	DisplayName string
	Role        string
}

// IsAdmin reports whether the user has the admin role.
func (u User) IsAdmin() bool {
	return u.Role == RoleAdmin
}

type userKey struct{}

// WithUser returns a context carrying u; the auth middleware calls it after validating a session.
func WithUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, userKey{}, u)
}

// CurrentUser returns the signed-in user, or false when the request is anonymous.
func CurrentUser(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(userKey{}).(User)
	return u, ok
}

// Package session provides session management utilities for the 3x-ui web panel.
// It handles user authentication state, login sessions, and session storage using Gin sessions.
package session

import (
	"encoding/gob"
	"net/http"

	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/logger"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

const (
	loginUserKey = "LOGIN_USER"
)

func init() {
	gob.Register(model.User{})
}

// SetLoginUser stores the authenticated user in the session and persists it.
// gin-contrib/sessions does not auto-save; callers that forget Save() leave
// the cookie out of sync with server state — this helper avoids that pitfall.
func SetLoginUser(c *gin.Context, user *model.User) error {
	if user == nil {
		return nil
	}
	s := sessions.Default(c)
	s.Set(loginUserKey, *user)
	return s.Save()
}

// GetLoginUser retrieves the authenticated user from the session.
// Returns nil if no user is logged in or if the session data is invalid.
func GetLoginUser(c *gin.Context) *model.User {
	s := sessions.Default(c)
	obj := s.Get(loginUserKey)
	if obj == nil {
		return nil
	}
	user, ok := obj.(model.User)
	if !ok {
		// Stale or incompatible session payload — wipe and persist immediately
		// so subsequent requests don't keep hitting the same broken cookie.
		s.Delete(loginUserKey)
		if err := s.Save(); err != nil {
			logger.Warning("session: failed to drop stale user payload:", err)
		}
		return nil
	}
	return &user
}

// IsLogin checks if a user is currently authenticated in the session.
func IsLogin(c *gin.Context) bool {
	return GetLoginUser(c) != nil
}

// HasRole reports whether the currently logged-in user has any of the given
// roles. If no user is logged in, returns false. Pre-RBAC sessions (legacy
// cookies issued before the RBAC migration where Role is empty) are treated
// as RoleSuperAdmin so existing single-admin deployments don't lose access
// after upgrade — the panel-level repair-on-startup migration ensures the
// underlying DB row already says super_admin.
func HasRole(c *gin.Context, roles ...string) bool {
	u := GetLoginUser(c)
	if u == nil {
		return false
	}
	role := u.Role
	if role == "" {
		role = model.RoleSuperAdmin
	}
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
}

// IsSuperAdmin is shorthand for HasRole(c, RoleSuperAdmin).
func IsSuperAdmin(c *gin.Context) bool {
	return HasRole(c, model.RoleSuperAdmin)
}

// CanWrite reports whether the logged-in user is allowed to perform write
// actions. RoleReadonly cannot; everyone else can (per-resource scoping for
// reseller is enforced separately at query time).
func CanWrite(c *gin.Context) bool {
	u := GetLoginUser(c)
	if u == nil {
		return false
	}
	role := u.Role
	if role == "" {
		role = model.RoleSuperAdmin
	}
	return role != model.RoleReadonly
}

// ClearSession invalidates the session and tells the browser to drop the cookie.
// The cookie attributes (Path/HttpOnly/SameSite) must mirror those used when
// the cookie was created or browsers will keep it.
func ClearSession(c *gin.Context) error {
	s := sessions.Default(c)
	s.Clear()
	cookiePath := c.GetString("base_path")
	if cookiePath == "" {
		cookiePath = "/"
	}
	s.Options(sessions.Options{
		Path:     cookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   c.Request.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})
	return s.Save()
}

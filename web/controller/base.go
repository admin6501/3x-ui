// Package controller provides HTTP request handlers and controllers for the 3x-ui web management panel.
// It handles routing, authentication, and API endpoints for managing Xray inbounds, settings, and more.
package controller

import (
	"net/http"
	"strings"

	"github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/mhsanaei/3x-ui/v2/web/locale"
	"github.com/mhsanaei/3x-ui/v2/web/session"

	"github.com/gin-gonic/gin"
)

// BaseController provides common functionality for all controllers, including authentication checks.
type BaseController struct{}

// checkLogin is a middleware that verifies user authentication and handles unauthorized access.
func (a *BaseController) checkLogin(c *gin.Context) {
	if !session.IsLogin(c) {
		if isAjax(c) {
			pureJsonMsg(c, http.StatusUnauthorized, false, I18nWeb(c, "pages.login.loginAgain"))
		} else {
			c.Redirect(http.StatusTemporaryRedirect, c.GetString("base_path"))
		}
		c.Abort()
	} else {
		c.Next()
	}
}

// requireRole returns a Gin middleware that aborts with 403 for any user
// whose RBAC role is not in the allowed set. Pre-RBAC sessions (no Role
// field) are treated as super_admin — see session.HasRole for rationale.
func requireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !session.IsLogin(c) {
			pureJsonMsg(c, http.StatusUnauthorized, false, I18nWeb(c, "pages.login.loginAgain"))
			c.Abort()
			return
		}
		if !session.HasRole(c, roles...) {
			pureJsonMsg(c, http.StatusForbidden, false, "forbidden: your role is not allowed for this action")
			c.Abort()
			return
		}
		c.Next()
	}
}

// requireSuperAdmin is a convenience wrapper for the most common gate.
func requireSuperAdmin() gin.HandlerFunc {
	return requireRole("super_admin")
}

// requireWriteAccess blocks readonly users from any mutating action.
func requireWriteAccess() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !session.IsLogin(c) {
			pureJsonMsg(c, http.StatusUnauthorized, false, I18nWeb(c, "pages.login.loginAgain"))
			c.Abort()
			return
		}
		if !session.CanWrite(c) {
			pureJsonMsg(c, http.StatusForbidden, false, "forbidden: read-only account")
			c.Abort()
			return
		}
		c.Next()
	}
}

// guardWriteMethods is the same as requireWriteAccess but only fires for
// mutating HTTP methods (POST/PUT/DELETE/PATCH). Read-only users can still
// hit GETs. Useful as a single g.Use(...) on a router group that mixes
// reads and writes (the existing inbound controller does that heavily).
//
// Note: a few endpoints (e.g. /onlines, /lastOnline, /clientIps) are POSTs
// but only return data — they're explicitly whitelisted so read-only
// admins can still browse the panel index. Whitelist matches by suffix
// of c.FullPath() so path parameters don't break the comparison.
func guardWriteMethods() gin.HandlerFunc {
	readLikePostSuffixes := []string{
		"/onlines",
		"/lastOnline",
		"/clientIps/:email",
		"/getClientTraffics/:email",
		"/getClientTrafficsById/:id",
		// Settings reads-via-POST (legacy convention in this panel):
		"/setting/defaultSettings", // public-ish bundle (subURI etc.)
		// Server status & config readers (panel index, info modals):
		"/api/server/status",
		"/api/server/getXrayVersion",
		"/api/server/getConfigJson",
		"/api/server/getNewX25519Cert",
		"/api/server/getNewVlessEnc",
		"/api/server/getNewEchCert",
		"/api/server/getNewmldsa65",
		"/api/server/getPanelUpdateInfo",
		"/api/custom-geo/list",
	}
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
			return
		}
		// Whitelist: POSTs that only fetch data must remain readable
		// for the readonly role; otherwise the panel index breaks.
		fp := c.FullPath()
		for _, sfx := range readLikePostSuffixes {
			if strings.HasSuffix(fp, sfx) {
				c.Next()
				return
			}
		}
		if !session.CanWrite(c) {
			pureJsonMsg(c, http.StatusForbidden, false, "forbidden: read-only account")
			c.Abort()
			return
		}
		c.Next()
	}
}

// I18nWeb retrieves an internationalized message for the web interface based on the current locale.
func I18nWeb(c *gin.Context, name string, params ...string) string {
	anyfunc, funcExists := c.Get("I18n")
	if !funcExists {
		logger.Warning("I18n function not exists in gin context!")
		return ""
	}
	i18nFunc, _ := anyfunc.(func(i18nType locale.I18nType, key string, keyParams ...string) string)
	msg := i18nFunc(locale.Web, name, params...)
	return msg
}

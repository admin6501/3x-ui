// Package controller — shared RBAC helpers used across HTTP handlers.
package controller

import (
	"net/http"

	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/web/service"
	"github.com/mhsanaei/3x-ui/v2/web/session"

	"github.com/gin-gonic/gin"
)

// resellerAllowedSet returns the set of inbound IDs the currently logged-in
// reseller is allowed to access. Returns ok=false for any non-reseller role
// (super_admin / manager / readonly) — those bypass the scope filter.
func resellerAllowedSet(c *gin.Context) (set map[int]struct{}, ok bool) {
	u := session.GetLoginUser(c)
	if u == nil || u.Role != model.RoleReseller {
		return nil, false
	}
	ids := service.AllowedInboundIDs(u)
	set = make(map[int]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set, true
}

// enforceInboundScope is a guard for endpoints that act on a specific
// inbound id. For resellers it 403s if the id is outside their allowed
// set; for everyone else it's a no-op. Returns true to continue handling.
func enforceInboundScope(c *gin.Context, inboundID int) bool {
	set, isReseller := resellerAllowedSet(c)
	if !isReseller {
		return true
	}
	if _, ok := set[inboundID]; ok {
		return true
	}
	pureJsonMsg(c, http.StatusForbidden, false, "forbidden: inbound is outside your reseller scope")
	c.Abort()
	return false
}

// enforceInboundScopeByEmail resolves the inbound that owns the client with
// the given email, then delegates to enforceInboundScope. Used to guard
// per-client endpoints whose URL only carries an email (not an inbound id)
// — e.g. clientIps, clearClientIps, updateClientTraffic, getClientTraffics.
//
// For super_admin / manager / readonly this is a no-op (it never queries
// the DB). For resellers a 404-shaped response is returned when the email
// has no client (so they can't probe for existence) and a 403 when the
// client exists but in an inbound outside their scope. Returns true to
// continue handling.
func enforceInboundScopeByEmail(c *gin.Context, email string) bool {
	if _, isReseller := resellerAllowedSet(c); !isReseller {
		return true
	}
	if email == "" {
		pureJsonMsg(c, http.StatusForbidden, false, "forbidden: empty email")
		c.Abort()
		return false
	}
	var svc service.InboundService
	_, inbound, err := svc.GetClientInboundByEmail(email)
	if err != nil || inbound == nil {
		// Treat "not found" identically to "out of scope" so resellers
		// cannot enumerate clients of other resellers via timing/error.
		pureJsonMsg(c, http.StatusForbidden, false, "forbidden: client is outside your reseller scope")
		c.Abort()
		return false
	}
	return enforceInboundScope(c, inbound.Id)
}

// enforceInboundScopeByClientUUID looks up which inbound owns the client
// with the given protocol-level identifier (UUID for vless/vmess, password
// for trojan, email for shadowsocks, auth for hysteria) and then runs the
// reseller scope check on that inbound. Used by getClientTrafficsById,
// which receives a UUID rather than a numeric inbound id — previously the
// route mounted the wrong middleware (scopeInboundParam) and silently
// no-op'd because strconv.Atoi failed on the UUID.
func enforceInboundScopeByClientUUID(c *gin.Context, clientUUID string) bool {
	if _, isReseller := resellerAllowedSet(c); !isReseller {
		return true
	}
	if clientUUID == "" {
		pureJsonMsg(c, http.StatusForbidden, false, "forbidden: empty client id")
		c.Abort()
		return false
	}
	var svc service.InboundService
	traffics, err := svc.GetClientTrafficByID(clientUUID)
	if err != nil || len(traffics) == 0 {
		pureJsonMsg(c, http.StatusForbidden, false, "forbidden: client is outside your reseller scope")
		c.Abort()
		return false
	}
	// All returned rows must be inside the reseller's scope — bail on the
	// first violation. In practice a UUID resolves to exactly one inbound,
	// but guard defensively in case of duplicated UUIDs across inbounds.
	for _, t := range traffics {
		if !enforceInboundScope(c, t.InboundId) {
			return false
		}
	}
	return true
}

// filterInboundsForRole returns only the inbounds the logged-in user is
// allowed to see. Super-admin / manager / readonly see everything; reseller
// sees only inbounds in their AllowedInbounds CSV.
func filterInboundsForRole(c *gin.Context, in []*model.Inbound) []*model.Inbound {
	set, isReseller := resellerAllowedSet(c)
	if !isReseller {
		return in
	}
	out := make([]*model.Inbound, 0, len(in))
	for _, ib := range in {
		if _, ok := set[ib.Id]; ok {
			out = append(out, ib)
		}
	}
	return out
}

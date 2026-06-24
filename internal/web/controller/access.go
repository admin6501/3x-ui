// Package controller — shared RBAC helpers used across HTTP handlers.
package controller

import (
	"net/http"
	"strconv"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/session"

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

// filterInboundOptionsForRole is the InboundOption-typed twin of
// filterInboundsForRole, used by the /options picker endpoint.
func filterInboundOptionsForRole(c *gin.Context, in []service.InboundOption) []service.InboundOption {
	set, isReseller := resellerAllowedSet(c)
	if !isReseller {
		return in
	}
	out := make([]service.InboundOption, 0, len(in))
	for _, o := range in {
		if _, ok := set[o.Id]; ok {
			out = append(out, o)
		}
	}
	return out
}

// scopeInboundParam is gin middleware that ensures the URL :id is in the
// reseller's allowed set. No-op for super_admin / manager / readonly.
func scopeInboundParam(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		// Not a valid id — let the downstream handler 400; the scope guard
		// has nothing to enforce.
		c.Next()
		return
	}
	if !enforceInboundScope(c, id) {
		return
	}
	c.Next()
}

// rejectReseller blocks resellers from panel-wide / create actions that
// cannot be scoped (add, import, bulkDel, resetAllTraffics, inbound
// edit/delete). No-op for every other role.
func rejectReseller(c *gin.Context) {
	if u := session.GetLoginUser(c); u != nil && u.Role == model.RoleReseller {
		pureJsonMsg(c, http.StatusForbidden, false, "forbidden: this action is not available to resellers")
		c.Abort()
		return
	}
	c.Next()
}

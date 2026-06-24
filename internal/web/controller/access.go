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

// resellerInboundSlice returns the reseller's allowed inbound IDs as a slice.
// ok=false for non-reseller roles (they bypass scoping). The slice may be
// empty for a reseller with no assigned inbounds (=> sees nothing).
func resellerInboundSlice(c *gin.Context) (ids []int, ok bool) {
	set, isReseller := resellerAllowedSet(c)
	if !isReseller {
		return nil, false
	}
	ids = make([]int, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	return ids, true
}

// filterClientsForRole keeps only the clients attached to at least one inbound
// the logged-in reseller is allowed to manage. No-op for other roles.
func filterClientsForRole(c *gin.Context, rows []service.ClientWithAttachments) []service.ClientWithAttachments {
	set, isReseller := resellerAllowedSet(c)
	if !isReseller {
		return rows
	}
	out := make([]service.ClientWithAttachments, 0, len(rows))
	for _, r := range rows {
		for _, id := range r.InboundIds {
			if _, ok := set[id]; ok {
				out = append(out, r)
				break
			}
		}
	}
	return out
}

// guardClientEmailScope aborts with 403 when the logged-in reseller tries to
// act on a client that is not attached to any of their allowed inbounds.
// No-op for non-reseller roles. Returns true to continue handling.
func guardClientEmailScope(c *gin.Context, svc *service.ClientService, email string) bool {
	set, isReseller := resellerAllowedSet(c)
	if !isReseller {
		return true
	}
	ids, err := svc.GetInboundIdsForEmail(nil, email)
	if err == nil {
		for _, id := range ids {
			if _, ok := set[id]; ok {
				return true
			}
		}
	}
	pureJsonMsg(c, http.StatusForbidden, false, "forbidden: client is outside your reseller scope")
	c.Abort()
	return false
}

// guardInboundIdsScope ensures every id is within the reseller's allowed set
// (and that at least one inbound was supplied). No-op for non-reseller roles.
func guardInboundIdsScope(c *gin.Context, inboundIds []int) bool {
	set, isReseller := resellerAllowedSet(c)
	if !isReseller {
		return true
	}
	if len(inboundIds) == 0 {
		pureJsonMsg(c, http.StatusForbidden, false, "forbidden: no inbound selected")
		c.Abort()
		return false
	}
	for _, id := range inboundIds {
		if _, ok := set[id]; !ok {
			pureJsonMsg(c, http.StatusForbidden, false, "forbidden: inbound is outside your reseller scope")
			c.Abort()
			return false
		}
	}
	return true
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

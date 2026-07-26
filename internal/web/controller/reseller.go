// Package controller — reseller self-service endpoints.
package controller

import (
	"net/http"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/session"

	"github.com/gin-gonic/gin"
)

// ResellerController exposes the logged-in reseller's own usage/quota snapshot
// under /panel/api/reseller. Accessible to any authenticated user; non-reseller
// roles get an empty/zeroed payload.
type ResellerController struct {
	adminService   service.AdminService
	inboundService service.InboundService
}

func NewResellerController(g *gin.RouterGroup) *ResellerController {
	a := &ResellerController{}
	g.GET("/me", a.me)
	g.GET("/overview", a.overview)
	return a
}

// overview backs the reseller dashboard: quota, client buckets, the inbounds
// they were given and the clients worth acting on, in one call. A non-reseller
// gets the same shape with empty lists rather than an error, so the page never
// has to special-case who is asking.
func (a *ResellerController) overview(c *gin.Context) {
	u := session.GetLoginUser(c)
	if u == nil {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	fresh, err := a.adminService.GetUserByID(u.Id)
	if err != nil {
		jsonMsg(c, "reseller overview", err)
		return
	}
	jsonObj(c, a.adminService.GetResellerOverview(fresh, &a.inboundService), nil)
}

type resellerMe struct {
	Username string `json:"username"`
	Role     string `json:"role"`
	service.ResellerStats
}

func (a *ResellerController) me(c *gin.Context) {
	u := session.GetLoginUser(c)
	if u == nil {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	// Reload fresh quota + counters from DB (session copy may predate the field).
	fresh, err := a.adminService.GetUserByID(u.Id)
	if err != nil {
		jsonMsg(c, "reseller me", err)
		return
	}
	out := resellerMe{Username: fresh.Username, Role: fresh.Role}
	if fresh.Role == model.RoleReseller {
		out.ResellerStats = a.adminService.GetResellerStats(fresh)
	}
	jsonObj(c, out, nil)
}

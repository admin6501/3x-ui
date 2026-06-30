// Package controller — admin RBAC HTTP handlers.
//
// All endpoints under /panel/api/admin require an authenticated super_admin
// (gated via requireSuperAdmin in api.go where the route group is mounted).
package controller

import (
	"net/http"
	"strconv"

	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/session"

	"github.com/gin-gonic/gin"
)

// AdminController exposes CRUD endpoints for admin user accounts and the
// audit log.
type AdminController struct {
	BaseController
	adminService   service.AdminService
	inboundService service.InboundService
	xrayService    service.XrayService
}

// NewAdminController wires the admin routes onto the given group.
func NewAdminController(g *gin.RouterGroup) *AdminController {
	a := &AdminController{}
	a.initRouter(g)
	return a
}

func (a *AdminController) initRouter(g *gin.RouterGroup) {
	g.GET("/list", a.list)
	g.POST("/add", a.add)
	g.POST("/update/:id", a.update)
	g.POST("/delete/:id", a.delete)
	g.POST("/resetPassword/:id", a.resetPassword)
	g.POST("/setEnabled/:id", a.setEnabled)
	g.POST("/resetResellerTraffic/:id", a.resetResellerTraffic)
	g.GET("/auditLog", a.auditLog)
	g.GET("/resellerStats", a.resellerStats)
}

func (a *AdminController) list(c *gin.Context) {
	rows, err := a.adminService.ListAdmins()
	if err != nil {
		jsonMsg(c, "list admins", err)
		return
	}
	jsonObj(c, rows, nil)
}

type adminCreateForm struct {
	Username        string `form:"username" json:"username"`
	Password        string `form:"password" json:"password"`
	Role            string `form:"role" json:"role"`
	AllowedInbounds string `form:"allowedInbounds" json:"allowedInbounds"`
	TrafficQuotaGB  int64  `form:"trafficQuotaGB" json:"trafficQuotaGB"`
	ClientQuota     int    `form:"clientQuota" json:"clientQuota"`
}

func (a *AdminController) add(c *gin.Context) {
	var f adminCreateForm
	if err := c.ShouldBind(&f); err != nil {
		jsonMsg(c, "create admin", err)
		return
	}
	actor := session.GetLoginUser(c)
	u, err := a.adminService.CreateAdmin(actor, f.Username, f.Password, f.Role, f.AllowedInbounds, f.TrafficQuotaGB, f.ClientQuota)
	if err != nil {
		jsonMsg(c, "create admin", err)
		return
	}
	// Don't echo the password hash back to the browser.
	u.Password = ""
	jsonObj(c, u, nil)
}

type adminUpdateForm struct {
	Username        string `form:"username" json:"username"`
	Role            string `form:"role" json:"role"`
	AllowedInbounds string `form:"allowedInbounds" json:"allowedInbounds"`
	TrafficQuotaGB  int64  `form:"trafficQuotaGB" json:"trafficQuotaGB"`
	ClientQuota     int    `form:"clientQuota" json:"clientQuota"`
}

func (a *AdminController) update(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	var f adminUpdateForm
	if err := c.ShouldBind(&f); err != nil {
		jsonMsg(c, "update admin", err)
		return
	}
	actor := session.GetLoginUser(c)
	if err := a.adminService.UpdateAdmin(actor, id, f.Username, f.Role, f.AllowedInbounds, f.TrafficQuotaGB, f.ClientQuota); err != nil {
		jsonMsg(c, "update admin", err)
		return
	}
	jsonMsg(c, "update admin", nil)
}

func (a *AdminController) delete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	actor := session.GetLoginUser(c)
	if err := a.adminService.DeleteAdmin(actor, id); err != nil {
		jsonMsg(c, "delete admin", err)
		return
	}
	jsonMsg(c, "delete admin", nil)
}

type resetPasswordForm struct {
	Password string `form:"password" json:"password"`
}

func (a *AdminController) resetPassword(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	var f resetPasswordForm
	if err := c.ShouldBind(&f); err != nil {
		jsonMsg(c, "reset password", err)
		return
	}
	actor := session.GetLoginUser(c)
	if err := a.adminService.ResetAdminPassword(actor, id, f.Password); err != nil {
		jsonMsg(c, "reset password", err)
		return
	}
	jsonMsg(c, "reset password", nil)
}

type setEnabledForm struct {
	Enabled bool `form:"enabled" json:"enabled"`
}

// setEnabled toggles whether an admin account (any role) can log in.
func (a *AdminController) setEnabled(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	var f setEnabledForm
	if err := c.ShouldBind(&f); err != nil {
		jsonMsg(c, "set enabled", err)
		return
	}
	actor := session.GetLoginUser(c)
	if err := a.adminService.SetAdminEnabled(actor, id, f.Enabled); err != nil {
		jsonMsg(c, "set enabled", err)
		return
	}
	jsonMsg(c, "set enabled", nil)
}

// resetResellerTraffic zeroes a reseller's consumed traffic and re-enables any
// inbounds the quota enforcer auto-disabled for them.
func (a *AdminController) resetResellerTraffic(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	actor := session.GetLoginUser(c)
	if err := a.adminService.ResetResellerTraffic(actor, id, &a.inboundService, &a.xrayService); err != nil {
		jsonMsg(c, "reset reseller traffic", err)
		return
	}
	jsonMsg(c, "reset reseller traffic", nil)
}

func (a *AdminController) auditLog(c *gin.Context) {
	limit := 200
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}
	rows, err := a.adminService.ListAuditLogs(limit)
	if err != nil {
		jsonMsg(c, "list audit logs", err)
		return
	}
	jsonObj(c, rows, nil)
}

// resellerStats returns a map of reseller user id -> aggregated usage stats
// (total traffic across all assigned inbounds, current + cumulative client
// counts, and quota caps). super_admin only.
func (a *AdminController) resellerStats(c *gin.Context) {
	stats, err := a.adminService.GetAllResellerStats()
	if err != nil {
		jsonMsg(c, "reseller stats", err)
		return
	}
	jsonObj(c, stats, nil)
}

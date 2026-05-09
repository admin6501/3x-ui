package controller

import (
	"github.com/mhsanaei/3x-ui/v2/web/middleware"

	"github.com/gin-gonic/gin"
)

// XUIController is the main controller for the X-UI panel, managing sub-controllers.
type XUIController struct {
	BaseController

	settingController     *SettingController
	xraySettingController *XraySettingController
	adminController       *AdminController
}

// NewXUIController creates a new XUIController and initializes its routes.
func NewXUIController(g *gin.RouterGroup) *XUIController {
	a := &XUIController{}
	a.initRouter(g)
	return a
}

// initRouter sets up the main panel routes and initializes sub-controllers.
func (a *XUIController) initRouter(g *gin.RouterGroup) {
	g = g.Group("/panel")
	g.Use(a.checkLogin)
	g.Use(middleware.CSRFMiddleware())
	// Read-only accounts can browse all GETs but cannot trigger any
	// mutating endpoint under /panel. Applied here (before sub-controllers
	// register their handlers) so it catches every nested route.
	g.Use(guardWriteMethods())

	g.GET("/", a.index)
	g.GET("/inbounds", a.inbounds)
	g.GET("/settings", requireSuperAdmin(), a.settings)
	g.GET("/xray", requireSuperAdmin(), a.xraySettings)
	g.GET("/admins", requireSuperAdmin(), a.admins)

	// SettingController exposes a mix of read-only (defaultSettings,
	// updateUser-self) and super_admin-only endpoints. Per-route gating
	// is applied inside SettingController.initRouter so every role can
	// load the panel index without 403'ing on the public-ish settings
	// bundle (which carries the subscription URI etc.).
	a.settingController = NewSettingController(g)

	// XraySettingController is fully panel-wide config — gated as a unit.
	xraySettingGroup := g.Group("")
	xraySettingGroup.Use(requireSuperAdmin())
	a.xraySettingController = NewXraySettingController(xraySettingGroup)

	// Admin RBAC endpoints — gated to super_admin only.
	adminGroup := g.Group("/admin")
	adminGroup.Use(requireSuperAdmin())
	a.adminController = NewAdminController(adminGroup)
}

// index renders the main panel index page.
func (a *XUIController) index(c *gin.Context) {
	html(c, "index.html", "pages.index.title", nil)
}

// inbounds renders the inbounds management page.
func (a *XUIController) inbounds(c *gin.Context) {
	html(c, "inbounds.html", "pages.inbounds.title", nil)
}

// settings renders the settings management page.
func (a *XUIController) settings(c *gin.Context) {
	html(c, "settings.html", "pages.settings.title", nil)
}

// xraySettings renders the Xray settings page.
func (a *XUIController) xraySettings(c *gin.Context) {
	html(c, "xray.html", "pages.xray.title", nil)
}

// admins renders the Admin Management page (super_admin only).
func (a *XUIController) admins(c *gin.Context) {
	html(c, "admins.html", "pages.admins.title", nil)
}

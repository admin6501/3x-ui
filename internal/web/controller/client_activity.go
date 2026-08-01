// Package controller — per-client activity endpoints.
package controller

import (
	"strconv"

	"github.com/mhsanaei/3x-ui/v3/internal/web/service"

	"github.com/gin-gonic/gin"
)

// ClientActivityController serves the Client Activity page: per-client
// connecting IPs, the operator behind each, and visited destinations, plus the
// action that wipes the collected data. Every route is gated to settings.manage
// by the caller — this is the browsing record of proxy users, so it is not
// exposed to managers or resellers, and it reports empty while tracking is off.
type ClientActivityController struct {
	activityService service.ClientActivityService
}

func NewClientActivityController(g *gin.RouterGroup) *ClientActivityController {
	a := &ClientActivityController{}
	g.GET("/status", a.status)
	g.GET("/list", a.list)
	g.GET("/detail/:email", a.detail)
	g.POST("/clear", a.clear)
	return a
}

// statusPayload tells the page whether tracking is on before it renders a table,
// so a disabled feature shows the "enable it in settings" state rather than an
// empty grid that looks broken.
type statusPayload struct {
	Enabled bool `json:"enabled"`
}

func (a *ClientActivityController) status(c *gin.Context) {
	jsonObj(c, statusPayload{Enabled: a.activityService.Enabled()}, nil)
}

func (a *ClientActivityController) list(c *gin.Context) {
	rows, err := a.activityService.ListSummaries()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.clientActivity.toasts.loadError"), err)
		return
	}
	jsonObj(c, gin.H{"enabled": a.activityService.Enabled(), "clients": rows}, nil)
}

func (a *ClientActivityController) detail(c *gin.Context) {
	email := c.Param("email")
	limit, _ := strconv.Atoi(c.Query("limit"))
	detail, err := a.activityService.Detail(email, limit)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.clientActivity.toasts.loadError"), err)
		return
	}
	jsonObj(c, detail, nil)
}

func (a *ClientActivityController) clear(c *gin.Context) {
	if err := a.activityService.Clear(); err != nil {
		jsonMsg(c, I18nWeb(c, "pages.clientActivity.toasts.clearError"), err)
		return
	}
	jsonMsg(c, I18nWeb(c, "pages.clientActivity.toasts.cleared"), nil)
}

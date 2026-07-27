// Package controller — reseller sales HTTP handlers.
//
// Everything under /panel/api/sales requires the admins.manage permission
// (gated in api.go where the route group is mounted): selling a reseller
// account is creating an admin account, so it is protected the same way.
package controller

import (
	"strconv"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/session"

	"github.com/gin-gonic/gin"
)

// SalesController exposes the reseller price list and the orders placed
// against it, so a shop can be run from the panel as well as from the bot.
type SalesController struct {
	BaseController
	salesService   service.SalesService
	inboundService service.InboundService
	xrayService    service.XrayService
}

func NewSalesController(g *gin.RouterGroup) *SalesController {
	a := &SalesController{}
	a.initRouter(g)
	return a
}

func (a *SalesController) initRouter(g *gin.RouterGroup) {
	g.GET("/packages", a.listPackages)
	g.POST("/packages/add", a.savePackage)
	g.POST("/packages/update/:id", a.savePackage)
	g.POST("/packages/del/:id", a.deletePackage)
	g.GET("/orders", a.listOrders)
	g.POST("/orders/add", a.createOrder)
	g.POST("/orders/approve/:id", a.approveOrder)
	g.POST("/orders/reject/:id", a.rejectOrder)
	g.GET("/stats", a.stats)
}

func (a *SalesController) listPackages(c *gin.Context) {
	rows, err := a.salesService.ListPackages()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, rows, nil)
}

type packageForm struct {
	Name            string `json:"name" form:"name"`
	Description     string `json:"description" form:"description"`
	Price           int64  `json:"price" form:"price"`
	TrafficGB       int64  `json:"trafficGB" form:"trafficGB"`
	ClientQuota     int    `json:"clientQuota" form:"clientQuota"`
	DurationDays    int    `json:"durationDays" form:"durationDays"`
	AllowedInbounds string `json:"allowedInbounds" form:"allowedInbounds"`
	Enable          bool   `json:"enable" form:"enable"`
	SortOrder       int    `json:"sortOrder" form:"sortOrder"`
}

// savePackage backs both add and update: the id in the path decides which.
func (a *SalesController) savePackage(c *gin.Context) {
	var f packageForm
	if err := c.ShouldBind(&f); err != nil {
		jsonMsg(c, I18nWeb(c, "pages.sales.saveFailed"), err)
		return
	}
	pkg := &model.ResellerPackage{
		Name:            f.Name,
		Description:     f.Description,
		Price:           f.Price,
		TrafficGB:       f.TrafficGB,
		ClientQuota:     f.ClientQuota,
		DurationDays:    f.DurationDays,
		AllowedInbounds: f.AllowedInbounds,
		Enable:          f.Enable,
		SortOrder:       f.SortOrder,
	}
	if raw := c.Param("id"); raw != "" {
		id, err := strconv.Atoi(raw)
		if err != nil {
			jsonMsg(c, I18nWeb(c, "pages.sales.saveFailed"), err)
			return
		}
		pkg.Id = id
	}
	if err := a.salesService.SavePackage(pkg); err != nil {
		jsonMsg(c, I18nWeb(c, "pages.sales.saveFailed"), err)
		return
	}
	jsonObj(c, pkg, nil)
}

func (a *SalesController) deletePackage(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.sales.deleteFailed"), err)
		return
	}
	if err := a.salesService.DeletePackage(id); err != nil {
		jsonMsg(c, I18nWeb(c, "pages.sales.deleteFailed"), err)
		return
	}
	jsonObj(c, nil, nil)
}

func (a *SalesController) listOrders(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	rows, err := a.salesService.ListOrders(c.Query("status"), limit)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, rows, nil)
}

// createOrder records a sale made outside Telegram — cash, a bank transfer the
// admin verified themselves — so it can be approved through the same path and
// show up in the same figures as a bot sale.
func (a *SalesController) createOrder(c *gin.Context) {
	var f struct {
		TelegramId   int64  `json:"telegramId" form:"telegramId"`
		TelegramName string `json:"telegramName" form:"telegramName"`
		PackageId    int    `json:"packageId" form:"packageId"`
		Kind         string `json:"kind" form:"kind"`
	}
	if err := c.ShouldBind(&f); err != nil {
		jsonMsg(c, I18nWeb(c, "pages.sales.saveFailed"), err)
		return
	}
	order, err := a.salesService.CreateOrder(f.TelegramId, f.TelegramName, f.PackageId, f.Kind)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.sales.saveFailed"), err)
		return
	}
	jsonObj(c, order, nil)
}

// approveOrder creates or tops up the buyer's reseller account. Unlike the bot,
// the panel can approve an order that carries no receipt — that is how an
// out-of-band sale (cash, bank transfer the admin verified themselves) is
// recorded.
func (a *SalesController) approveOrder(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.sales.approveFailed"), err)
		return
	}
	actor := session.GetLoginUser(c)
	result, err := a.salesService.ApproveOrder(actor, id, false, &a.inboundService, &a.xrayService)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.sales.approveFailed"), err)
		return
	}
	jsonObj(c, map[string]any{
		"username":    result.Username,
		"password":    result.Password,
		"isNew":       result.IsNew,
		"trafficGB":   result.TrafficGB,
		"clientQuota": result.ClientQuota,
	}, nil)
}

func (a *SalesController) rejectOrder(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.sales.rejectFailed"), err)
		return
	}
	var body struct {
		Note string `json:"note" form:"note"`
	}
	_ = c.ShouldBind(&body)
	if _, err := a.salesService.RejectOrder(id, body.Note); err != nil {
		jsonMsg(c, I18nWeb(c, "pages.sales.rejectFailed"), err)
		return
	}
	jsonObj(c, nil, nil)
}

func (a *SalesController) stats(c *gin.Context) {
	jsonObj(c, a.salesService.Stats(), nil)
}

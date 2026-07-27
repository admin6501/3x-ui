// Package controller — Telegram shop HTTP handlers.
//
// Everything under /panel/api/shop requires the admins.manage permission
// (gated in api.go where the route group is mounted): the shop moves money and
// creates access, so it is protected like admin management.
package controller

import (
	"strconv"

	"github.com/mhsanaei/3x-ui/v3/internal/web/service"

	"github.com/gin-gonic/gin"
)

// ShopController exposes the Telegram shop's wallets, top-up requests and the
// configs bought with them, so a shop can be run from the panel as well as
// from the bot.
type ShopController struct {
	BaseController
	shopService    service.ShopService
	inboundService service.InboundService
}

func NewShopController(g *gin.RouterGroup) *ShopController {
	a := &ShopController{}
	a.initRouter(g)
	return a
}

func (a *ShopController) initRouter(g *gin.RouterGroup) {
	g.GET("/users", a.shopUsers)
	g.POST("/users/:id/adjust", a.shopAdjust)
	g.POST("/users/:id/block", a.shopBlock)
	g.GET("/topups", a.shopTopUps)
	g.POST("/topups/approve/:id", a.shopApproveTopUp)
	g.POST("/topups/reject/:id", a.shopRejectTopUp)
	g.GET("/configs", a.shopConfigs)
	g.POST("/configs/del/:id", a.shopDeleteConfig)
	g.GET("/stats", a.shopStats)
	g.POST("/bill", a.shopBillNow)
}

func (a *ShopController) shopUsers(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "200"))
	rows, err := a.shopService.ListUsers(limit)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, rows, nil)
}

// shopAdjust credits or debits a wallet by hand. The sign of the amount is the
// direction, so one endpoint covers a refund and a correction alike.
func (a *ShopController) shopAdjust(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.shop.adjustFailed"), err)
		return
	}
	var body struct {
		Amount  int64  `json:"amount" form:"amount"`
		Details string `json:"details" form:"details"`
	}
	if err := c.ShouldBind(&body); err != nil {
		jsonMsg(c, I18nWeb(c, "pages.shop.adjustFailed"), err)
		return
	}
	balance, err := a.shopService.Adjust(id, body.Amount, body.Details)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.shop.adjustFailed"), err)
		return
	}
	// A correction in either direction can switch the user's configs on or off.
	a.shopService.BillAll(&a.inboundService)
	jsonObj(c, map[string]any{"balance": balance}, nil)
}

func (a *ShopController) shopBlock(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	var body struct {
		Blocked bool `json:"blocked" form:"blocked"`
	}
	_ = c.ShouldBind(&body)
	if err := a.shopService.SetBlocked(id, body.Blocked); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	a.shopService.BillAll(&a.inboundService)
	jsonObj(c, nil, nil)
}

func (a *ShopController) shopTopUps(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	rows, err := a.shopService.ListTopUps(c.Query("status"), limit)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, rows, nil)
}

func (a *ShopController) shopApproveTopUp(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.shop.approveFailed"), err)
		return
	}
	row, balance, err := a.shopService.ApproveTopUp(id)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.shop.approveFailed"), err)
		return
	}
	// Paying puts the user's configs back on without waiting for the next tick.
	a.shopService.BillAll(&a.inboundService)
	jsonObj(c, map[string]any{"telegramId": row.TelegramId, "amount": row.Amount, "balance": balance}, nil)
}

func (a *ShopController) shopRejectTopUp(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.shop.rejectFailed"), err)
		return
	}
	var body struct {
		Note string `json:"note" form:"note"`
	}
	_ = c.ShouldBind(&body)
	if _, err := a.shopService.RejectTopUp(id, body.Note); err != nil {
		jsonMsg(c, I18nWeb(c, "pages.shop.rejectFailed"), err)
		return
	}
	jsonObj(c, nil, nil)
}

// shopConfigs lists every config the shop sold, with its live meter reading and
// what it has cost so far.
func (a *ShopController) shopConfigs(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "200"))
	rows, err := a.shopService.ListAllConfigs(limit)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	out := make([]service.ConfigUsage, 0, len(rows))
	for i := range rows {
		out = append(out, a.shopService.Usage(&rows[i]))
	}
	jsonObj(c, out, nil)
}

func (a *ShopController) shopDeleteConfig(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.shop.deleteFailed"), err)
		return
	}
	if err := a.shopService.DeleteConfig(&a.inboundService, id); err != nil {
		jsonMsg(c, I18nWeb(c, "pages.shop.deleteFailed"), err)
		return
	}
	jsonObj(c, nil, nil)
}

func (a *ShopController) shopStats(c *gin.Context) {
	jsonObj(c, a.shopService.Stats(), nil)
}

// shopBillNow runs the metering on demand, so an admin can see the effect of a
// price change without waiting for the next scheduled run.
func (a *ShopController) shopBillNow(c *gin.Context) {
	result := a.shopService.BillAll(&a.inboundService)
	jsonObj(c, map[string]any{"charged": result.Charged, "wallets": result.ChargedUsers}, nil)
}

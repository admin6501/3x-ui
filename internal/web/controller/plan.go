package controller

import (
	"strconv"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"

	"github.com/gin-gonic/gin"
)

// PlanController exposes CRUD for reusable client plan/package templates under
// /panel/api/plans. Listing is available to any authenticated admin (so the
// client form can offer plans, including for resellers); mutating routes are
// restricted to super_admin / manager (readonly is blocked globally, reseller
// via rejectReseller).
type PlanController struct {
	planService service.PlanService
}

func NewPlanController(g *gin.RouterGroup) *PlanController {
	a := &PlanController{}
	a.initRouter(g)
	return a
}

func (a *PlanController) initRouter(g *gin.RouterGroup) {
	g.GET("/list", a.list)
	g.POST("/add", rejectReseller, a.create)
	g.POST("/update/:id", rejectReseller, a.update)
	g.POST("/del/:id", rejectReseller, a.delete)
}

func (a *PlanController) list(c *gin.Context) {
	rows, err := a.planService.List()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, rows, nil)
}

func (a *PlanController) create(c *gin.Context) {
	var p model.Plan
	if err := c.ShouldBindJSON(&p); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	if err := a.planService.Create(&p); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, p, nil)
}

func (a *PlanController) update(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	var p model.Plan
	if err := c.ShouldBindJSON(&p); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	if err := a.planService.Update(id, &p); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, gin.H{"id": id}, nil)
}

func (a *PlanController) delete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	if err := a.planService.Delete(id); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, gin.H{"id": id}, nil)
}

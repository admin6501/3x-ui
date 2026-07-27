// Package controller — custom admin role HTTP handlers.
//
// All endpoints under /panel/api/roles require the admins.manage permission
// (gated in api.go where the route group is mounted), the same gate that
// protects admin account management itself.
package controller

import (
	"strconv"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"

	"github.com/gin-gonic/gin"
)

// RoleController exposes CRUD for admin-defined roles.
type RoleController struct {
	BaseController
	roleService service.AdminRoleService
}

// NewRoleController wires the role routes onto the given group.
func NewRoleController(g *gin.RouterGroup) *RoleController {
	a := &RoleController{}
	a.initRouter(g)
	return a
}

func (a *RoleController) initRouter(g *gin.RouterGroup) {
	g.GET("/list", a.list)
	g.GET("/permissions", a.permissions)
	g.POST("/add", a.add)
	g.POST("/update/:id", a.update)
	g.POST("/del/:id", a.delete)
}

// roleForm accepts permissions either as a JSON array or as a CSV string, so
// the same endpoint works from the SPA and from a plain form post.
type roleForm struct {
	Name           string   `form:"name" json:"name"`
	Permissions    []string `form:"permissions" json:"permissions"`
	PermissionsCSV string   `form:"permissionsCsv" json:"permissionsCsv"`
}

func (f roleForm) perms() []string {
	if len(f.Permissions) > 0 {
		return f.Permissions
	}
	if strings.TrimSpace(f.PermissionsCSV) == "" {
		return nil
	}
	return strings.Split(f.PermissionsCSV, ",")
}

func (a *RoleController) list(c *gin.Context) {
	rows, err := a.roleService.List()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, rows, nil)
}

// permissions returns the closed set of permission keys a role can be built
// from, so the UI never has to hard-code them.
func (a *RoleController) permissions(c *gin.Context) {
	jsonObj(c, model.AllPermissions, nil)
}

func (a *RoleController) add(c *gin.Context) {
	var f roleForm
	if err := c.ShouldBind(&f); err != nil {
		jsonMsg(c, I18nWeb(c, "pages.admins.roleSaveFailed"), err)
		return
	}
	row, err := a.roleService.Create(f.Name, f.perms())
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.admins.roleSaveFailed"), err)
		return
	}
	jsonObj(c, row, nil)
}

func (a *RoleController) update(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.admins.roleSaveFailed"), err)
		return
	}
	var f roleForm
	if err := c.ShouldBind(&f); err != nil {
		jsonMsg(c, I18nWeb(c, "pages.admins.roleSaveFailed"), err)
		return
	}
	row, err := a.roleService.Update(id, f.Name, f.perms())
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.admins.roleSaveFailed"), err)
		return
	}
	jsonObj(c, row, nil)
}

func (a *RoleController) delete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.admins.roleDeleteFailed"), err)
		return
	}
	if err := a.roleService.Delete(id); err != nil {
		jsonMsg(c, I18nWeb(c, "pages.admins.roleDeleteFailed"), err)
		return
	}
	jsonObj(c, nil, nil)
}

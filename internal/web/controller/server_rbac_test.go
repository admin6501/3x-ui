package controller

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/session"
)

// newServerRBACEngine mounts the real ServerController route table behind the
// same chain APIController.initRouter puts in front of it, with the given role
// logged in. initRouter is called directly so the assertions run against the
// production registration — a gate dropped from a route fails these tests.
func newServerRBACEngine(t *testing.T, role string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	if err := database.InitDB(filepath.Join(dbDir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })

	user := &model.User{Username: "rbac-" + role, Password: "x", Role: role}
	if err := database.GetDB().Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	engine := gin.New()
	store := cookie.NewStore([]byte("server-rbac-test-secret"))
	engine.Use(sessions.Sessions("3x-ui", store))
	engine.Use(func(c *gin.Context) {
		if err := session.SetLoginUser(c, user); err != nil {
			t.Errorf("SetLoginUser: %v", err)
		}
		c.Next()
	})

	api := engine.Group("/panel/api")
	api.Use((&APIController{}).checkAPIAuth)
	api.Use(guardWriteMethods())
	(&ServerController{}).initRouter(api.Group("/server"))
	return engine
}

// gatedServerRoutes are the endpoints that expose panel-wide state or perform a
// panel-wide operation. None of them may be reachable by a role that lacks the
// matching permission — the database in particular carries admin password
// hashes, the session secret and every node API token.
var gatedServerRoutes = []struct {
	method string
	path   string
}{
	{http.MethodGet, "/panel/api/server/getDb"},
	{http.MethodGet, "/panel/api/server/getMigration"},
	{http.MethodGet, "/panel/api/server/getConfigJson"},
	{http.MethodGet, "/panel/api/server/clientIps"},
	{http.MethodPost, "/panel/api/server/importDB"},
	{http.MethodPost, "/panel/api/server/updatePanel"},
	{http.MethodPost, "/panel/api/server/stopXrayService"},
	{http.MethodPost, "/panel/api/server/restartXrayService"},
	{http.MethodPost, "/panel/api/server/installXray/v1.8.0"},
	{http.MethodPost, "/panel/api/server/updateGeofile"},
	{http.MethodPost, "/panel/api/server/logs/20"},
	{http.MethodPost, "/panel/api/server/xraylogs/20"},
	{http.MethodPost, "/panel/api/server/clientIps"},
}

func TestServerRoutes_ForbiddenForLowPrivilegedRoles(t *testing.T) {
	for _, role := range []string{model.RoleReadonly, model.RoleReseller, model.RoleManager} {
		t.Run(role, func(t *testing.T) {
			engine := newServerRBACEngine(t, role)
			for _, rt := range gatedServerRoutes {
				rec := httptest.NewRecorder()
				req := httptest.NewRequest(rt.method, rt.path, nil)
				req.Header.Set("X-Requested-With", "XMLHttpRequest")
				engine.ServeHTTP(rec, req)
				if rec.Code != http.StatusForbidden {
					t.Errorf("%s %s: got %d, want 403 for role %s (body: %s)",
						rt.method, rt.path, rec.Code, role, rec.Body.String())
				}
			}
		})
	}
}

// TestServerRoutes_SuperAdminPassesTheGate pins the other half of the contract:
// the gates reject by permission, not by blocking the routes outright.
func TestServerRoutes_SuperAdminPassesTheGate(t *testing.T) {
	engine := newServerRBACEngine(t, model.RoleSuperAdmin)
	for _, rt := range []struct{ method, path string }{
		{http.MethodGet, "/panel/api/server/getConfigJson"},
		{http.MethodGet, "/panel/api/server/clientIps"},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(rt.method, rt.path, nil)
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		engine.ServeHTTP(rec, req)
		if rec.Code == http.StatusForbidden {
			t.Errorf("%s %s: super_admin was refused (body: %s)", rt.method, rt.path, rec.Body.String())
		}
	}
}

// TestServerRoutes_ObservationalEndpointsStayOpen keeps the gating from
// overreaching: panel health and fresh key material disclose no stored secret
// and every admin role needs them.
func TestServerRoutes_ObservationalEndpointsStayOpen(t *testing.T) {
	engine := newServerRBACEngine(t, model.RoleReadonly)
	for _, path := range []string{
		"/panel/api/server/getNewUUID",
		"/panel/api/server/getPanelUpdateInfo",
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		engine.ServeHTTP(rec, req)
		if rec.Code == http.StatusForbidden {
			t.Errorf("GET %s: readonly was refused an observational endpoint", path)
		}
	}
}

// TestServerRoutes_ApiTokenKeepsFullPrivilege pins the node-to-node contract:
// a parent panel drives its nodes with the node's API token, and those calls
// land on permission-gated endpoints. The token must not inherit the role of
// whatever account happens to be first in the table — here a readonly one —
// or a demoted account #1 would silently break the fleet.
func TestServerRoutes_ApiTokenKeepsFullPrivilege(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	if err := database.InitDB(filepath.Join(dbDir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })

	// Demote every existing account, so the first row is deliberately not a
	// super_admin. The token must still carry full privilege.
	if err := database.GetDB().Model(model.User{}).Where("1 = 1").
		Update("role", model.RoleReadonly).Error; err != nil {
		t.Fatalf("demote first user: %v", err)
	}

	a := &APIController{}
	token, err := a.apiTokenService.Create("node-link")
	if err != nil {
		t.Fatalf("create api token: %v", err)
	}

	engine := gin.New()
	store := cookie.NewStore([]byte("api-token-privilege-test"))
	engine.Use(sessions.Sessions("3x-ui", store))
	api := engine.Group("/panel/api")
	api.Use(a.checkAPIAuth)
	api.Use(guardWriteMethods())
	(&ServerController{}).initRouter(api.Group("/server"))

	// Read-only representatives of each gate, so the assertion is about the
	// gate and not about a handler that wants a live xray process.
	for _, rt := range []struct{ method, path string }{
		{http.MethodGet, "/panel/api/server/getConfigJson"}, // xray.manage
		{http.MethodGet, "/panel/api/server/clientIps"},     // settings.manage
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(rt.method, rt.path, nil)
		req.Header.Set("Authorization", "Bearer "+token.Token)
		engine.ServeHTTP(rec, req)
		if rec.Code == http.StatusForbidden {
			t.Errorf("%s %s: API token was refused (body: %s)", rt.method, rt.path, rec.Body.String())
		}
	}
}

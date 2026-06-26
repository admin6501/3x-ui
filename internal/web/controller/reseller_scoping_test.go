package controller

import (
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/session"

	"github.com/gin-gonic/gin"
)

func setupCtrlDB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dir)
	if err := database.InitDB(filepath.Join(dir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
}

func ctxForUser(u *model.User) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	session.SetAPIAuthUser(c, u)
	return c
}

// TestResellerEmailSetScoping ensures the online/summary scoping helper only
// returns clients attached to the reseller's assigned inbounds, and is a no-op
// (ok=false) for non-reseller roles.
func TestResellerEmailSetScoping(t *testing.T) {
	setupCtrlDB(t)
	db := database.GetDB()
	ctrl := &ClientController{}

	// Two inbounds; reseller is assigned only to #1.
	db.Create(&model.Inbound{Id: 1, Protocol: model.VLESS, Remark: "A"})
	db.Create(&model.Inbound{Id: 2, Protocol: model.VLESS, Remark: "B"})

	link := func(email string, inboundID int) {
		rec := &model.ClientRecord{Email: email, UUID: email}
		if err := db.Create(rec).Error; err != nil {
			t.Fatalf("create client %s: %v", email, err)
		}
		if err := db.Create(&model.ClientInbound{ClientId: rec.Id, InboundId: inboundID}).Error; err != nil {
			t.Fatalf("link %s: %v", email, err)
		}
	}
	link("alice", 1) // in scope
	link("bob", 2)   // out of scope

	reseller := &model.User{Id: 50, Username: "res", Role: model.RoleReseller, AllowedInbounds: "1"}
	set, ok := ctrl.resellerEmailSet(ctxForUser(reseller))
	if !ok {
		t.Fatal("expected ok=true for reseller")
	}
	if _, in := set["alice"]; !in {
		t.Error("alice (inbound #1) should be in reseller scope")
	}
	if _, in := set["bob"]; in {
		t.Error("bob (inbound #2) must NOT be in reseller scope")
	}

	// Super-admin bypasses scoping entirely.
	admin := &model.User{Id: 1, Username: "admin", Role: model.RoleSuperAdmin}
	if _, ok := ctrl.resellerEmailSet(ctxForUser(admin)); ok {
		t.Error("expected ok=false (no scoping) for super_admin")
	}
}

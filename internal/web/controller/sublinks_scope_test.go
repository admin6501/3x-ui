package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/session"

	"github.com/gin-gonic/gin"
)

// seedSubIdFixture creates two inbounds with one client each, addressed by
// subscription id, and returns nothing but the ids used below.
func seedSubIdFixture(t *testing.T) {
	t.Helper()
	db := database.GetDB()
	db.Create(&model.Inbound{Id: 1, Protocol: model.VLESS, Remark: "mine"})
	db.Create(&model.Inbound{Id: 2, Protocol: model.VLESS, Remark: "theirs"})

	link := func(email, subId string, inboundID int) {
		rec := &model.ClientRecord{Email: email, UUID: email, SubID: subId}
		if err := db.Create(rec).Error; err != nil {
			t.Fatalf("create client %s: %v", email, err)
		}
		if err := db.Create(&model.ClientInbound{ClientId: rec.Id, InboundId: inboundID}).Error; err != nil {
			t.Fatalf("link %s: %v", email, err)
		}
	}
	link("alice", "sub-in-scope", 1)
	link("bob", "sub-out-of-scope", 2)
}

// ctxForSubId builds a request context addressing :subId as the given user.
func ctxForSubId(u *model.User, subId string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/panel/api/clients/subLinks/"+subId, nil)
	c.Params = gin.Params{{Key: "subId", Value: subId}}
	session.SetAPIAuthUser(c, u)
	return c, rec
}

// TestSubLinksScope_ResellerRefusedOutsideScope is the regression guard for the
// one client route that had no scope check. The links behind a subId embed the
// client's UUID/password, so reaching another reseller's subId hands over
// working credentials.
func TestSubLinksScope_ResellerRefusedOutsideScope(t *testing.T) {
	setupCtrlDB(t)
	seedSubIdFixture(t)
	svc := &service.ClientService{}

	reseller := &model.User{Id: 50, Username: "res", Role: model.RoleReseller, AllowedInbounds: "1"}

	c, rec := ctxForSubId(reseller, "sub-out-of-scope")
	if guardClientSubIdScope(c, svc, "sub-out-of-scope") {
		t.Fatal("reseller was allowed a subId attached only to inbound #2")
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("got status %d, want 403", rec.Code)
	}

	c, _ = ctxForSubId(reseller, "sub-in-scope")
	if !guardClientSubIdScope(c, svc, "sub-in-scope") {
		t.Error("reseller was refused a subId on their own inbound #1")
	}
}

// TestSubLinksScope_UnscopedRolesUnaffected keeps the guard from becoming a
// blanket restriction: only inbounds.scoped roles are filtered.
func TestSubLinksScope_UnscopedRolesUnaffected(t *testing.T) {
	setupCtrlDB(t)
	seedSubIdFixture(t)
	svc := &service.ClientService{}

	admin := &model.User{Id: 1, Username: "admin", Role: model.RoleSuperAdmin}
	c, _ := ctxForSubId(admin, "sub-out-of-scope")
	if !guardClientSubIdScope(c, svc, "sub-out-of-scope") {
		t.Error("super_admin must not be scoped")
	}
}

// TestSubLinksScope_UnknownSubIdRefusedForReseller pins the fail-closed
// behaviour: a subId with no clients resolves to no inbounds, which must not
// read as "no restriction applies".
func TestSubLinksScope_UnknownSubIdRefusedForReseller(t *testing.T) {
	setupCtrlDB(t)
	seedSubIdFixture(t)
	svc := &service.ClientService{}

	reseller := &model.User{Id: 50, Username: "res", Role: model.RoleReseller, AllowedInbounds: "1"}
	c, rec := ctxForSubId(reseller, "no-such-sub")
	if guardClientSubIdScope(c, svc, "no-such-sub") {
		t.Fatal("unknown subId must be refused for a scoped role")
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("got status %d, want 403", rec.Code)
	}
}

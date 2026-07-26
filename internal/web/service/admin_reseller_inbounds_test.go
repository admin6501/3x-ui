package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func seedReseller(t *testing.T, username string, allowed string, quotaGB int64) *model.User {
	t.Helper()
	u := &model.User{Username: username, Password: "x", Role: model.RoleReseller, AllowedInbounds: allowed, TrafficQuotaGB: quotaGB}
	if err := database.GetDB().Create(u).Error; err != nil {
		t.Fatalf("seed reseller: %v", err)
	}
	return u
}

func inboundEnabled(t *testing.T, id int) bool {
	t.Helper()
	var ib model.Inbound
	if err := database.GetDB().First(&ib, id).Error; err != nil {
		t.Fatalf("load inbound %d: %v", id, err)
	}
	return ib.Enable
}

func trackedRows(t *testing.T, resellerID int) []model.ResellerQuotaDisabledInbound {
	t.Helper()
	var rows []model.ResellerQuotaDisabledInbound
	if err := database.GetDB().Where("reseller_id = ?", resellerID).Find(&rows).Error; err != nil {
		t.Fatalf("load tracking rows: %v", err)
	}
	return rows
}

// TestDisablingResellerTakesTheirInboundsDown is the behaviour an admin expects
// from "disable this reseller": their customers stop connecting, not just the
// reseller's own login. Re-enabling must restore exactly what was taken down.
func TestDisablingResellerTakesTheirInboundsDown(t *testing.T) {
	setupBulkDB(t)
	adminSvc := &AdminService{}
	inboundSvc := &InboundService{}

	ib1 := mkInbound(t, 31001, model.VLESS, `{"clients":[]}`)
	ib2 := mkInbound(t, 31002, model.VLESS, `{"clients":[]}`)
	// A third inbound the admin already switched off by hand.
	ib3 := mkInbound(t, 31003, model.VLESS, `{"clients":[]}`)
	if _, err := inboundSvc.SetInboundEnable(ib3.Id, false); err != nil {
		t.Fatalf("pre-disable inbound: %v", err)
	}

	r := seedReseller(t, "res-a", intsToCSV(ib1.Id, ib2.Id, ib3.Id), 0)
	actor := &model.User{Id: 1, Username: "admin", Role: model.RoleSuperAdmin}

	if err := adminSvc.SetAdminEnabled(actor, r.Id, false, inboundSvc, nil); err != nil {
		t.Fatalf("disable reseller: %v", err)
	}
	for _, id := range []int{ib1.Id, ib2.Id} {
		if inboundEnabled(t, id) {
			t.Errorf("inbound %d should be disabled with the reseller account", id)
		}
	}
	// The hand-disabled inbound is not recorded, so enabling later cannot
	// silently switch it back on.
	rows := trackedRows(t, r.Id)
	if len(rows) != 2 {
		t.Fatalf("tracked %d inbounds, want 2 (the ones we actually disabled)", len(rows))
	}
	for _, row := range rows {
		if row.Reason != model.ResellerDisableReasonAccount {
			t.Errorf("inbound %d tracked with reason %q, want %q", row.InboundId, row.Reason, model.ResellerDisableReasonAccount)
		}
	}

	if err := adminSvc.SetAdminEnabled(actor, r.Id, true, inboundSvc, nil); err != nil {
		t.Fatalf("enable reseller: %v", err)
	}
	for _, id := range []int{ib1.Id, ib2.Id} {
		if !inboundEnabled(t, id) {
			t.Errorf("inbound %d should be back on with the reseller account", id)
		}
	}
	if inboundEnabled(t, ib3.Id) {
		t.Error("an inbound the admin disabled by hand must stay disabled")
	}
	if rows := trackedRows(t, r.Id); len(rows) != 0 {
		t.Errorf("tracking rows should be cleared after re-enabling, got %d", len(rows))
	}
}

// TestDeletingResellerHandsBackItsInbounds covers the leak: an inbound taken
// down for a reseller who is then deleted had nobody left to restore it.
func TestDeletingResellerHandsBackItsInbounds(t *testing.T) {
	setupBulkDB(t)
	adminSvc := &AdminService{}
	inboundSvc := &InboundService{}

	ib := mkInbound(t, 31011, model.VLESS, `{"clients":[]}`)
	r := seedReseller(t, "res-b", intsToCSV(ib.Id), 0)
	actor := &model.User{Id: 1, Username: "admin", Role: model.RoleSuperAdmin}

	if err := adminSvc.SetAdminEnabled(actor, r.Id, false, inboundSvc, nil); err != nil {
		t.Fatalf("disable reseller: %v", err)
	}
	if inboundEnabled(t, ib.Id) {
		t.Fatal("inbound should be down while the account is disabled")
	}

	if err := adminSvc.DeleteAdmin(actor, r.Id, inboundSvc, nil); err != nil {
		t.Fatalf("delete reseller: %v", err)
	}
	if !inboundEnabled(t, ib.Id) {
		t.Error("deleting the reseller must hand its inbound back, not leave it disabled forever")
	}
	if rows := trackedRows(t, r.Id); len(rows) != 0 {
		t.Errorf("tracking rows should not outlive the reseller, got %d", len(rows))
	}
}

// TestUpdatingResellerReleasesRemovedInbounds covers the same leak on the
// narrower path: taking an inbound away from a reseller.
func TestUpdatingResellerReleasesRemovedInbounds(t *testing.T) {
	setupBulkDB(t)
	adminSvc := &AdminService{}
	inboundSvc := &InboundService{}

	kept := mkInbound(t, 31021, model.VLESS, `{"clients":[]}`)
	removed := mkInbound(t, 31022, model.VLESS, `{"clients":[]}`)
	r := seedReseller(t, "res-c", intsToCSV(kept.Id, removed.Id), 0)
	actor := &model.User{Id: 1, Username: "admin", Role: model.RoleSuperAdmin}

	if err := adminSvc.SetAdminEnabled(actor, r.Id, false, inboundSvc, nil); err != nil {
		t.Fatalf("disable reseller: %v", err)
	}

	// Take one inbound away while the account is still disabled.
	if err := adminSvc.UpdateAdmin(actor, r.Id, "res-c", model.RoleReseller, intsToCSV(kept.Id), 0, 0, inboundSvc, nil); err != nil {
		t.Fatalf("update reseller: %v", err)
	}
	if !inboundEnabled(t, removed.Id) {
		t.Error("an inbound taken off a reseller must be handed back, not left disabled")
	}
	if inboundEnabled(t, kept.Id) {
		t.Error("the inbound the reseller still owns must stay down while the account is disabled")
	}

	// Changing the role away from reseller releases the rest.
	if err := adminSvc.UpdateAdmin(actor, r.Id, "res-c", model.RoleManager, "", 0, 0, inboundSvc, nil); err != nil {
		t.Fatalf("update role: %v", err)
	}
	if !inboundEnabled(t, kept.Id) {
		t.Error("leaving the reseller role must release the inbounds held for it")
	}
	if rows := trackedRows(t, r.Id); len(rows) != 0 {
		t.Errorf("no tracking rows should survive, got %d", len(rows))
	}
}

// TestQuotaEnforcerLeavesDisabledAccountsAlone keeps the two mechanisms from
// fighting: a quota recovery must not switch on inbounds that are down because
// the account itself is disabled.
func TestQuotaEnforcerLeavesDisabledAccountsAlone(t *testing.T) {
	setupBulkDB(t)
	adminSvc := &AdminService{}
	inboundSvc := &InboundService{}

	ib := mkInbound(t, 31031, model.VLESS, `{"clients":[]}`)
	r := seedReseller(t, "res-d", intsToCSV(ib.Id), 100)
	actor := &model.User{Id: 1, Username: "admin", Role: model.RoleSuperAdmin}

	if err := adminSvc.SetAdminEnabled(actor, r.Id, false, inboundSvc, nil); err != nil {
		t.Fatalf("disable reseller: %v", err)
	}

	// The reseller is nowhere near their quota, so the enforcer's "recovered"
	// path would previously have re-enabled the inbound.
	adminSvc.EnforceResellerQuotas(inboundSvc)

	if inboundEnabled(t, ib.Id) {
		t.Error("the quota enforcer must not revive inbounds of a disabled account")
	}
	rows := trackedRows(t, r.Id)
	if len(rows) != 1 || rows[0].Reason != model.ResellerDisableReasonAccount {
		t.Fatalf("account tracking row must survive the enforcer, got %+v", rows)
	}
}

func intsToCSV(ids ...int) string {
	out := ""
	for i, id := range ids {
		if i > 0 {
			out += ","
		}
		out += itoa(id)
	}
	return out
}

package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// TestQuotaDisabledInboundOutlivesItsReseller reproduces the way a panel loses
// every client on an inbound and never gets them back: a reseller runs out of
// quota, the enforcer switches their inbound off, and then the reseller row
// goes away without anything switching it back on. Nothing else in the panel
// re-enables an inbound, so it stays dark forever — a restart or an upgrade
// changes nothing, because the state is in the database.
func TestQuotaDisabledInboundOutlivesItsReseller(t *testing.T) {
	setupBulkDB(t)
	adminSvc := &AdminService{}
	inboundSvc := &InboundService{}
	db := database.GetDB()

	ib := mkInbound(t, 35001, model.VLESS, `{"clients":[]}`)
	// The reseller is over their 1 GB quota on this inbound.
	if err := db.Model(&model.Inbound{}).Where("id = ?", ib.Id).
		Update("up", int64(5)*resellerBytesPerGB).Error; err != nil {
		t.Fatalf("seed traffic: %v", err)
	}
	r := seedReseller(t, "gone-soon", intsToCSV(ib.Id), 1)

	adminSvc.EnforceResellerQuotas(inboundSvc)
	if inboundEnabled(t, ib.Id) {
		t.Fatal("the enforcer should have switched the inbound off")
	}

	// The reseller disappears without going through DeleteAdmin — an older
	// build, a manual delete, a restored backup. The tracking row is all that
	// is left, and it points at nobody.
	if err := db.Delete(&model.User{}, r.Id).Error; err != nil {
		t.Fatalf("delete reseller: %v", err)
	}

	// An upgrade, a restart, another enforcement run: none of it helps today.
	adminSvc.EnforceResellerQuotas(inboundSvc)
	if inboundEnabled(t, ib.Id) {
		t.Fatal("precondition failed: something already re-enabled the inbound")
	}

	// The repair is what has to bring it back.
	if !adminSvc.ReleaseOrphanedResellerInbounds(inboundSvc) {
		t.Fatal("the repair reported nothing to do")
	}
	if !inboundEnabled(t, ib.Id) {
		t.Error("the orphaned inbound is still off — every client on it stays dark")
	}
	if rows := trackedRows(t, r.Id); len(rows) != 0 {
		t.Errorf("the orphaned tracking row survived: %+v", rows)
	}
	// Running it again finds nothing left to do.
	if adminSvc.ReleaseOrphanedResellerInbounds(inboundSvc) {
		t.Error("the repair is not idempotent")
	}
}

// TestRepairLeavesLiveResellersAlone: an inbound that is legitimately off
// because its reseller is over quota (or disabled) must stay off.
func TestRepairLeavesLiveResellersAlone(t *testing.T) {
	setupBulkDB(t)
	adminSvc := &AdminService{}
	inboundSvc := &InboundService{}
	db := database.GetDB()

	overQuota := mkInbound(t, 35011, model.VLESS, `{"clients":[]}`)
	if err := db.Model(&model.Inbound{}).Where("id = ?", overQuota.Id).
		Update("up", int64(5)*resellerBytesPerGB).Error; err != nil {
		t.Fatalf("seed traffic: %v", err)
	}
	seedReseller(t, "still-here", intsToCSV(overQuota.Id), 1)
	adminSvc.EnforceResellerQuotas(inboundSvc)
	if inboundEnabled(t, overQuota.Id) {
		t.Fatal("precondition: the inbound should be off")
	}

	suspended := mkInbound(t, 35012, model.VLESS, `{"clients":[]}`)
	other := seedReseller(t, "suspended", intsToCSV(suspended.Id), 0)
	actor := &model.User{Id: 1, Username: "admin", Role: model.RoleSuperAdmin}
	if err := adminSvc.SetAdminEnabled(actor, other.Id, false, inboundSvc, nil); err != nil {
		t.Fatalf("disable: %v", err)
	}

	if adminSvc.ReleaseOrphanedResellerInbounds(inboundSvc) {
		t.Error("the repair touched inbounds that still have an owner")
	}
	if inboundEnabled(t, overQuota.Id) {
		t.Error("an over-quota reseller's inbound must stay off")
	}
	if inboundEnabled(t, suspended.Id) {
		t.Error("a disabled account's inbound must stay off")
	}
}

// TestRepairReleasesInboundsOfADemotedReseller: taking the reseller role away
// leaves the same orphan behind if it happens outside UpdateAdmin.
func TestRepairReleasesInboundsOfADemotedReseller(t *testing.T) {
	setupBulkDB(t)
	adminSvc := &AdminService{}
	inboundSvc := &InboundService{}
	db := database.GetDB()

	ib := mkInbound(t, 35021, model.VLESS, `{"clients":[]}`)
	if err := db.Model(&model.Inbound{}).Where("id = ?", ib.Id).
		Update("up", int64(5)*resellerBytesPerGB).Error; err != nil {
		t.Fatalf("seed traffic: %v", err)
	}
	r := seedReseller(t, "demoted", intsToCSV(ib.Id), 1)
	adminSvc.EnforceResellerQuotas(inboundSvc)
	if inboundEnabled(t, ib.Id) {
		t.Fatal("precondition: the inbound should be off")
	}

	if err := db.Model(&model.User{}).Where("id = ?", r.Id).
		Update("role", model.RoleManager).Error; err != nil {
		t.Fatalf("demote: %v", err)
	}
	if !adminSvc.ReleaseOrphanedResellerInbounds(inboundSvc) {
		t.Fatal("the repair should have released the inbound")
	}
	if !inboundEnabled(t, ib.Id) {
		t.Error("an account that is no longer a reseller must not hold an inbound down")
	}
}

// TestAssigningAnInboundThatDoesNotExistIsRefused: a typo in an inbound id used
// to produce an account scoped to nothing, which looks fine and governs
// nothing. It has to fail at the moment it is typed.
func TestAssigningAnInboundThatDoesNotExistIsRefused(t *testing.T) {
	setupBulkDB(t)
	adminSvc := &AdminService{}
	real := mkInbound(t, 35031, model.VLESS, `{"clients":[]}`)
	actor := &model.User{Id: 1, Username: "admin", Role: model.RoleSuperAdmin}

	if _, err := adminSvc.CreateAdmin(actor, "typo", "pw", model.RoleReseller, "4242", 10, 5); err == nil {
		t.Error("a reseller was created on an inbound that does not exist")
	}
	if _, err := adminSvc.CreateAdmin(actor, "typo2", "pw", model.RoleReseller,
		intsToCSV(real.Id)+",4242", 10, 5); err == nil {
		t.Error("one bad id among good ones should still be refused")
	}
	u, err := adminSvc.CreateAdmin(actor, "good", "pw", model.RoleReseller, intsToCSV(real.Id), 10, 5)
	if err != nil {
		t.Fatalf("a real inbound was refused: %v", err)
	}

	// Deleting the inbound later must not make unrelated edits to that account
	// impossible — the stale id is already assigned, so it is not re-checked.
	if err := database.GetDB().Delete(&model.Inbound{}, real.Id).Error; err != nil {
		t.Fatalf("delete inbound: %v", err)
	}
	if err := adminSvc.UpdateAdmin(actor, u.Id, "good", model.RoleReseller,
		intsToCSV(real.Id), 20, 5, &InboundService{}, nil); err != nil {
		t.Errorf("an edit carrying an already-assigned stale id was refused: %v", err)
	}
}

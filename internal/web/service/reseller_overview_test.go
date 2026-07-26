package service

import (
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

// seedOverviewClient creates a client record, links it to an inbound and gives
// it a traffic row, which is the shape the dashboard reads.
func seedOverviewClient(t *testing.T, inboundId int, email string, enable bool, up, down, total, expiry, createdAt int64) {
	t.Helper()
	db := database.GetDB()
	rec := &model.ClientRecord{Email: email, Enable: enable, TotalGB: total, ExpiryTime: expiry}
	if err := db.Create(rec).Error; err != nil {
		t.Fatalf("create client %q: %v", email, err)
	}
	// `enable` defaults to true and `created_at` is filled by gorm on insert, so
	// both have to be written back explicitly for the test to control them.
	if err := db.Model(&model.ClientRecord{}).Where("id = ?", rec.Id).
		Updates(map[string]any{"enable": enable, "created_at": createdAt}).Error; err != nil {
		t.Fatalf("set client %q state: %v", email, err)
	}
	if err := db.Create(&model.ClientInbound{ClientId: rec.Id, InboundId: inboundId}).Error; err != nil {
		t.Fatalf("link client %q: %v", email, err)
	}
	traffic := &xray.ClientTraffic{Email: email, Enable: enable, Up: up, Down: down, Total: total, ExpiryTime: expiry, InboundId: inboundId}
	if err := db.Create(traffic).Error; err != nil {
		t.Fatalf("create traffic for %q: %v", email, err)
	}
}

// TestResellerOverviewBucketsClients is the dashboard's whole point: the four
// headline numbers have to agree with what the reseller sees in the client
// list, including a client that is "enabled" but out of traffic or past its
// expiry date.
func TestResellerOverviewBucketsClients(t *testing.T) {
	setupBulkDB(t)
	adminSvc := &AdminService{}
	inboundSvc := &InboundService{}

	ib := mkInbound(t, 32001, model.VLESS, `{"clients":[]}`)
	// An inbound the reseller does NOT own — none of its clients may leak in.
	other := mkInbound(t, 32002, model.VLESS, `{"clients":[]}`)

	now := time.Now()
	nowMs := now.UnixMilli()
	const gb = int64(1024 * 1024 * 1024)

	seedOverviewClient(t, ib.Id, "alive", true, gb, gb, 100*gb, now.Add(60*24*time.Hour).UnixMilli(), nowMs-5000)
	seedOverviewClient(t, ib.Id, "renew-me", true, gb, 0, 100*gb, now.Add(3*24*time.Hour).UnixMilli(), nowMs-4000)
	seedOverviewClient(t, ib.Id, "expired", true, gb, 0, 100*gb, now.Add(-time.Hour).UnixMilli(), nowMs-3000)
	seedOverviewClient(t, ib.Id, "depleted", true, 5*gb, 5*gb, 10*gb, 0, nowMs-2000)
	seedOverviewClient(t, ib.Id, "switched-off", false, 0, 0, 0, 0, nowMs-1000)
	seedOverviewClient(t, other.Id, "not-mine", true, gb, gb, 0, 0, nowMs)

	r := seedReseller(t, "res-overview", intsToCSV(ib.Id), 50)

	out := adminSvc.GetResellerOverview(r, inboundSvc)

	if out.ClientsActive != 2 {
		t.Errorf("active = %d, want 2 (alive + renew-me)", out.ClientsActive)
	}
	if out.ClientsExpiring != 1 {
		t.Errorf("expiring = %d, want 1 (renew-me)", out.ClientsExpiring)
	}
	if out.ClientsEnded != 2 {
		t.Errorf("ended = %d, want 2 (expired + depleted)", out.ClientsEnded)
	}
	if out.ClientsDisabled != 1 {
		t.Errorf("disabled = %d, want 1 (switched-off)", out.ClientsDisabled)
	}

	if len(out.Inbounds) != 1 || out.Inbounds[0].Id != ib.Id {
		t.Fatalf("inbounds = %+v, want only the assigned one", out.Inbounds)
	}
	if out.Inbounds[0].Clients != 5 {
		t.Errorf("inbound client count = %d, want 5", out.Inbounds[0].Clients)
	}

	if len(out.ExpiringSoon) != 1 || out.ExpiringSoon[0].Email != "renew-me" {
		t.Errorf("expiringSoon = %+v, want just renew-me", out.ExpiringSoon)
	}
	// Newest first, and never a client from an inbound the reseller lacks.
	if len(out.Recent) == 0 || out.Recent[0].Email != "switched-off" {
		t.Errorf("recent[0] = %+v, want the newest client", out.Recent)
	}
	for _, c := range out.Recent {
		if c.Email == "not-mine" {
			t.Fatal("a client from an unassigned inbound leaked into the overview")
		}
	}
}

// TestResellerOverviewIsEmptyForOtherRoles keeps the endpoint safe to call from
// any session: a non-reseller must not receive somebody else's client list.
func TestResellerOverviewIsEmptyForOtherRoles(t *testing.T) {
	setupBulkDB(t)
	adminSvc := &AdminService{}

	ib := mkInbound(t, 32011, model.VLESS, `{"clients":[]}`)
	seedOverviewClient(t, ib.Id, "somebody", true, 0, 0, 0, 0, time.Now().UnixMilli())

	admin := &model.User{Id: 1, Username: "admin", Role: model.RoleSuperAdmin}
	out := adminSvc.GetResellerOverview(admin, nil)

	if out.Username != "admin" || out.Role != model.RoleSuperAdmin {
		t.Errorf("identity should still be returned, got %q/%q", out.Username, out.Role)
	}
	if len(out.Inbounds) != 0 || len(out.Recent) != 0 || out.ClientsActive != 0 {
		t.Errorf("a non-reseller must get an empty overview, got %+v", out)
	}
}

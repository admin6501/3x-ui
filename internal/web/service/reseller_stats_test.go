package service

import (
	"path/filepath"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func setupResellerDB(t *testing.T) {
	t.Helper()
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	if err := database.InitDB(filepath.Join(dbDir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
}

// TestResellerStatsAndQuota verifies that GetResellerStats sums traffic across
// ALL assigned inbounds as a single number, counts distinct clients, and that
// the cumulative created counter increments and survives client deletion.
func TestResellerStatsAndQuota(t *testing.T) {
	setupResellerDB(t)
	db := database.GetDB()
	adminSvc := AdminService{}

	// Two inbounds assigned to the reseller, one not.
	ib1 := mkInbound(t, 10001, model.VLESS, clientsSettings(t, nil))
	ib2 := mkInbound(t, 10002, model.VLESS, clientsSettings(t, nil))
	ibOther := mkInbound(t, 10003, model.VLESS, clientsSettings(t, nil))

	// Give the assigned inbounds traffic; the unassigned one must NOT count.
	db.Model(&model.Inbound{}).Where("id = ?", ib1.Id).Updates(map[string]any{"up": int64(5 * 1024 * 1024 * 1024), "down": int64(5 * 1024 * 1024 * 1024)}) // 10 GB
	db.Model(&model.Inbound{}).Where("id = ?", ib2.Id).Updates(map[string]any{"up": int64(2 * 1024 * 1024 * 1024), "down": int64(0)})                       // 2 GB
	db.Model(&model.Inbound{}).Where("id = ?", ibOther.Id).Updates(map[string]any{"up": int64(99 * 1024 * 1024 * 1024), "down": int64(0)})                   // ignored

	// Create the reseller with quotas (20 GB / 3 clients).
	reseller, err := adminSvc.CreateAdmin(nil, "res1", "pw", model.RoleReseller,
		itoa(ib1.Id)+","+itoa(ib2.Id), 20, 3)
	if err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	reloaded, _ := adminSvc.GetUserByID(reseller.Id)

	// Seed 2 distinct clients attached to assigned inbounds + 1 to the other.
	cr := func(email string, ibIDs ...int) {
		rec := &model.ClientRecord{Email: email, UUID: email}
		if err := db.Create(rec).Error; err != nil {
			t.Fatalf("create client %s: %v", email, err)
		}
		for _, id := range ibIDs {
			if err := db.Create(&model.ClientInbound{ClientId: rec.Id, InboundId: id}).Error; err != nil {
				t.Fatalf("link client %s -> %d: %v", email, id, err)
			}
		}
	}
	cr("alice", ib1.Id)
	cr("bob", ib1.Id, ib2.Id) // distinct client across two inbounds -> counts once
	cr("carol", ibOther.Id)   // outside scope -> ignored

	stats := adminSvc.GetResellerStats(reloaded)

	wantTraffic := int64(12 * 1024 * 1024 * 1024) // 10 + 2 GB
	if stats.TrafficUsedBytes != wantTraffic {
		t.Errorf("TrafficUsedBytes = %d, want %d", stats.TrafficUsedBytes, wantTraffic)
	}
	if stats.CurrentClients != 2 {
		t.Errorf("CurrentClients = %d, want 2 (alice + bob, deduped)", stats.CurrentClients)
	}
	if stats.TrafficQuotaGB != 20 || stats.ClientQuota != 3 {
		t.Errorf("quota = %dGB/%d, want 20/3", stats.TrafficQuotaGB, stats.ClientQuota)
	}

	// Cumulative counter: increment by 2, then "delete" a client and confirm
	// the counter does NOT drop.
	adminSvc.IncrementClientsCreated(reseller.Id, 2)
	after, _ := adminSvc.GetUserByID(reseller.Id)
	if after.ClientsCreatedTotal != 2 {
		t.Errorf("ClientsCreatedTotal = %d, want 2", after.ClientsCreatedTotal)
	}
	// Simulate deletion of bob from scope; created-total must stay 2.
	db.Where("inbound_id IN ?", []int{ib1.Id, ib2.Id}).Delete(&model.ClientInbound{})
	after2, _ := adminSvc.GetUserByID(reseller.Id)
	if after2.ClientsCreatedTotal != 2 {
		t.Errorf("after delete ClientsCreatedTotal = %d, want 2 (must not decrement)", after2.ClientsCreatedTotal)
	}
	statsAfter := adminSvc.GetResellerStats(after2)
	if statsAfter.CurrentClients != 0 {
		t.Errorf("CurrentClients after delete = %d, want 0", statsAfter.CurrentClients)
	}
	if statsAfter.ClientsCreatedTotal != 2 {
		t.Errorf("stats.ClientsCreatedTotal after delete = %d, want 2", statsAfter.ClientsCreatedTotal)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

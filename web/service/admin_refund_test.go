package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
)

// TestRefundUsageFloorAtZero ensures that refunding more bytes than the
// current usage clamps to 0 rather than going negative. This is the
// guardrail that lets the new "refund on delete" path safely apply to
// pre-existing clients whose allocations were never accumulated.
func TestRefundUsageFloorAtZero(t *testing.T) {
	if err := database.InitDB(":memory:"); err != nil {
		t.Fatalf("init db: %v", err)
	}
	db := database.GetDB()

	u := &model.User{
		Username:     "r1",
		Password:     "x",
		Role:         model.RoleReseller,
		TrafficQuota: 100 * 1024 * 1024 * 1024,
		TrafficUsed:  5 * 1024 * 1024 * 1024,
	}
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	s := &AdminService{}
	// Refund larger than usage: floor at 0
	if err := s.RefundUsage(u, 50*1024*1024*1024); err != nil {
		t.Fatalf("refund: %v", err)
	}
	if u.TrafficUsed != 0 {
		t.Fatalf("in-memory copy not zeroed: got %d", u.TrafficUsed)
	}
	var fresh model.User
	if err := db.First(&fresh, u.Id).Error; err != nil {
		t.Fatalf("reread: %v", err)
	}
	if fresh.TrafficUsed != 0 {
		t.Fatalf("db row not zeroed: got %d", fresh.TrafficUsed)
	}
}

// TestRefundUsagePartial: a refund smaller than current usage subtracts
// exactly that amount. Mirrors the common "client used some traffic,
// delete refunds the unused remainder" path.
func TestRefundUsagePartial(t *testing.T) {
	if err := database.InitDB(":memory:"); err != nil {
		t.Fatalf("init db: %v", err)
	}
	db := database.GetDB()
	u := &model.User{Username: "r2", Password: "x", Role: model.RoleReseller, TrafficUsed: 1000}
	_ = db.Create(u).Error

	s := &AdminService{}
	if err := s.RefundUsage(u, 300); err != nil {
		t.Fatalf("refund: %v", err)
	}
	if u.TrafficUsed != 700 {
		t.Fatalf("in-memory got %d want 700", u.TrafficUsed)
	}
	var fresh model.User
	_ = db.First(&fresh, u.Id).Error
	if fresh.TrafficUsed != 700 {
		t.Fatalf("db got %d want 700", fresh.TrafficUsed)
	}
}

// TestRefundUsageNoopForNonReseller ensures non-reseller roles are skipped.
func TestRefundUsageNoopForNonReseller(t *testing.T) {
	if err := database.InitDB(":memory:"); err != nil {
		t.Fatalf("init db: %v", err)
	}
	db := database.GetDB()
	u := &model.User{Username: "m", Password: "x", Role: model.RoleManager, TrafficUsed: 1000}
	_ = db.Create(u).Error

	s := &AdminService{}
	if err := s.RefundUsage(u, 500); err != nil {
		t.Fatalf("refund: %v", err)
	}
	if u.TrafficUsed != 1000 {
		t.Fatalf("non-reseller mutated: got %d want 1000", u.TrafficUsed)
	}
}

package service

import (
	"path/filepath"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

func setupMigrationDB(t *testing.T) {
	t.Helper()
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	if err := database.InitDB(filepath.Join(dbDir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
}

// TestDetachedClientKeepsItsTrafficRow is the regression this exists for.
//
// Detaching a client from its last inbound deliberately keeps its usage and
// expiry so it can be re-attached later. The client then has no entry in any
// inbound's settings.clients[] JSON, which used to be the sole definition of
// "orphaned" — so every `x-ui migrate` and every backup restore silently threw
// that row away while the client itself stayed in the list.
func TestDetachedClientKeepsItsTrafficRow(t *testing.T) {
	setupMigrationDB(t)
	db := database.GetDB()

	// Alive in the clients table, attached to no inbound: what Detach leaves.
	if err := db.Create(&model.ClientRecord{Email: "detached@example.com", Enable: true}).Error; err != nil {
		t.Fatalf("create client: %v", err)
	}
	if err := db.Create(&xray.ClientTraffic{
		Email: "detached@example.com", Up: 5_000_000, Down: 9_000_000, Total: 10 << 30,
	}).Error; err != nil {
		t.Fatalf("create traffic: %v", err)
	}

	// A row for a client that really is gone from both places.
	if err := db.Create(&xray.ClientTraffic{Email: "deleted@example.com", Up: 1, Down: 1}).Error; err != nil {
		t.Fatalf("create orphan traffic: %v", err)
	}

	svc := InboundService{}
	svc.MigrationRemoveOrphanedTraffics()

	var kept xray.ClientTraffic
	if err := db.Where("email = ?", "detached@example.com").First(&kept).Error; err != nil {
		t.Fatalf("a detached-but-alive client lost its traffic row: %v", err)
	}
	if kept.Up != 5_000_000 || kept.Down != 9_000_000 {
		t.Errorf("usage was altered: up %d down %d", kept.Up, kept.Down)
	}

	var orphanCount int64
	db.Model(&xray.ClientTraffic{}).Where("email = ?", "deleted@example.com").Count(&orphanCount)
	if orphanCount != 0 {
		t.Error("a genuinely orphaned row survived; the sweep no longer does its job")
	}
}

// TestMigrationIsIdempotent: it runs on every migrate and every restore, so a
// second pass must not start deleting what the first one kept.
func TestMigrationIsIdempotent(t *testing.T) {
	setupMigrationDB(t)
	db := database.GetDB()

	if err := db.Create(&model.ClientRecord{Email: "alive@example.com", Enable: true}).Error; err != nil {
		t.Fatalf("create client: %v", err)
	}
	if err := db.Create(&xray.ClientTraffic{Email: "alive@example.com", Up: 42}).Error; err != nil {
		t.Fatalf("create traffic: %v", err)
	}

	svc := InboundService{}
	for pass := range 3 {
		svc.MigrationRemoveOrphanedTraffics()
		var count int64
		db.Model(&xray.ClientTraffic{}).Where("email = ?", "alive@example.com").Count(&count)
		if count != 1 {
			t.Fatalf("pass %d removed a live client's traffic row", pass+1)
		}
	}
}

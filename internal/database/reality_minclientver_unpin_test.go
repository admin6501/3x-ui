package database

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func storedMinClientVer(t *testing.T, id int) (string, bool) {
	t.Helper()
	var ib model.Inbound
	if err := db.Model(&model.Inbound{}).Where("id = ?", id).First(&ib).Error; err != nil {
		t.Fatalf("load inbound %d: %v", id, err)
	}
	var stream struct {
		Reality struct {
			MinClientVer *string `json:"minClientVer"`
		} `json:"realitySettings"`
	}
	if err := json.Unmarshal([]byte(ib.StreamSettings), &stream); err != nil {
		t.Fatalf("unmarshal stream of inbound %d: %v", id, err)
	}
	if stream.Reality.MinClientVer == nil {
		return "", false
	}
	return *stream.Reality.MinClientVer, true
}

// TestUnpinSeederClearsPinnedInboundsOnUpgrade drives the migration the way an
// upgrade does — against real rows in a real database — because the defect it
// fixes was exactly this: the code that wrote the value was reverted while the
// rows it had already rewritten kept it.
func TestUnpinSeederClearsPinnedInboundsOnUpgrade(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dir)
	if err := InitDB(filepath.Join(dir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = CloseDB() })

	stream := func(minClientVer string, security string) string {
		reality := map[string]any{"dest": "example.com:443", "privateKey": "k"}
		if minClientVer != "" {
			reality["minClientVer"] = minClientVer
		}
		raw, err := json.Marshal(map[string]any{
			"network": "tcp", "security": security, "realitySettings": reality,
		})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return string(raw)
	}

	rows := []*model.Inbound{
		{Id: 1, Remark: "pinned", Tag: "in-1", Protocol: model.VLESS, StreamSettings: stream(model.PinnedRealityMinClientVer, "reality")},
		{Id: 2, Remark: "operator-chose", Tag: "in-2", Protocol: model.VLESS, StreamSettings: stream("1.0.0", "reality")},
		{Id: 3, Remark: "untouched", Tag: "in-3", Protocol: model.VLESS, StreamSettings: stream("", "reality")},
	}
	for _, ib := range rows {
		if err := db.Create(ib).Error; err != nil {
			t.Fatalf("create inbound %d: %v", ib.Id, err)
		}
	}

	// Simulate the upgrade path: the seeder has not run on this database yet.
	if err := db.Where("seeder_name = ?", "InboundRealityMinClientVerUnpin").
		Delete(&model.HistoryOfSeeders{}).Error; err != nil {
		t.Fatalf("clear seeder history: %v", err)
	}

	if err := unpinRealityMinClientVer(); err != nil {
		t.Fatalf("unpinRealityMinClientVer: %v", err)
	}

	if got, present := storedMinClientVer(t, 1); present {
		t.Errorf("inbound 1 still pinned to %q", got)
	}
	if got, present := storedMinClientVer(t, 2); !present || got != "1.0.0" {
		t.Errorf("inbound 2: operator's 1.0.0 became %q (present=%t)", got, present)
	}
	if _, present := storedMinClientVer(t, 3); present {
		t.Error("inbound 3 gained a minClientVer it never had")
	}

	var history []string
	if err := db.Model(&model.HistoryOfSeeders{}).Pluck("seeder_name", &history).Error; err != nil {
		t.Fatalf("read seeder history: %v", err)
	}
	if !slices.Contains(history, "InboundRealityMinClientVerUnpin") {
		t.Error("seeder did not record itself; it would run again on every start")
	}
}

package service

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// TestAddingAClientKeepsUnmirroredNeighbours is the bug behind "I pointed the
// shop bot at one of my inbounds and my own clients on it stopped working".
//
// Adding a client rewrites the target inbound's whole clients array, and on the
// way it dropped every client whose email had no client_records row. Those rows
// are a mirror, not the source of truth — a JSON client can outlive its mirror
// after a partial restore, a hand-edited inbound, or an import. Wiping them
// turns one add into a silent mass delete of working accounts.
func TestAddingAClientKeepsUnmirroredNeighbours(t *testing.T) {
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	if err := database.InitDB(filepath.Join(dbDir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })

	db := database.GetDB()

	// Two clients live in the inbound's JSON. Only "mirrored" also has a
	// client_records row; "orphan" is the one a partial restore left behind.
	settings, _ := json.Marshal(map[string]any{"clients": []map[string]any{
		{"email": "mirrored@example.com", "id": "11111111-1111-1111-1111-111111111111", "enable": true, "subId": "submirrored1"},
		{"email": "orphan@example.com", "id": "22222222-2222-2222-2222-222222222222", "enable": true, "subId": "suborphan001"},
	}})
	inbound := &model.Inbound{
		Tag: "vless-shop", Enable: true, Port: 51001, Protocol: model.VLESS,
		StreamSettings: `{"network":"tcp","security":"reality"}`,
		Settings:       string(settings),
	}
	if err := db.Create(inbound).Error; err != nil {
		t.Fatalf("create inbound: %v", err)
	}

	clientSvc := ClientService{}
	inboundSvc := InboundService{}

	// Mirror only the first client, leaving the second an orphan.
	if err := db.Create(&model.ClientRecord{
		Email: "mirrored@example.com", SubID: "submirrored1", Enable: true,
	}).Error; err != nil {
		t.Fatalf("create client record: %v", err)
	}

	// Now the shop adds its own client to the same inbound.
	if _, err := clientSvc.CreateOne(&inboundSvc, inbound.Id, model.Client{
		Email: "tg123_abcd", SubID: "tgsubid123456789", Enable: true,
	}); err != nil {
		t.Fatalf("CreateOne: %v", err)
	}

	fresh, err := inboundSvc.GetInbound(inbound.Id)
	if err != nil {
		t.Fatalf("GetInbound: %v", err)
	}
	clients, err := inboundSvc.GetClients(fresh)
	if err != nil {
		t.Fatalf("GetClients: %v", err)
	}
	got := map[string]bool{}
	for _, c := range clients {
		got[c.Email] = true
	}
	for _, want := range []string{"mirrored@example.com", "orphan@example.com", "tg123_abcd"} {
		if !got[want] {
			t.Errorf("client %q was dropped from the inbound; have %v", want, got)
		}
	}
}

// TestCompactOrphansStillDropsJustDeletedClients guards the behaviour the
// unmirrored-neighbour fix must not lose: a client deleted through the panel
// that a stale write hands back has to stay deleted rather than be resurrected.
func TestCompactOrphansStillDropsJustDeletedClients(t *testing.T) {
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	if err := database.InitDB(filepath.Join(dbDir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })

	db := database.GetDB()
	if err := db.Create(&model.ClientRecord{
		Email: "kept@example.com", SubID: "subkept00001", Enable: true,
	}).Error; err != nil {
		t.Fatalf("create client record: %v", err)
	}

	clients := []any{
		map[string]any{"email": "kept@example.com"},
		map[string]any{"email": "deleted@example.com"},  // no record, and deleted
		map[string]any{"email": "restored@example.com"}, // no record, never deleted
	}

	// Nothing deleted yet: the list passes through untouched.
	if got := compactOrphans(db, clients); len(got) != 3 {
		t.Fatalf("with no tombstones, compactOrphans kept %d of 3", len(got))
	}

	tombstoneClientEmail("deleted@example.com")
	t.Cleanup(func() { clearTombstones() })

	got := compactOrphans(db, clients)
	have := map[string]bool{}
	for _, c := range got {
		have[c.(map[string]any)["email"].(string)] = true
	}
	if have["deleted@example.com"] {
		t.Error("a just-deleted client was resurrected")
	}
	if !have["kept@example.com"] || !have["restored@example.com"] {
		t.Errorf("compaction took a client it should have kept: %v", have)
	}
}

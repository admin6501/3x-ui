package database

import (
	"encoding/json"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func externalProxyOf(t *testing.T, inboundId int) []map[string]any {
	t.Helper()
	ib := &model.Inbound{}
	if err := GetDB().First(ib, inboundId).Error; err != nil {
		t.Fatalf("load inbound: %v", err)
	}
	stream := map[string]any{}
	if err := json.Unmarshal([]byte(ib.StreamSettings), &stream); err != nil {
		t.Fatalf("stream json: %v", err)
	}
	raw, _ := stream["externalProxy"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, e := range raw {
		if m, ok := e.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func rerunMirrorMigration(t *testing.T) {
	t.Helper()
	GetDB().Where("seeder_name = ?", "ExternalProxyMirrorFromHosts").Delete(&model.HistoryOfSeeders{})
	if err := syncExternalProxyMirrorFromHosts(); err != nil {
		t.Fatalf("syncExternalProxyMirrorFromHosts: %v", err)
	}
}

// An inbound whose legacy externalProxy array was frozen at its pre-Hosts value
// gets it rewritten from the host rows, so the panel's links stop showing the
// stale endpoint.
func TestMigrate_ExternalProxyMirrorFollowsHosts(t *testing.T) {
	initMigrateDB(t)
	ib := seedInboundWithStream(t, "mirror1", 5561,
		`{"network":"tcp","security":"none","externalProxy":[{"forceTls":"same","dest":"stale.cdn.com","port":9999,"remark":"stale"}]}`)
	if err := GetDB().Create(&model.Host{
		InboundId: ib.Id, SortOrder: 0, Remark: "live", Address: "live.cdn.com", Port: 2087, Security: "tls",
	}).Error; err != nil {
		t.Fatalf("create host: %v", err)
	}

	rerunMirrorMigration(t)

	eps := externalProxyOf(t, ib.Id)
	if len(eps) != 1 {
		t.Fatalf("externalProxy = %v, want 1 entry", eps)
	}
	if eps[0]["dest"] != "live.cdn.com" || eps[0]["remark"] != "live" || eps[0]["forceTls"] != "tls" {
		t.Fatalf("entry not rebuilt from host: %v", eps[0])
	}
	if eps[0]["port"].(float64) != 2087 {
		t.Fatalf("port = %v, want 2087", eps[0]["port"])
	}
}

// Inbounds that never had hosts keep whatever externalProxy they carry: the
// hand-written legacy array is still a supported setup.
func TestMigrate_ExternalProxyMirrorLeavesHostlessInbounds(t *testing.T) {
	initMigrateDB(t)
	ib := seedInboundWithStream(t, "mirror2", 5562,
		`{"network":"tcp","security":"none","externalProxy":[{"forceTls":"same","dest":"manual.cdn.com","port":8080,"remark":"manual"}]}`)

	rerunMirrorMigration(t)

	eps := externalProxyOf(t, ib.Id)
	if len(eps) != 1 || eps[0]["dest"] != "manual.cdn.com" {
		t.Fatalf("manual entries must survive, got %v", eps)
	}
}

// The migration is self-gated: a second run (with the history row present) is a
// no-op, so a mirror that drifted on purpose isn't rewritten twice.
func TestMigrate_ExternalProxyMirrorIsSelfGated(t *testing.T) {
	initMigrateDB(t)
	ib := seedInboundWithStream(t, "mirror3", 5563, `{"network":"tcp","security":"none"}`)
	if err := GetDB().Create(&model.Host{
		InboundId: ib.Id, SortOrder: 0, Remark: "h1", Address: "h1.cdn.com", Port: 443,
	}).Error; err != nil {
		t.Fatalf("create host: %v", err)
	}

	rerunMirrorMigration(t)
	if len(externalProxyOf(t, ib.Id)) != 1 {
		t.Fatalf("first run must write the mirror")
	}

	// Wipe the array and re-run WITHOUT clearing the history row.
	if err := GetDB().Model(&model.Inbound{}).Where("id = ?", ib.Id).
		Update("stream_settings", `{"network":"tcp","security":"none"}`).Error; err != nil {
		t.Fatalf("reset stream: %v", err)
	}
	if err := syncExternalProxyMirrorFromHosts(); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if eps := externalProxyOf(t, ib.Id); len(eps) != 0 {
		t.Fatalf("second run must be a no-op, got %v", eps)
	}
}

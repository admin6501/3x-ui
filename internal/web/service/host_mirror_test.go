package service

import (
	"encoding/json"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// mirrorOf reads the inbound's streamSettings.externalProxy array back.
func mirrorOf(t *testing.T, inboundId int) []map[string]any {
	t.Helper()
	ib := &model.Inbound{}
	if err := database.GetDB().First(ib, inboundId).Error; err != nil {
		t.Fatalf("load inbound: %v", err)
	}
	stream := map[string]any{}
	if ib.StreamSettings != "" {
		if err := json.Unmarshal([]byte(ib.StreamSettings), &stream); err != nil {
			t.Fatalf("stream json: %v (%s)", err, ib.StreamSettings)
		}
	}
	raw, ok := stream["externalProxy"].([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(raw))
	for _, e := range raw {
		if m, ok := e.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func mkInboundWithStream(t *testing.T, port int, stream string) *model.Inbound {
	t.Helper()
	ib := mkInbound(t, port, model.VLESS, `{"clients":[]}`)
	if err := database.GetDB().Model(&model.Inbound{}).
		Where("id = ?", ib.Id).
		Update("stream_settings", stream).Error; err != nil {
		t.Fatalf("seed stream: %v", err)
	}
	ib.StreamSettings = stream
	return ib
}

// A host mutation must rewrite the inbound's legacy externalProxy array so the
// panel's own link generator (which still reads it) shows the host's endpoint.
func TestHostMirror_AddHostWritesExternalProxy(t *testing.T) {
	setupBulkDB(t)
	svc := &HostService{}
	ib := mkInboundWithStream(t, 4431, `{"network":"tcp","security":"none","externalProxy":[{"dest":"stale.example.com","port":9999,"remark":"old","forceTls":"same"}]}`)

	if _, err := svc.AddHost(&model.Host{
		InboundId: ib.Id,
		Remark:    "cdn",
		Address:   "cdn.example.com",
		Port:      2053,
		Security:  "tls",
		Sni:       "sni.example.com",
	}); err != nil {
		t.Fatalf("AddHost: %v", err)
	}

	eps := mirrorOf(t, ib.Id)
	if len(eps) != 1 {
		t.Fatalf("mirror len = %d, want 1 (%v)", len(eps), eps)
	}
	if eps[0]["dest"] != "cdn.example.com" || eps[0]["remark"] != "cdn" {
		t.Fatalf("stale entry survived: %v", eps[0])
	}
	if eps[0]["port"].(float64) != 2053 || eps[0]["forceTls"] != "tls" || eps[0]["sni"] != "sni.example.com" {
		t.Fatalf("host fields not mirrored: %v", eps[0])
	}
	// The rest of the stream settings must survive the rewrite.
	ib2 := &model.Inbound{}
	if err := database.GetDB().First(ib2, ib.Id).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	stream := map[string]any{}
	_ = json.Unmarshal([]byte(ib2.StreamSettings), &stream)
	if stream["network"] != "tcp" || stream["security"] != "none" {
		t.Fatalf("stream settings clobbered: %s", ib2.StreamSettings)
	}
}

// Deleting the last host must clear the mirror — otherwise the removed host
// keeps producing links through the legacy fallback path.
func TestHostMirror_DeleteLastHostClearsMirror(t *testing.T) {
	setupBulkDB(t)
	svc := &HostService{}
	ib := mkInboundWithStream(t, 4432, `{"network":"tcp","security":"none"}`)
	h := mkHost(t, svc, ib.Id, "edge", 0)

	if len(mirrorOf(t, ib.Id)) != 1 {
		t.Fatalf("mirror not written on add")
	}
	if err := svc.DeleteHost(h.Id); err != nil {
		t.Fatalf("DeleteHost: %v", err)
	}
	if eps := mirrorOf(t, ib.Id); len(eps) != 0 {
		t.Fatalf("mirror must be empty after deleting the last host, got %v", eps)
	}
}

// Disabled hosts and hosts excluded from the raw subscription are not part of
// the panel's links, so they stay out of the mirror.
func TestHostMirror_SkipsDisabledAndRawExcluded(t *testing.T) {
	setupBulkDB(t)
	svc := &HostService{}
	ib := mkInboundWithStream(t, 4433, `{"network":"tcp","security":"none"}`)
	keep := mkHost(t, svc, ib.Id, "keep", 0)
	off := mkHost(t, svc, ib.Id, "off", 1)
	excluded, err := svc.AddHost(&model.Host{
		InboundId:           ib.Id,
		Remark:              "jsonOnly",
		Address:             "json.example.com",
		Port:                8443,
		ExcludeFromSubTypes: []string{"raw"},
	})
	if err != nil {
		t.Fatalf("AddHost excluded: %v", err)
	}
	_ = excluded
	if err := svc.SetHostEnable(off.Id, false); err != nil {
		t.Fatalf("SetHostEnable: %v", err)
	}

	eps := mirrorOf(t, ib.Id)
	if len(eps) != 1 || eps[0]["remark"] != keep.Remark {
		t.Fatalf("mirror = %v, want only %q", eps, keep.Remark)
	}
}

// An override-only host (no address/port of its own) mirrors a blank dest and
// the inbound's port; the link generators resolve the address per request.
func TestHostMirror_OverrideOnlyHostInheritsInboundPort(t *testing.T) {
	setupBulkDB(t)
	svc := &HostService{}
	ib := mkInboundWithStream(t, 4434, `{"network":"tcp","security":"none"}`)
	if _, err := svc.AddHost(&model.Host{InboundId: ib.Id, Remark: "override", Security: "none"}); err != nil {
		t.Fatalf("AddHost: %v", err)
	}

	eps := mirrorOf(t, ib.Id)
	if len(eps) != 1 {
		t.Fatalf("mirror len = %d, want 1", len(eps))
	}
	if eps[0]["dest"] != "" {
		t.Fatalf("dest = %v, want empty (inherits the inbound address)", eps[0]["dest"])
	}
	if eps[0]["port"].(float64) != float64(ib.Port) {
		t.Fatalf("port = %v, want inbound port %d", eps[0]["port"], ib.Port)
	}
}

// An inbound with no host rows may still carry a hand-written externalProxy
// array (the legacy feature is still supported) — a non-forced sync leaves it
// alone.
func TestHostMirror_HostlessInboundKeepsManualEntries(t *testing.T) {
	setupBulkDB(t)
	stream := `{"network":"tcp","security":"none","externalProxy":[{"dest":"manual.example.com","port":8080,"remark":"manual","forceTls":"same"}]}`
	ib := mkInboundWithStream(t, 4435, stream)

	if err := syncInboundHostMirror(nil, ib.Id, false); err != nil {
		t.Fatalf("sync: %v", err)
	}
	eps := mirrorOf(t, ib.Id)
	if len(eps) != 1 || eps[0]["dest"] != "manual.example.com" {
		t.Fatalf("manual externalProxy must survive, got %v", eps)
	}
}

// Reordering hosts reorders the mirror, because the panel renders one link per
// entry in that order.
func TestHostMirror_ReorderReordersMirror(t *testing.T) {
	setupBulkDB(t)
	svc := &HostService{}
	ib := mkInboundWithStream(t, 4436, `{"network":"tcp","security":"none"}`)
	first := mkHost(t, svc, ib.Id, "first", 0)
	second := mkHost(t, svc, ib.Id, "second", 1)

	if err := svc.ReorderHosts([]int{second.Id, first.Id}); err != nil {
		t.Fatalf("ReorderHosts: %v", err)
	}
	eps := mirrorOf(t, ib.Id)
	if len(eps) != 2 || eps[0]["remark"] != "second" || eps[1]["remark"] != "first" {
		t.Fatalf("mirror order = %v", eps)
	}
}

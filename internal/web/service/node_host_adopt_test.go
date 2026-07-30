package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"gorm.io/gorm"
)

// TestAdoptNodeHostsCopiesOverridesToTheCentralInbound is the regression for
// adopted inbounds losing their subscription TLS settings.
//
// Per-inbound Host overrides are looked up by local inbound id when
// subscriptions render, and nothing else in the node sync fetches them. Without
// this import an inbound adopted from a managed node gets zero Host rows on the
// master, so its configs fall back to a bare TLS block without the fingerprint
// and SNI the node was configured with.
func TestAdoptNodeHostsCopiesOverridesToTheCentralInbound(t *testing.T) {
	setupConflictDB(t)
	db := database.GetDB()

	const nodeInboundId, centralInboundId = 7, 42
	hosts := []*model.Host{
		{Id: 1, InboundId: nodeInboundId, Remark: "cdn-front", Tags: []string{"cf"}},
		{Id: 2, InboundId: nodeInboundId, Remark: "direct"},
		// Belongs to a different inbound on the node: must not be copied.
		{Id: 3, InboundId: 99, Remark: "other-inbound"},
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		adoptNodeHosts(tx, hosts, nodeInboundId, centralInboundId)
		return nil
	}); err != nil {
		t.Fatalf("transaction: %v", err)
	}

	var copied []model.Host
	if err := db.Where("inbound_id = ?", centralInboundId).Order("remark").Find(&copied).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if len(copied) != 2 {
		t.Fatalf("copied %d host rows, want 2: %+v", len(copied), copied)
	}
	if copied[0].Remark != "cdn-front" || copied[1].Remark != "direct" {
		t.Errorf("wrong rows copied: %+v", copied)
	}
	// Re-pointed at the central inbound: this is what subscription rendering
	// looks the overrides up by, and copying them unchanged would attach them
	// to whatever local inbound happens to hold the node's id.
	for _, h := range copied {
		if h.InboundId != centralInboundId {
			t.Errorf("host %q points at inbound %d, want the central %d", h.Remark, h.InboundId, centralInboundId)
		}
	}

	var strays int64
	db.Model(&model.Host{}).Where("inbound_id = ?", 99).Count(&strays)
	if strays != 0 {
		t.Error("copied a host belonging to a different inbound on the node")
	}
}

// TestAdoptNodeHostsIgnoresNothingToDo keeps the adoption path cheap and safe
// for the overwhelmingly common case: a node with no overrides at all.
func TestAdoptNodeHostsIgnoresNothingToDo(t *testing.T) {
	setupConflictDB(t)
	db := database.GetDB()

	if err := db.Transaction(func(tx *gorm.DB) error {
		adoptNodeHosts(tx, nil, 1, 2)
		adoptNodeHosts(tx, []*model.Host{}, 1, 2)
		adoptNodeHosts(tx, []*model.Host{{InboundId: 1}}, 0, 2)
		adoptNodeHosts(tx, []*model.Host{{InboundId: 1}}, 1, 0)
		return nil
	}); err != nil {
		t.Fatalf("transaction: %v", err)
	}

	var count int64
	db.Model(&model.Host{}).Count(&count)
	if count != 0 {
		t.Errorf("wrote %d host rows for a no-op adoption", count)
	}
}

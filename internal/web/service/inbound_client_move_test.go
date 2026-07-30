package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

// TestLookupFollowsAClientThatMoved is the regression for a config the buyer
// can plainly see but the bot cannot build a link for.
//
// client_traffics.inbound_id is a legacy single-inbound pointer, and moving a
// client between inbounds does not update it. The lookup trusted that pointer
// whenever the inbound still existed, so it searched the old inbound, did not
// find the email, and failed — breaking the Telegram bot's link and QR.
func TestLookupFollowsAClientThatMoved(t *testing.T) {
	setupConflictDB(t)
	db := database.GetDB()
	svc := InboundService{}

	const email = "mover@example.com"
	clients := `{"clients":[{"id":"11111111-2222-4333-8444-555555555555","email":"` + email + `","enable":true}]}`

	// Both inbounds exist; the client now lives on the second one.
	seedInboundConflictNode(t, "old-home", "", 10001, model.VLESS, `{"network":"tcp"}`, `{"clients":[]}`, nil)
	seedInboundConflictNode(t, "new-home", "", 10002, model.VLESS, `{"network":"tcp"}`, clients, nil)
	old := inboundByTag(t, "old-home")
	now := inboundByTag(t, "new-home")

	// The stale pointer still names the inbound the client left.
	if err := db.Create(&xray.ClientTraffic{InboundId: old.Id, Email: email, Enable: true}).Error; err != nil {
		t.Fatalf("seed traffic: %v", err)
	}
	// The authoritative link: a real client row joined to the inbound that
	// actually hosts it now. This is what the re-resolution follows.
	rec := &model.ClientRecord{Email: email, Enable: true}
	if err := db.Create(rec).Error; err != nil {
		t.Fatalf("seed client record: %v", err)
	}
	if err := db.Create(&model.ClientInbound{ClientId: rec.Id, InboundId: now.Id}).Error; err != nil {
		t.Fatalf("seed client_inbounds: %v", err)
	}

	_, resolved, err := svc.GetClientInboundByEmail(email)
	if err != nil {
		t.Fatalf("GetClientInboundByEmail: %v", err)
	}
	if resolved == nil {
		t.Fatal("no inbound resolved for a client that exists")
	}
	if resolved.Id != now.Id {
		t.Errorf("resolved inbound %d (%q), want %d (%q) — the lookup followed the stale pointer",
			resolved.Id, resolved.Remark, now.Id, now.Remark)
	}
}

// TestLookupKeepsAnAccuratePointer: the re-resolution must not fire when the
// pointer is right, or every lookup would pay for extra queries.
func TestLookupKeepsAnAccuratePointer(t *testing.T) {
	setupConflictDB(t)
	db := database.GetDB()
	svc := InboundService{}

	const email = "settled@example.com"
	clients := `{"clients":[{"id":"11111111-2222-4333-8444-555555555555","email":"` + email + `","enable":true}]}`
	seedInboundConflictNode(t, "home", "", 10003, model.VLESS, `{"network":"tcp"}`, clients, nil)
	home := inboundByTag(t, "home")

	if err := db.Create(&xray.ClientTraffic{InboundId: home.Id, Email: email, Enable: true}).Error; err != nil {
		t.Fatalf("seed traffic: %v", err)
	}

	_, resolved, err := svc.GetClientInboundByEmail(email)
	if err != nil {
		t.Fatalf("GetClientInboundByEmail: %v", err)
	}
	if resolved == nil || resolved.Id != home.Id {
		t.Fatalf("resolved %v, want inbound %d", resolved, home.Id)
	}
}

// inboundByTag reads back a seeded inbound so the test can use its assigned id.
func inboundByTag(t *testing.T, tag string) *model.Inbound {
	t.Helper()
	var ib model.Inbound
	if err := database.GetDB().Where("tag = ?", tag).First(&ib).Error; err != nil {
		t.Fatalf("read inbound %q: %v", tag, err)
	}
	return &ib
}

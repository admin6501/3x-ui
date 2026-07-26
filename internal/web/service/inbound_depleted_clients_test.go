package service

import (
	"sort"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

// seedTraffic writes one client_traffics row. Callers set the fields that
// decide "depleted": total/up/down for the quota side, expiryTime for the
// date side, and reset for the auto-renew opt-out.
func seedTraffic(t *testing.T, row xray.ClientTraffic) {
	t.Helper()
	if err := database.GetDB().Create(&row).Error; err != nil {
		t.Fatalf("seed traffic %s: %v", row.Email, err)
	}
}

// TestDepletedEmailsByInbound covers the filter behind the inbound row action
// "delete expired clients": expired-by-date and quota-exhausted clients are
// selected, everyone else (including auto-renew rows) is left alone, and only
// clients of the requested inbound are ever returned.
func TestDepletedEmailsByInbound(t *testing.T) {
	setupBulkDB(t)
	inboundSvc := &InboundService{}

	past := time.Now().Add(-24 * time.Hour).UnixMilli()
	future := time.Now().Add(24 * time.Hour).UnixMilli()

	clients := []model.Client{
		{Email: "expired@x", ID: "11111111-1111-1111-1111-111111111111", Enable: true},
		{Email: "exhausted@x", ID: "22222222-2222-2222-2222-222222222222", Enable: true},
		{Email: "active@x", ID: "33333333-3333-3333-3333-333333333333", Enable: true},
		{Email: "unlimited@x", ID: "44444444-4444-4444-4444-444444444444", Enable: true},
		{Email: "renewing@x", ID: "55555555-5555-5555-5555-555555555555", Enable: true},
		{Email: "noRow@x", ID: "66666666-6666-6666-6666-666666666666", Enable: true},
	}
	ib := mkInbound(t, 21001, model.VLESS, clientsSettings(t, clients))
	other := []model.Client{
		{Email: "otherExpired@x", ID: "77777777-7777-7777-7777-777777777777", Enable: true},
	}
	ibOther := mkInbound(t, 21002, model.VLESS, clientsSettings(t, other))

	seedTraffic(t, xray.ClientTraffic{InboundId: ib.Id, Email: "expired@x", Enable: true, ExpiryTime: past})
	// Casing drifts between settings JSON and traffic rows in the wild, so the
	// match has to be case-insensitive while the settings spelling is returned.
	seedTraffic(t, xray.ClientTraffic{InboundId: ib.Id, Email: "Exhausted@X", Enable: true, Up: 6, Down: 5, Total: 10})
	seedTraffic(t, xray.ClientTraffic{InboundId: ib.Id, Email: "active@x", Enable: true, Up: 1, Down: 1, Total: 10, ExpiryTime: future})
	seedTraffic(t, xray.ClientTraffic{InboundId: ib.Id, Email: "unlimited@x", Enable: true, Up: 1 << 40, Down: 1 << 40})
	seedTraffic(t, xray.ClientTraffic{InboundId: ib.Id, Email: "renewing@x", Enable: true, ExpiryTime: past, Reset: 30})
	seedTraffic(t, xray.ClientTraffic{InboundId: ibOther.Id, Email: "otherExpired@x", Enable: true, ExpiryTime: past})

	got, err := inboundSvc.DepletedEmailsByInbound(ib.Id)
	if err != nil {
		t.Fatalf("DepletedEmailsByInbound: %v", err)
	}
	sort.Strings(got)
	want := []string{"exhausted@x", "expired@x"}
	if len(got) != len(want) {
		t.Fatalf("depleted emails = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("depleted emails = %v, want %v", got, want)
		}
	}

	// An inbound whose clients are all healthy yields nothing to delete, so the
	// controller can short-circuit instead of running a bulk delete.
	healthy := []model.Client{{Email: "fresh@x", ID: "88888888-8888-8888-8888-888888888888", Enable: true}}
	ibHealthy := mkInbound(t, 21003, model.VLESS, clientsSettings(t, healthy))
	seedTraffic(t, xray.ClientTraffic{InboundId: ibHealthy.Id, Email: "fresh@x", Enable: true, ExpiryTime: future})
	empty, err := inboundSvc.DepletedEmailsByInbound(ibHealthy.Id)
	if err != nil {
		t.Fatalf("DepletedEmailsByInbound (healthy): %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected no depleted emails, got %v", empty)
	}
}

// TestDepletedEmailsByInboundSurvivesBulkDelete wires the filter to the delete
// path the controller uses: only the ended clients disappear from the inbound.
func TestDepletedEmailsByInboundSurvivesBulkDelete(t *testing.T) {
	setupBulkDB(t)
	inboundSvc := &InboundService{}
	clientSvc := &ClientService{}

	past := time.Now().Add(-time.Hour).UnixMilli()
	clients := []model.Client{
		{Email: "gone@x", ID: "11111111-1111-1111-1111-111111111111", Enable: true},
		{Email: "stays@x", ID: "22222222-2222-2222-2222-222222222222", Enable: true},
	}
	ib := mkInbound(t, 21004, model.VLESS, clientsSettings(t, clients))
	if err := clientSvc.SyncInbound(nil, ib.Id, clients); err != nil {
		t.Fatalf("seed linkage: %v", err)
	}
	seedTraffic(t, xray.ClientTraffic{InboundId: ib.Id, Email: "gone@x", Enable: true, ExpiryTime: past})
	seedTraffic(t, xray.ClientTraffic{InboundId: ib.Id, Email: "stays@x", Enable: true})

	emails, err := inboundSvc.DepletedEmailsByInbound(ib.Id)
	if err != nil {
		t.Fatalf("DepletedEmailsByInbound: %v", err)
	}
	if len(emails) != 1 || emails[0] != "gone@x" {
		t.Fatalf("depleted emails = %v, want [gone@x]", emails)
	}

	res, _, err := clientSvc.BulkDelete(inboundSvc, emails, false)
	if err != nil {
		t.Fatalf("BulkDelete: %v", err)
	}
	if res.Deleted != 1 {
		t.Fatalf("deleted = %d, want 1 (skipped: %v)", res.Deleted, res.Skipped)
	}

	after, err := inboundSvc.GetInbound(ib.Id)
	if err != nil {
		t.Fatalf("GetInbound: %v", err)
	}
	remaining, err := inboundSvc.GetClients(after)
	if err != nil {
		t.Fatalf("GetClients: %v", err)
	}
	if len(remaining) != 1 || remaining[0].Email != "stays@x" {
		t.Fatalf("remaining clients = %v, want [stays@x]", sortedEmails(remaining))
	}
}

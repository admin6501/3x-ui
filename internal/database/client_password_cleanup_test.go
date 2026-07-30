package database

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func initCleanupDB(t *testing.T) {
	t.Helper()
	if err := InitDB(filepath.Join(t.TempDir(), "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = CloseDB() })
}

func seedInboundOfProtocol(t *testing.T, tag string, port int, proto model.Protocol, settings string) *model.Inbound {
	t.Helper()
	ib := &model.Inbound{
		UserId: 1, Tag: tag, Enable: true, Port: port, Protocol: proto,
		Remark: tag, Settings: settings, StreamSettings: `{"network":"tcp"}`,
	}
	if err := GetDB().Create(ib).Error; err != nil {
		t.Fatalf("create inbound %s: %v", tag, err)
	}
	return ib
}

func seedClientLinkedTo(t *testing.T, email, password string, inbounds ...*model.Inbound) *model.ClientRecord {
	t.Helper()
	rec := &model.ClientRecord{Email: email, UUID: "uuid-" + email, Password: password, Enable: true}
	if err := GetDB().Create(rec).Error; err != nil {
		t.Fatalf("create client %s: %v", email, err)
	}
	for _, ib := range inbounds {
		link := &model.ClientInbound{ClientId: rec.Id, InboundId: ib.Id}
		if err := GetDB().Create(link).Error; err != nil {
			t.Fatalf("link %s to %s: %v", email, ib.Tag, err)
		}
	}
	return rec
}

func passwordOf(t *testing.T, id int) string {
	t.Helper()
	var rec model.ClientRecord
	if err := GetDB().First(&rec, id).Error; err != nil {
		t.Fatalf("reload client %d: %v", id, err)
	}
	return rec.Password
}

func settingsOf(t *testing.T, id int) string {
	t.Helper()
	var ib model.Inbound
	if err := GetDB().First(&ib, id).Error; err != nil {
		t.Fatalf("reload inbound %d: %v", id, err)
	}
	return ib.Settings
}

const clientsWithPassword = `{"clients":[{"id":"u1","email":"a@x","password":"secret"}],"decryption":"none"}`

// TestSharedClientKeepsItsPassword is the case that makes this cleanup
// dangerous. client_inbounds is many-to-many, so one client can serve a VLESS
// inbound (where the password is dead weight) and a Trojan inbound (where it is
// the only credential). Clearing on the first would silently lock the client
// out of the second.
func TestSharedClientKeepsItsPassword(t *testing.T) {
	initCleanupDB(t)
	vless := seedInboundOfProtocol(t, "in-vless", 10001, model.VLESS, clientsWithPassword)
	trojan := seedInboundOfProtocol(t, "in-trojan", 10002, model.Trojan, clientsWithPassword)
	shared := seedClientLinkedTo(t, "shared@x", "still-needed", vless, trojan)

	if err := clearUnusedClientPasswords(); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	if got := passwordOf(t, shared.Id); got != "still-needed" {
		t.Errorf("password = %q, want it kept for the trojan inbound", got)
	}
	if got := settingsOf(t, trojan.Id); !strings.Contains(got, "secret") {
		t.Errorf("trojan inbound lost its client password: %s", got)
	}
}

// TestVlessOnlyClientIsCleared is the fix: a client that no password-using
// inbound serves has nothing to authenticate with it.
func TestVlessOnlyClientIsCleared(t *testing.T) {
	initCleanupDB(t)
	vless := seedInboundOfProtocol(t, "in-vless", 10001, model.VLESS, clientsWithPassword)
	vmess := seedInboundOfProtocol(t, "in-vmess", 10002, model.VMESS, clientsWithPassword)
	only := seedClientLinkedTo(t, "only@x", "leftover", vless, vmess)

	if err := clearUnusedClientPasswords(); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	if got := passwordOf(t, only.Id); got != "" {
		t.Errorf("password = %q, want it cleared", got)
	}
	for _, ib := range []*model.Inbound{vless, vmess} {
		if got := settingsOf(t, ib.Id); strings.Contains(got, "secret") {
			t.Errorf("%s kept a client password: %s", ib.Tag, got)
		}
		// The uuid is the actual credential on these protocols.
		if got := settingsOf(t, ib.Id); !strings.Contains(got, "u1") {
			t.Errorf("%s lost its client uuid: %s", ib.Tag, got)
		}
	}
}

// TestUnlinkedClientIsLeftAlone: with no client_inbounds rows there is nothing
// saying the password is unused, so the conservative choice is to keep it.
func TestUnlinkedClientIsLeftAlone(t *testing.T) {
	initCleanupDB(t)
	orphan := seedClientLinkedTo(t, "orphan@x", "unknown-purpose")

	if err := clearUnusedClientPasswords(); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	if got := passwordOf(t, orphan.Id); got != "unknown-purpose" {
		t.Errorf("password = %q, want an unlinked client left untouched", got)
	}
}

// TestCleanupIsIdempotent — the seeder is gated on its history row, but a
// second run must still be harmless.
func TestCleanupIsIdempotent(t *testing.T) {
	initCleanupDB(t)
	vless := seedInboundOfProtocol(t, "in-vless", 10001, model.VLESS, clientsWithPassword)
	trojan := seedInboundOfProtocol(t, "in-trojan", 10002, model.Trojan, clientsWithPassword)
	cleared := seedClientLinkedTo(t, "cleared@x", "leftover", vless)
	kept := seedClientLinkedTo(t, "kept@x", "still-needed", trojan)

	for range 2 {
		if err := clearUnusedClientPasswords(); err != nil {
			t.Fatalf("cleanup: %v", err)
		}
	}

	if got := passwordOf(t, cleared.Id); got != "" {
		t.Errorf("cleared password = %q, want empty", got)
	}
	if got := passwordOf(t, kept.Id); got != "still-needed" {
		t.Errorf("kept password = %q, want it preserved", got)
	}
}

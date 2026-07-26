package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

// setDeviceSettings writes the panel-wide HWID settings straight to the setting
// table, which is what SettingService reads.
func setDeviceSettings(t *testing.T, enable, forced bool, defaultLimit int) {
	t.Helper()
	values := map[string]string{
		"hwidEnable":       fmt.Sprintf("%t", enable),
		"hwidForced":       fmt.Sprintf("%t", forced),
		"hwidDefaultLimit": fmt.Sprintf("%d", defaultLimit),
	}
	db := database.GetDB()
	for k, v := range values {
		db.Where("key = ?", k).Delete(&model.Setting{})
		if err := db.Create(&model.Setting{Key: k, Value: v}).Error; err != nil {
			t.Fatalf("seed setting %s: %v", k, err)
		}
	}
}

func seedClientRecord(t *testing.T, email string, hwidLimit int) {
	t.Helper()
	rec := model.ClientRecord{Email: email, SubID: "sub-" + email, Enable: true, HwidLimit: hwidLimit}
	if err := database.GetDB().Create(&rec).Error; err != nil {
		t.Fatalf("seed client %s: %v", email, err)
	}
}

func device(hwid string) DeviceInfo {
	return DeviceInfo{Hwid: hwid, DeviceOS: "android", OSVersion: "14", DeviceModel: "Pixel", UserAgent: "happ/1.0", Ip: "10.0.0.1"}
}

// TestDeviceLimitEnforcement covers the decision table of the per-client device
// cap: disabled panel, known device, new device under/over the cap, and the
// per-client override winning over the panel default.
func TestDeviceLimitEnforcement(t *testing.T) {
	setupBulkDB(t)
	svc := &ClientDeviceService{}

	seedClientRecord(t, "alice@x", 0)
	seedClientRecord(t, "bob@x", 3)

	// Enforcement off: anything passes and nothing is recorded.
	setDeviceSettings(t, false, false, 1)
	if res, err := svc.Check("alice@x", device("dev-1")); err != nil || res != DeviceAllowed {
		t.Fatalf("disabled: got (%v, %v), want (DeviceAllowed, nil)", res, err)
	}
	if n, _ := svc.Count("alice@x"); n != 0 {
		t.Fatalf("disabled: expected no devices recorded, got %d", n)
	}

	// Enabled with a panel default of 2 devices.
	setDeviceSettings(t, true, false, 2)
	for _, hwid := range []string{"dev-1", "dev-2"} {
		if res, err := svc.Check("alice@x", device(hwid)); err != nil || res != DeviceAllowed {
			t.Fatalf("%s: got (%v, %v), want allowed", hwid, res, err)
		}
	}
	if n, _ := svc.Count("alice@x"); n != 2 {
		t.Fatalf("expected 2 devices, got %d", n)
	}

	// A bare re-fetch (apps commonly send only x-hwid after the first call)
	// must not blank out what we already know about the device.
	if _, err := svc.Check("alice@x", DeviceInfo{Hwid: "dev-1"}); err != nil {
		t.Fatalf("bare re-check: %v", err)
	}
	known, err := svc.List("alice@x")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, d := range known {
		if d.Hwid != "dev-1" {
			continue
		}
		if d.DeviceModel != "Pixel" || d.DeviceOS != "android" || d.OSVersion != "14" {
			t.Fatalf("bare re-check wiped device details: %+v", d)
		}
	}

	// A third device is refused...
	if res, _ := svc.Check("alice@x", device("dev-3")); res != DeviceLimitReached {
		t.Fatalf("third device: got %v, want DeviceLimitReached", res)
	}
	// ...while the devices already registered keep working.
	if res, _ := svc.Check("alice@x", device("dev-1")); res != DeviceAllowed {
		t.Fatalf("known device after cap: got %v, want DeviceAllowed", res)
	}
	if n, _ := svc.Count("alice@x"); n != 2 {
		t.Fatalf("re-check must not add a row, got %d devices", n)
	}

	// Per-client limit (3) overrides the panel default (2).
	for _, hwid := range []string{"b-1", "b-2", "b-3"} {
		if res, _ := svc.Check("bob@x", device(hwid)); res != DeviceAllowed {
			t.Fatalf("bob %s: want allowed under per-client limit 3", hwid)
		}
	}
	if res, _ := svc.Check("bob@x", device("b-4")); res != DeviceLimitReached {
		t.Fatalf("bob b-4: got %v, want DeviceLimitReached", res)
	}

	// Freeing one slot lets a new device in again.
	devices, err := svc.List("alice@x")
	if err != nil || len(devices) != 2 {
		t.Fatalf("List: got %d devices, err %v", len(devices), err)
	}
	if err := svc.Delete("alice@x", devices[0].Id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if res, _ := svc.Check("alice@x", device("dev-3")); res != DeviceAllowed {
		t.Fatalf("after freeing a slot: got %v, want DeviceAllowed", res)
	}

	// Clearing forgets everything.
	removed, err := svc.Clear("alice@x")
	if err != nil || removed != 2 {
		t.Fatalf("Clear: removed %d, err %v; want 2", removed, err)
	}
	if n, _ := svc.Count("alice@x"); n != 0 {
		t.Fatalf("after Clear: %d devices left", n)
	}
}

// TestDeviceLimitUnlimitedAndForced covers the two settings that don't depend on
// the count: an effective limit of 0 (unlimited) and the "HWID required" switch.
func TestDeviceLimitUnlimitedAndForced(t *testing.T) {
	setupBulkDB(t)
	svc := &ClientDeviceService{}
	seedClientRecord(t, "carol@x", 0)

	// Limit 0 everywhere = unlimited, but devices are still recorded so an
	// admin can see what connected.
	setDeviceSettings(t, true, false, 0)
	for i := range 5 {
		if res, _ := svc.Check("carol@x", device(fmt.Sprintf("d-%d", i))); res != DeviceAllowed {
			t.Fatalf("unlimited: device %d was refused", i)
		}
	}
	if n, _ := svc.Count("carol@x"); n != 5 {
		t.Fatalf("unlimited: expected 5 recorded devices, got %d", n)
	}

	// No HWID header: allowed while "forced" is off, refused once it is on.
	if res, _ := svc.Check("carol@x", DeviceInfo{}); res != DeviceAllowed {
		t.Fatal("missing hwid must pass while forced is off")
	}
	setDeviceSettings(t, true, true, 0)
	if res, _ := svc.Check("carol@x", DeviceInfo{}); res != DeviceMissingHwid {
		t.Fatal("missing hwid must be refused while forced is on")
	}
	// A request that does identify itself still passes with forced on.
	if res, _ := svc.Check("carol@x", device("d-0")); res != DeviceAllowed {
		t.Fatal("known device must pass with forced on")
	}
}

// TestDeviceLookupsAndCleanup covers the plumbing the UI and the client
// lifecycle depend on: subId -> email, batch counts, rename, and delete.
func TestDeviceLookupsAndCleanup(t *testing.T) {
	setupBulkDB(t)
	svc := &ClientDeviceService{}
	setDeviceSettings(t, true, false, 5)
	seedClientRecord(t, "dave@x", 0)
	seedClientRecord(t, "erin@x", 0)

	if _, err := svc.Check("dave@x", device("d-1")); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if _, err := svc.Check("erin@x", device("e-1")); err != nil {
		t.Fatalf("Check: %v", err)
	}

	email, err := svc.EmailBySubID("sub-dave@x")
	if err != nil || email != "dave@x" {
		t.Fatalf("EmailBySubID: got (%q, %v), want dave@x", email, err)
	}
	if email, err := svc.EmailBySubID("nope"); err != nil || email != "" {
		t.Fatalf("unknown subId: got (%q, %v), want empty without error", email, err)
	}

	counts, err := svc.CountByEmails([]string{"dave@x", "erin@x", "ghost@x"})
	if err != nil {
		t.Fatalf("CountByEmails: %v", err)
	}
	if counts["dave@x"] != 1 || counts["erin@x"] != 1 {
		t.Fatalf("CountByEmails: got %v", counts)
	}
	if _, ok := counts["ghost@x"]; ok {
		t.Fatal("CountByEmails must not invent rows for clients with no devices")
	}

	// A rename carries the devices along instead of resetting them.
	if err := svc.RenameEmail(nil, "dave@x", "dave2@x"); err != nil {
		t.Fatalf("RenameEmail: %v", err)
	}
	if n, _ := svc.Count("dave2@x"); n != 1 {
		t.Fatalf("after rename: got %d devices on the new email, want 1", n)
	}
	if n, _ := svc.Count("dave@x"); n != 0 {
		t.Fatalf("after rename: %d devices left on the old email", n)
	}

	// Deleting the client drops its devices.
	if err := svc.DeleteForEmails(nil, []string{"erin@x"}); err != nil {
		t.Fatalf("DeleteForEmails: %v", err)
	}
	if n, _ := svc.Count("erin@x"); n != 0 {
		t.Fatalf("after DeleteForEmails: %d devices left", n)
	}
}

// TestExpiredEmailsOlderThan pins the selection rule behind the auto-delete
// job: only date-expired clients age, the grace period is respected, and both
// auto-renew rows and quota-exhausted clients are left alone.
func TestExpiredEmailsOlderThan(t *testing.T) {
	setupBulkDB(t)
	svc := &ClientService{}

	day := int64(24 * 60 * 60 * 1000)
	now := time.Now().UnixMilli()
	seed := func(email string, expiry int64, reset int, up, total int64) {
		t.Helper()
		row := xray.ClientTraffic{Email: email, Enable: true, ExpiryTime: expiry, Reset: reset, Up: up, Total: total}
		if err := database.GetDB().Create(&row).Error; err != nil {
			t.Fatalf("seed traffic %s: %v", email, err)
		}
	}

	seed("long-expired@x", now-10*day, 0, 0, 0)
	seed("just-expired@x", now-1*day, 0, 0, 0)
	seed("active@x", now+10*day, 0, 0, 0)
	seed("never-expires@x", 0, 0, 0, 0)
	seed("renewing@x", now-10*day, 30, 0, 0)
	seed("exhausted@x", 0, 0, 100, 100)

	// Zero grace is the "never delete" setting, not "delete everything".
	if emails, err := svc.ExpiredEmailsOlderThan(0); err != nil || len(emails) != 0 {
		t.Fatalf("grace 0: got %v (err %v), want nothing", emails, err)
	}
	if emails, err := svc.ExpiredEmailsOlderThan(-5); err != nil || len(emails) != 0 {
		t.Fatalf("negative grace: got %v (err %v), want nothing", emails, err)
	}

	// A 7-day grace catches only the client that has been expired that long.
	emails, err := svc.ExpiredEmailsOlderThan(7)
	if err != nil {
		t.Fatalf("ExpiredEmailsOlderThan(7): %v", err)
	}
	if len(emails) != 1 || emails[0] != "long-expired@x" {
		t.Fatalf("grace 7: got %v, want [long-expired@x]", emails)
	}

	// Shrinking the grace to a day also catches the recently expired one, but
	// still never the renewing or quota-exhausted clients.
	emails, err = svc.ExpiredEmailsOlderThan(1)
	if err != nil {
		t.Fatalf("ExpiredEmailsOlderThan(1): %v", err)
	}
	got := map[string]bool{}
	for _, e := range emails {
		got[e] = true
	}
	if !got["long-expired@x"] || !got["just-expired@x"] {
		t.Fatalf("grace 1: got %v, want both expired clients", emails)
	}
	for _, unwanted := range []string{"active@x", "never-expires@x", "renewing@x", "exhausted@x"} {
		if got[unwanted] {
			t.Fatalf("grace 1: %s must not be selected (got %v)", unwanted, emails)
		}
	}
}

// TestDeleteExpiredOlderThanRemovesOnlyAgedClients runs the auto-delete path
// end to end against real inbound settings.
func TestDeleteExpiredOlderThanRemovesOnlyAgedClients(t *testing.T) {
	setupBulkDB(t)
	svc := &ClientService{}
	inboundSvc := &InboundService{}

	day := int64(24 * 60 * 60 * 1000)
	now := time.Now().UnixMilli()
	clients := []model.Client{
		{Email: "old@x", ID: "11111111-1111-1111-1111-111111111111", Enable: true},
		{Email: "fresh@x", ID: "22222222-2222-2222-2222-222222222222", Enable: true},
	}
	ib := mkInbound(t, 22001, model.VLESS, clientsSettings(t, clients))
	if err := svc.SyncInbound(nil, ib.Id, clients); err != nil {
		t.Fatalf("seed linkage: %v", err)
	}
	for _, row := range []xray.ClientTraffic{
		{InboundId: ib.Id, Email: "old@x", Enable: true, ExpiryTime: now - 30*day},
		{InboundId: ib.Id, Email: "fresh@x", Enable: true, ExpiryTime: now - 1*day},
	} {
		if err := database.GetDB().Create(&row).Error; err != nil {
			t.Fatalf("seed traffic: %v", err)
		}
	}

	deleted, _, err := svc.DeleteExpiredOlderThan(inboundSvc, 7)
	if err != nil {
		t.Fatalf("DeleteExpiredOlderThan: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}

	after, err := inboundSvc.GetInbound(ib.Id)
	if err != nil {
		t.Fatalf("GetInbound: %v", err)
	}
	remaining, err := inboundSvc.GetClients(after)
	if err != nil {
		t.Fatalf("GetClients: %v", err)
	}
	if len(remaining) != 1 || remaining[0].Email != "fresh@x" {
		t.Fatalf("remaining = %v, want [fresh@x]", sortedEmails(remaining))
	}
}

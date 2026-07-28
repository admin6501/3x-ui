package service

import (
	"path/filepath"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"

	"gorm.io/gorm"
)

// attachClient links an email to inbounds the way the panel does, so the
// multiplier lookup has something to join through.
func attachClient(t *testing.T, email string, inboundIds ...int) {
	t.Helper()
	db := database.GetDB()
	rec := &model.ClientRecord{Email: email, SubID: "sub-" + email, Enable: true}
	if err := db.Create(rec).Error; err != nil {
		t.Fatalf("create client record %s: %v", email, err)
	}
	for _, id := range inboundIds {
		if err := db.Create(&model.ClientInbound{ClientId: rec.Id, InboundId: id}).Error; err != nil {
			t.Fatalf("attach %s to %d: %v", email, id, err)
		}
	}
	if err := db.Create(&xray.ClientTraffic{InboundId: inboundIds[0], Email: email, Enable: true}).Error; err != nil {
		t.Fatalf("create client_traffics %s: %v", email, err)
	}
}

func mkMultiplierDB(t *testing.T) *gorm.DB {
	t.Helper()
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	if err := database.InitDB(filepath.Join(dbDir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
	return database.GetDB()
}

// TestTrafficMultiplierChargesMoreThanWasMoved is the feature: on an inbound set
// to 2, a client who moves a gigabyte has two taken off their quota.
func TestTrafficMultiplierChargesMoreThanWasMoved(t *testing.T) {
	db := mkMultiplierDB(t)

	plain := &model.Inbound{UserId: 1, Tag: "plain", Enable: true, Port: 41001, Protocol: model.VLESS, TrafficMultiplier: 1}
	doubled := &model.Inbound{UserId: 1, Tag: "doubled", Enable: true, Port: 41002, Protocol: model.VLESS, TrafficMultiplier: 2}
	for _, ib := range []*model.Inbound{plain, doubled} {
		if err := db.Create(ib).Error; err != nil {
			t.Fatalf("create inbound %s: %v", ib.Tag, err)
		}
	}
	attachClient(t, "normal@example.com", plain.Id)
	attachClient(t, "expensive@example.com", doubled.Id)

	svc := InboundService{}
	if err := svc.addClientTraffic(db, []*xray.ClientTraffic{
		{Email: "normal@example.com", Up: 1000, Down: 2000},
		{Email: "expensive@example.com", Up: 1000, Down: 2000},
	}); err != nil {
		t.Fatalf("addClientTraffic: %v", err)
	}

	for _, tc := range []struct {
		email            string
		wantUp, wantDown int64
	}{
		{"normal@example.com", 1000, 2000},
		{"expensive@example.com", 2000, 4000},
	} {
		var row xray.ClientTraffic
		if err := db.Model(xray.ClientTraffic{}).Where("email = ?", tc.email).First(&row).Error; err != nil {
			t.Fatalf("reload %s: %v", tc.email, err)
		}
		if row.Up != tc.wantUp || row.Down != tc.wantDown {
			t.Errorf("%s: up=%d down=%d, want %d/%d", tc.email, row.Up, row.Down, tc.wantUp, tc.wantDown)
		}
	}
}

// TestMultiAttachedClientPaysTheHighestRate: xray counts per email, not per
// inbound, so a client on several inbounds arrives as one number that cannot be
// split. Charging the lowest rate would let anyone dodge an expensive inbound by
// also attaching to a cheap one.
func TestMultiAttachedClientPaysTheHighestRate(t *testing.T) {
	db := mkMultiplierDB(t)

	cheap := &model.Inbound{UserId: 1, Tag: "cheap", Enable: true, Port: 41011, Protocol: model.VLESS, TrafficMultiplier: 1}
	pricey := &model.Inbound{UserId: 1, Tag: "pricey", Enable: true, Port: 41012, Protocol: model.VLESS, TrafficMultiplier: 3}
	for _, ib := range []*model.Inbound{cheap, pricey} {
		if err := db.Create(ib).Error; err != nil {
			t.Fatalf("create inbound %s: %v", ib.Tag, err)
		}
	}
	attachClient(t, "both@example.com", cheap.Id, pricey.Id)

	svc := InboundService{}
	if err := svc.addClientTraffic(db, []*xray.ClientTraffic{
		{Email: "both@example.com", Up: 100, Down: 100},
	}); err != nil {
		t.Fatalf("addClientTraffic: %v", err)
	}

	var row xray.ClientTraffic
	if err := db.Model(xray.ClientTraffic{}).Where("email = ?", "both@example.com").First(&row).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if row.Up != 300 || row.Down != 300 {
		t.Errorf("up=%d down=%d, want 300/300 (the highest attached multiplier)", row.Up, row.Down)
	}
}

// TestMultiplierIsInertUntilSomeoneSetsOne: an inbound left at 1 — and one
// created before the column existed, which reads 0 — counts traffic as measured.
func TestMultiplierIsInertUntilSomeoneSetsOne(t *testing.T) {
	db := mkMultiplierDB(t)

	one := &model.Inbound{UserId: 1, Tag: "one", Enable: true, Port: 41021, Protocol: model.VLESS, TrafficMultiplier: 1}
	if err := db.Create(one).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	legacy := &model.Inbound{UserId: 1, Tag: "legacy", Enable: true, Port: 41022, Protocol: model.VLESS}
	if err := db.Create(legacy).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	// Force the pre-migration value a legacy row would carry.
	if err := db.Model(&model.Inbound{}).Where("id = ?", legacy.Id).
		Update("traffic_multiplier", 0).Error; err != nil {
		t.Fatalf("clear multiplier: %v", err)
	}
	attachClient(t, "a@example.com", one.Id)
	attachClient(t, "b@example.com", legacy.Id)

	svc := InboundService{}
	if m := svc.trafficMultipliersByEmail([]string{"a@example.com", "b@example.com"}); m != nil {
		t.Errorf("no inbound has a multiplier, but got %v", m)
	}
	if err := svc.addClientTraffic(db, []*xray.ClientTraffic{
		{Email: "a@example.com", Up: 7, Down: 9},
		{Email: "b@example.com", Up: 7, Down: 9},
	}); err != nil {
		t.Fatalf("addClientTraffic: %v", err)
	}
	for _, email := range []string{"a@example.com", "b@example.com"} {
		var row xray.ClientTraffic
		if err := db.Model(xray.ClientTraffic{}).Where("email = ?", email).First(&row).Error; err != nil {
			t.Fatalf("reload %s: %v", email, err)
		}
		if row.Up != 7 || row.Down != 9 {
			t.Errorf("%s: up=%d down=%d, want 7/9 untouched", email, row.Up, row.Down)
		}
	}
}

// TestMultiplierArithmetic pins the rounding and the bounds.
func TestMultiplierArithmetic(t *testing.T) {
	cases := []struct {
		bytes int64
		mult  float64
		want  int64
	}{
		{1000, 2, 2000},
		{1000, 1, 1000},
		{1000, 0.5, 500},
		{1000, 1.5, 1500},
		{3, 1.5, 5}, // 4.5 rounds to nearest, not down
		{1, 0.1, 1}, // real traffic never rounds away to free
		{0, 2, 0},   // nothing moved, nothing charged
		{-5, 2, -5}, // a negative delta is not ours to scale
	}
	for _, tc := range cases {
		if got := ApplyTrafficMultiplier(tc.bytes, tc.mult); got != tc.want {
			t.Errorf("ApplyTrafficMultiplier(%d, %v) = %d, want %d", tc.bytes, tc.mult, got, tc.want)
		}
	}

	// Zero and negatives mean "unset", never "this inbound is free".
	for _, in := range []float64{0, -1, -0.5} {
		if got := NormalizeTrafficMultiplier(in); got != 1 {
			t.Errorf("NormalizeTrafficMultiplier(%v) = %v, want 1", in, got)
		}
	}
	if got := NormalizeTrafficMultiplier(1000); got != maxTrafficMultiplier {
		t.Errorf("NormalizeTrafficMultiplier(1000) = %v, want %v", got, maxTrafficMultiplier)
	}
	if got := NormalizeTrafficMultiplier(2.5); got != 2.5 {
		t.Errorf("NormalizeTrafficMultiplier(2.5) = %v, want 2.5", got)
	}
}

// TestUpdatingAnInboundKeepsItsMultiplier: the inbound edit form is not the only
// caller of Update, and one that does not know about the field must not silently
// reset a paying inbound back to 1.
func TestUpdatingAnInboundKeepsItsMultiplier(t *testing.T) {
	db := mkMultiplierDB(t)
	svc := InboundService{}

	ib := &model.Inbound{
		UserId: 1, Tag: "keeps", Enable: true, Port: 41031, Protocol: model.VLESS,
		Settings: `{"clients":[]}`, TrafficMultiplier: 2,
	}
	if err := db.Create(ib).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	// An update that leaves the field at its zero value.
	patch := *ib
	patch.TrafficMultiplier = 0
	patch.Remark = "renamed"
	if _, _, err := svc.UpdateInbound(&patch); err != nil {
		t.Fatalf("UpdateInbound: %v", err)
	}
	reloaded, err := svc.GetInbound(ib.Id)
	if err != nil {
		t.Fatalf("GetInbound: %v", err)
	}
	if reloaded.TrafficMultiplier != 2 {
		t.Errorf("multiplier = %v after an unrelated update, want 2", reloaded.TrafficMultiplier)
	}

	// An update that does set it takes effect.
	patch.TrafficMultiplier = 3
	if _, _, err := svc.UpdateInbound(&patch); err != nil {
		t.Fatalf("UpdateInbound: %v", err)
	}
	reloaded, _ = svc.GetInbound(ib.Id)
	if reloaded.TrafficMultiplier != 3 {
		t.Errorf("multiplier = %v, want 3", reloaded.TrafficMultiplier)
	}
}

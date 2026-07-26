package job

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	xuilogger "github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"

	"github.com/op/go-logging"
)

// The job logs what it deleted; without a logger backend those calls panic on
// a nil logger, so every test in this file needs one initialised.
var jobLoggerOnce sync.Once

func setupJobDB(t *testing.T) {
	t.Helper()
	jobLoggerOnce.Do(func() { xuilogger.InitLogger(logging.ERROR) })
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	if err := database.InitDB(filepath.Join(dbDir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
}

func setAutoDeleteSettings(t *testing.T, enable bool, days int) {
	t.Helper()
	db := database.GetDB()
	for key, value := range map[string]string{
		"autoDeleteExpiredEnable": fmt.Sprintf("%t", enable),
		"autoDeleteExpiredDays":   fmt.Sprintf("%d", days),
	} {
		db.Where("key = ?", key).Delete(&model.Setting{})
		if err := db.Create(&model.Setting{Key: key, Value: value}).Error; err != nil {
			t.Fatalf("seed setting %s: %v", key, err)
		}
	}
}

// seedExpiredClient creates an inbound holding one client plus its traffic row,
// so the job has something real to delete.
func seedExpiredClient(t *testing.T, port int, email string, expiryTime int64) {
	t.Helper()
	clients := []model.Client{{Email: email, ID: "11111111-1111-1111-1111-11111111111" + fmt.Sprint(port%10), Enable: true, ExpiryTime: expiryTime}}
	payload, err := json.Marshal(map[string][]model.Client{"clients": clients})
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	ib := &model.Inbound{Tag: fmt.Sprintf("in-%d", port), Enable: true, Port: port, Protocol: model.VLESS, Settings: string(payload)}
	if err := database.GetDB().Create(ib).Error; err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	clientSvc := service.ClientService{}
	if err := clientSvc.SyncInbound(nil, ib.Id, clients); err != nil {
		t.Fatalf("SyncInbound: %v", err)
	}
	row := xray.ClientTraffic{InboundId: ib.Id, Email: email, Enable: true, ExpiryTime: expiryTime}
	if err := database.GetDB().Create(&row).Error; err != nil {
		t.Fatalf("create traffic row: %v", err)
	}
}

func clientExists(t *testing.T, email string) bool {
	t.Helper()
	var count int64
	if err := database.GetDB().Model(&model.ClientRecord{}).Where("email = ?", email).Count(&count).Error; err != nil {
		t.Fatalf("count clients: %v", err)
	}
	return count > 0
}

// TestAutoDeleteExpiredClientsJobGuards is the safety contract of the job: it
// must delete nothing at all unless an admin both switched it on and set a
// non-zero grace period.
func TestAutoDeleteExpiredClientsJobGuards(t *testing.T) {
	setupJobDB(t)
	longAgo := time.Now().AddDate(0, 0, -60).UnixMilli()
	seedExpiredClient(t, 26001, "ancient@x", longAgo)

	job := NewAutoDeleteExpiredClientsJob()

	// Defaults (nothing configured): the client survives.
	job.Run()
	if !clientExists(t, "ancient@x") {
		t.Fatal("job deleted a client with no settings configured")
	}

	// Switched off but with a grace period set.
	setAutoDeleteSettings(t, false, 7)
	job.Run()
	if !clientExists(t, "ancient@x") {
		t.Fatal("job deleted a client while switched off")
	}

	// Switched on but left at zero days — the "never delete" setting.
	setAutoDeleteSettings(t, true, 0)
	job.Run()
	if !clientExists(t, "ancient@x") {
		t.Fatal("job deleted a client with a zero-day grace period")
	}

	// Both guards satisfied: now it goes.
	setAutoDeleteSettings(t, true, 7)
	job.Run()
	if clientExists(t, "ancient@x") {
		t.Fatal("job did not delete a client expired 60 days ago with a 7-day grace")
	}
}

// TestAutoDeleteExpiredClientsJobRespectsGrace checks the job only sweeps
// clients past the configured age and leaves everyone else alone.
func TestAutoDeleteExpiredClientsJobRespectsGrace(t *testing.T) {
	setupJobDB(t)
	now := time.Now()
	seedExpiredClient(t, 26011, "old@x", now.AddDate(0, 0, -30).UnixMilli())
	seedExpiredClient(t, 26012, "recent@x", now.AddDate(0, 0, -2).UnixMilli())
	seedExpiredClient(t, 26013, "future@x", now.AddDate(0, 0, 30).UnixMilli())
	seedExpiredClient(t, 26014, "forever@x", 0)

	setAutoDeleteSettings(t, true, 7)
	NewAutoDeleteExpiredClientsJob().Run()

	if clientExists(t, "old@x") {
		t.Error("a client expired 30 days ago should have been deleted")
	}
	for _, survivor := range []string{"recent@x", "future@x", "forever@x"} {
		if !clientExists(t, survivor) {
			t.Errorf("%s must not be deleted with a 7-day grace period", survivor)
		}
	}
}

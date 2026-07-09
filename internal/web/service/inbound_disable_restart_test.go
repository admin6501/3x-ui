package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// Disabling a local, enabled inbound must report needRestart=true. The runtime
// API DelInbound only closes the listener; connections already established keep
// flowing until xray fully restarts, so a disabled inbound's clients used to
// stay connected (reported bug). needRestart forces the full restart that
// actually cuts them off.
func TestSetInboundEnableDisableForcesRestart(t *testing.T) {
	setupBulkDB(t)
	svc := &InboundService{}

	source := []model.Client{
		{Email: "alice@x", ID: "11111111-1111-1111-1111-111111111111", SubID: "sa", Enable: true},
	}
	ib := mkInbound(t, 21001, model.VLESS, clientsSettings(t, source))

	// Disable it: expect a restart to be requested.
	needRestart, err := svc.SetInboundEnable(ib.Id, false)
	if err != nil {
		t.Fatalf("SetInboundEnable(false): %v", err)
	}
	if !needRestart {
		t.Fatalf("disabling a local enabled inbound must request a restart so live client connections are dropped")
	}

	// The row must actually be marked disabled.
	got, err := svc.GetInbound(ib.Id)
	if err != nil {
		t.Fatalf("GetInbound: %v", err)
	}
	if got.Enable {
		t.Fatalf("inbound should be disabled after SetInboundEnable(false)")
	}

	// No-op toggle (already disabled) should not request a restart.
	needRestart, err = svc.SetInboundEnable(ib.Id, false)
	if err != nil {
		t.Fatalf("SetInboundEnable(false) no-op: %v", err)
	}
	if needRestart {
		t.Fatalf("no-op disable must not request a restart")
	}
	_ = database.GetDB()
}

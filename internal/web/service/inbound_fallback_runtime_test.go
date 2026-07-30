package service

import (
	"encoding/json"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// TestRuntimeInboundCarriesFallbacks is the regression for fallback rules that
// exist on the master but never reach a node.
//
// Fallbacks live in the inbound_fallbacks table and were folded into settings
// only by the master's own config builder. The runtime inbound pushed to a node
// went out without them, so the rules an operator configured simply did not
// exist there and every connection that should have been handed to a child
// inbound was dropped instead.
func TestRuntimeInboundCarriesFallbacks(t *testing.T) {
	setupConflictDB(t)
	db := database.GetDB()
	svc := InboundService{}

	master := &model.Inbound{
		Tag: "master", Enable: true, Port: 20443, Protocol: model.VLESS,
		StreamSettings: `{"network":"tcp","security":"tls"}`,
		Settings:       `{"clients":[],"decryption":"none"}`,
	}
	if err := db.Create(master).Error; err != nil {
		t.Fatalf("seed master: %v", err)
	}
	child := &model.Inbound{
		Tag: "child", Enable: true, Port: 20444, Protocol: model.VLESS,
		StreamSettings: `{"network":"ws"}`, Settings: `{"clients":[],"decryption":"none"}`,
	}
	if err := db.Create(child).Error; err != nil {
		t.Fatalf("seed child: %v", err)
	}
	if err := db.Create(&model.InboundFallback{
		MasterId: master.Id, ChildId: child.Id, Path: "/vlws", Xver: 2,
	}).Error; err != nil {
		t.Fatalf("seed fallback: %v", err)
	}

	runtimeInbound, err := svc.buildRuntimeInboundForAPI(db, master)
	if err != nil {
		t.Fatalf("buildRuntimeInboundForAPI: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal([]byte(runtimeInbound.Settings), &settings); err != nil {
		t.Fatalf("runtime settings are not valid json: %v", err)
	}
	fallbacks, ok := settings["fallbacks"].([]any)
	if !ok || len(fallbacks) != 1 {
		t.Fatalf("runtime inbound carries no fallbacks: %s", runtimeInbound.Settings)
	}
	rule, _ := fallbacks[0].(map[string]any)
	if rule["path"] != "/vlws" {
		t.Errorf("fallback path = %v, want /vlws", rule["path"])
	}
}

// TestRuntimeInboundWithoutFallbacksIsUnchanged: an inbound with no rules must
// not gain an empty fallbacks key, which xray would reject.
func TestRuntimeInboundWithoutFallbacksIsUnchanged(t *testing.T) {
	setupConflictDB(t)
	db := database.GetDB()
	svc := InboundService{}

	plain := &model.Inbound{
		Tag: "plain", Enable: true, Port: 20445, Protocol: model.VLESS,
		StreamSettings: `{"network":"tcp","security":"tls"}`,
		Settings:       `{"clients":[],"decryption":"none"}`,
	}
	if err := db.Create(plain).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	runtimeInbound, err := svc.buildRuntimeInboundForAPI(db, plain)
	if err != nil {
		t.Fatalf("buildRuntimeInboundForAPI: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal([]byte(runtimeInbound.Settings), &settings); err != nil {
		t.Fatalf("not valid json: %v", err)
	}
	if _, present := settings["fallbacks"]; present {
		t.Errorf("added a fallbacks key with no rules: %s", runtimeInbound.Settings)
	}
}

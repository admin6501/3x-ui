package job

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// TestHeartbeatBroadcastPayloadRedactsApiToken is the regression guard for the
// node-credential leak: the heartbeat pushes its payload to every connected
// WebSocket session regardless of role, so a raw model.Node — which serialises
// ApiToken — would hand every reseller and read-only account the credentials to
// administer every node.
func TestHeartbeatBroadcastPayloadRedactsApiToken(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dir)
	if err := database.InitDB(filepath.Join(dir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })

	const secret = "NODE-API-TOKEN-DO-NOT-LEAK"
	node := &model.Node{Name: "de-fra-1", Address: "node1.example.com", Port: 2053, ApiToken: secret}
	if err := database.GetDB().Create(node).Error; err != nil {
		t.Fatalf("create node: %v", err)
	}

	payload, err := NewNodeHeartbeatJob().broadcastPayload()
	if err != nil {
		t.Fatalf("broadcastPayload: %v", err)
	}
	if len(payload) != 1 {
		t.Fatalf("got %d nodes, want 1", len(payload))
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(encoded), secret) {
		t.Fatalf("broadcast payload carries the node API token: %s", encoded)
	}

	var back []map[string]any
	if err := json.Unmarshal(encoded, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := back[0]["apiToken"]; got != "" {
		t.Errorf("apiToken = %v, want empty", got)
	}
	if got := back[0]["hasApiToken"]; got != true {
		t.Errorf("hasApiToken = %v, want true so the edit form still knows one is stored", got)
	}
}

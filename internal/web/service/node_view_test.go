package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// TestNodeViewRedactsTheApiToken is the whole point: a node's API token is
// full control of that node's own panel, and the node list is readable by
// every signed-in role because the Inbounds, Hosts and Clients pages all need
// to name the node an inbound lives on. Serialising the raw model there handed
// a manager, a reseller or a read-only account the credentials to administer
// every node.
func TestNodeViewRedactsTheApiToken(t *testing.T) {
	node := &model.Node{Id: 1, Name: "de-fra-1", ApiToken: "super-secret-token"}

	encoded, err := json.Marshal(NodeViewOf(node))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(encoded), "super-secret-token") {
		t.Fatalf("the API token reached the response body:\n%s", encoded)
	}

	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded["apiToken"] != "" {
		t.Errorf("apiToken = %v, want an empty string", decoded["apiToken"])
	}
	if decoded["hasApiToken"] != true {
		t.Error("hasApiToken must be true so the edit form knows one is stored")
	}
	// The rest of the node still has to be there — this is the read contract
	// the Nodes page renders from.
	if decoded["name"] != "de-fra-1" {
		t.Errorf("name = %v, want de-fra-1", decoded["name"])
	}
}

// TestNodeViewReportsAMissingToken: an mTLS node has no token, and the form
// must not be told one is stored or it would let the field stay blank.
func TestNodeViewReportsAMissingToken(t *testing.T) {
	encoded, err := json.Marshal(NodeViewOf(&model.Node{Id: 2, Name: "mtls-node", TlsVerifyMode: "mtls"}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded["hasApiToken"] != false {
		t.Error("hasApiToken must be false when no token is stored")
	}
}

// TestNodeViewsRedactsEveryNode guards the list endpoint, which is where the
// exposure was widest.
func TestNodeViewsRedactsEveryNode(t *testing.T) {
	views := NodeViews([]*model.Node{
		{Id: 1, ApiToken: "token-a"},
		{Id: 2, ApiToken: "token-b"},
		{Id: 3},
	})
	encoded, err := json.Marshal(views)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, secret := range []string{"token-a", "token-b"} {
		if strings.Contains(string(encoded), secret) {
			t.Errorf("%s leaked in the list response", secret)
		}
	}
	if len(views) != 3 {
		t.Errorf("returned %d views, want 3", len(views))
	}
}

// TestUpdateKeepsTheStoredTokenWhenBlank is the other half of write-only
// tokens, and the one that would break every node if it were wrong.
//
// The edit form is never given the token, so it submits an empty field. If
// Update wrote that through, saving any unrelated change — a rename, a sync
// mode switch — would wipe the credential and the node would go unreachable on
// the next tick.
func TestUpdateKeepsTheStoredTokenWhenBlank(t *testing.T) {
	setupConflictDB(t)
	svc := &NodeService{}

	original := &model.Node{
		Name: "keeper", Scheme: "https", Address: "node.example.com", Port: 2053,
		BasePath: "/", ApiToken: "stored-token", Enable: true, AllowPrivateAddress: true,
	}
	if err := svc.Create(original); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// What the edit form sends back: everything but the token.
	edited := *original
	edited.Remark = "renamed"
	edited.ApiToken = ""
	if err := svc.Update(original.Id, &edited); err != nil {
		t.Fatalf("Update: %v", err)
	}

	after, err := svc.GetById(original.Id)
	if err != nil {
		t.Fatalf("GetById: %v", err)
	}
	if after.ApiToken != "stored-token" {
		t.Fatalf("token = %q after a blank submit; the node is now unreachable", after.ApiToken)
	}
	if after.Remark != "renamed" {
		t.Errorf("the edit itself was not applied: remark = %q", after.Remark)
	}
}

// TestUpdateAcceptsANewToken: blank means unchanged, but a value still has to
// replace the old one or the token could never be rotated.
func TestUpdateAcceptsANewToken(t *testing.T) {
	setupConflictDB(t)
	svc := &NodeService{}

	original := &model.Node{
		Name: "rotator", Scheme: "https", Address: "node2.example.com", Port: 2053,
		BasePath: "/", ApiToken: "old-token", Enable: true, AllowPrivateAddress: true,
	}
	if err := svc.Create(original); err != nil {
		t.Fatalf("Create: %v", err)
	}

	edited := *original
	edited.ApiToken = "rotated-token"
	if err := svc.Update(original.Id, &edited); err != nil {
		t.Fatalf("Update: %v", err)
	}

	after, err := svc.GetById(original.Id)
	if err != nil {
		t.Fatalf("GetById: %v", err)
	}
	if after.ApiToken != "rotated-token" {
		t.Errorf("token = %q, want the rotated value", after.ApiToken)
	}
}

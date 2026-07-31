package websocket

import (
	"encoding/json"
	"testing"
	"time"
)

// readOne waits briefly for a message on the client's Send channel.
func readOne(t *testing.T, c *Client) (Message, bool) {
	t.Helper()
	select {
	case raw, ok := <-c.Send:
		if !ok {
			t.Fatalf("client %s send channel closed", c.ID)
		}
		var msg Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			t.Fatalf("unmarshal broadcast: %v", err)
		}
		return msg, true
	case <-time.After(time.Second):
		return Message{}, false
	}
}

func newRunningHub(t *testing.T) *Hub {
	t.Helper()
	h := NewHub()
	go h.Run()
	t.Cleanup(func() { h.cancel() })
	return h
}

// registerAndSettle registers clients and waits for the hub's ops loop to pick
// them up, so a broadcast issued right after is not raced against registration.
func registerAndSettle(t *testing.T, h *Hub, clients ...*Client) {
	t.Helper()
	for _, c := range clients {
		h.Register(c)
	}
	deadline := time.Now().Add(time.Second)
	for h.GetClientCount() < len(clients) {
		if time.Now().After(deadline) {
			t.Fatalf("hub registered %d of %d clients", h.GetClientCount(), len(clients))
		}
		time.Sleep(time.Millisecond)
	}
}

// TestBroadcast_ScopedClientGetsInvalidateNotInbounds is the regression guard
// for the reseller scope bypass: inbound payloads carry settings JSON with
// every client's credentials, so a scoped session must be told to re-fetch
// through the scope-filtered REST endpoint instead of being handed the payload.
func TestBroadcast_ScopedClientGetsInvalidateNotInbounds(t *testing.T) {
	h := newRunningHub(t)
	unscoped := NewClient("admin")
	scoped := NewScopedClient("reseller")
	registerAndSettle(t, h, unscoped, scoped)

	secret := []map[string]any{{"id": 7, "settings": `{"clients":[{"id":"SECRET-UUID"}]}`}}
	h.Broadcast(MessageTypeInbounds, secret)

	got, ok := readOne(t, unscoped)
	if !ok {
		t.Fatal("unscoped client received nothing")
	}
	if got.Type != MessageTypeInbounds {
		t.Fatalf("unscoped client: got type %q, want %q", got.Type, MessageTypeInbounds)
	}

	got, ok = readOne(t, scoped)
	if !ok {
		t.Fatal("scoped client received nothing; it should get an invalidate signal")
	}
	if got.Type != MessageTypeInvalidate {
		t.Fatalf("scoped client received %q — scope-sensitive payload leaked", got.Type)
	}
	payload, _ := got.Payload.(map[string]any)
	if payload["type"] != string(MessageTypeInbounds) {
		t.Fatalf("scoped client: invalidate names %v, want %q", payload["type"], MessageTypeInbounds)
	}
}

// TestBroadcast_ScopedClientNeverSeesClientWideStats covers the other two
// panel-wide feeds. Both are re-fetched from the clients collection, which the
// REST layer restricts to the reseller's inbounds.
func TestBroadcast_ScopedClientNeverSeesClientWideStats(t *testing.T) {
	for _, msgType := range []MessageType{MessageTypeTraffic, MessageTypeClientStats} {
		t.Run(string(msgType), func(t *testing.T) {
			h := newRunningHub(t)
			scoped := NewScopedClient("reseller")
			registerAndSettle(t, h, scoped)

			h.Broadcast(msgType, map[string]any{"clients": []string{"someone-elses@example.com"}})

			got, ok := readOne(t, scoped)
			if !ok {
				t.Fatal("scoped client received nothing")
			}
			if got.Type != MessageTypeInvalidate {
				t.Fatalf("scoped client received %q — panel-wide client data leaked", got.Type)
			}
			payload, _ := got.Payload.(map[string]any)
			if payload["type"] != string(MessageTypeClients) {
				t.Fatalf("invalidate names %v, want %q", payload["type"], MessageTypeClients)
			}
		})
	}
}

// TestBroadcast_NonSensitiveTypesReachScopedClients keeps the fix narrow:
// server status and xray state say nothing about other tenants' inbounds, so
// scoped sessions still get them in full.
func TestBroadcast_NonSensitiveTypesReachScopedClients(t *testing.T) {
	h := newRunningHub(t)
	scoped := NewScopedClient("reseller")
	registerAndSettle(t, h, scoped)

	h.Broadcast(MessageTypeStatus, map[string]any{"cpu": 12})

	got, ok := readOne(t, scoped)
	if !ok {
		t.Fatal("scoped client received nothing")
	}
	if got.Type != MessageTypeStatus {
		t.Fatalf("got type %q, want %q", got.Type, MessageTypeStatus)
	}
}

package model

import (
	"encoding/json"
	"testing"
)

// TestOnlyPasswordProtocolsAreRecognised pins the rule the cleanup depends on.
// Getting this wrong in the permissive direction would strip the real
// credential off every Trojan and Shadowsocks client on the panel.
func TestOnlyPasswordProtocolsAreRecognised(t *testing.T) {
	for _, p := range []Protocol{Trojan, Shadowsocks} {
		if !ProtocolUsesClientPassword(p) {
			t.Errorf("%s authenticates by password but was not recognised", p)
		}
	}
	for _, p := range []Protocol{VLESS, VMESS, Hysteria, WireGuard, MTProto} {
		if ProtocolUsesClientPassword(p) {
			t.Errorf("%s does not authenticate by password", p)
		}
	}
}

const vlessSettingsWithPasswords = `{
  "clients": [
    {"id": "uuid-one", "email": "a@x", "flow": "xtls-rprx-vision", "password": "leftover"},
    {"id": "uuid-two", "email": "b@x", "password": ""}
  ],
  "decryption": "none",
  "fallbacks": []
}`

// TestStrippingRemovesEveryClientPassword covers the blank case too: an empty
// password still renders as a row in the connection info, so leaving it behind
// would leave the confusing display in place.
func TestStrippingRemovesEveryClientPassword(t *testing.T) {
	got, changed := StripClientPasswords(vlessSettingsWithPasswords)
	if !changed {
		t.Fatal("reported no change")
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("rewrote settings into invalid JSON: %v", err)
	}
	for i, entry := range parsed["clients"].([]any) {
		if _, present := entry.(map[string]any)["password"]; present {
			t.Errorf("client %d kept its password", i)
		}
	}
}

// TestStrippingKeepsEverythingElse guards the seeder: it rewrites stored
// settings for every VLESS and VMess inbound, so dropping a key here would
// destroy working inbounds. The uuid is the credential on these protocols.
func TestStrippingKeepsEverythingElse(t *testing.T) {
	got, changed := StripClientPasswords(vlessSettingsWithPasswords)
	if !changed {
		t.Fatal("reported no change")
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed["decryption"] != "none" {
		t.Errorf("lost decryption: %v", parsed["decryption"])
	}
	if _, ok := parsed["fallbacks"]; !ok {
		t.Error("lost fallbacks")
	}

	clients := parsed["clients"].([]any)
	if len(clients) != 2 {
		t.Fatalf("client count = %d, want 2", len(clients))
	}
	first := clients[0].(map[string]any)
	if first["id"] != "uuid-one" {
		t.Errorf("lost the uuid: %v", first["id"])
	}
	if first["email"] != "a@x" {
		t.Errorf("lost the email: %v", first["email"])
	}
	if first["flow"] != "xtls-rprx-vision" {
		t.Errorf("lost the flow: %v", first["flow"])
	}
	if second := clients[1].(map[string]any); second["id"] != "uuid-two" {
		t.Errorf("lost the second client's uuid: %v", second["id"])
	}
}

// TestStrippingLeavesCleanSettingsAlone keeps the seeder from rewriting — and
// reordering — settings that have nothing to clean.
func TestStrippingLeavesCleanSettingsAlone(t *testing.T) {
	for name, in := range map[string]string{
		"no passwords": `{"clients":[{"id":"u","email":"a@x"}],"decryption":"none"}`,
		"no clients":   `{"decryption":"none"}`,
		"empty":        "",
		"not json":     "definitely not json",
	} {
		got, changed := StripClientPasswords(in)
		if changed {
			t.Errorf("%s: reported a change", name)
		}
		if got != in {
			t.Errorf("%s: altered the settings", name)
		}
	}
}

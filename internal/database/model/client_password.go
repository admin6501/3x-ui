package model

import "encoding/json"

// ProtocolUsesClientPassword reports whether a protocol authenticates its
// clients with a password.
//
// Mirrors the per-protocol switch that builds client entries in
// service/xray.go: VLESS and VMess identify by uuid and the core ignores a
// password on them entirely, so one stored against such a client is dead
// weight that the panel used to display as though it meant something.
func ProtocolUsesClientPassword(p Protocol) bool {
	return p == Trojan || p == Shadowsocks
}

// StripClientPasswords removes the password from every client in an inbound's
// settings blob, returning the rewritten JSON and whether anything changed.
//
// Callers are responsible for only applying this to protocols that do not
// authenticate by password — it does not check, because the settings blob does
// not carry the protocol.
func StripClientPasswords(settings string) (string, bool) {
	if settings == "" {
		return settings, false
	}
	var parsed map[string]any
	if json.Unmarshal([]byte(settings), &parsed) != nil {
		return settings, false
	}
	clients, ok := parsed["clients"].([]any)
	if !ok {
		return settings, false
	}
	changed := false
	for _, entry := range clients {
		client, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if _, present := client["password"]; !present {
			continue
		}
		delete(client, "password")
		changed = true
	}
	if !changed {
		return settings, false
	}
	rewritten, err := json.MarshalIndent(parsed, "", "  ")
	if err != nil {
		return settings, false
	}
	return string(rewritten), true
}

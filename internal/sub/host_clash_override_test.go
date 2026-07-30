package sub

import "testing"

// TestHostOverridesReachTransportSettings is the regression for a Host record
// that works in the raw link and silently does nothing in Clash or JSON.
//
// The raw-link path applies host/path through the share params, but the Clash
// and JSON renderers read the transport settings object instead. Without this
// the client connected with the inbound's original host and path — which the
// CDN in front of it does not accept, so the config simply failed.
func TestHostOverridesReachTransportSettings(t *testing.T) {
	for _, transport := range []string{"wsSettings", "httpupgradeSettings", "xhttpSettings"} {
		stream := map[string]any{
			transport: map[string]any{"host": "origin.example.com", "path": "/original"},
		}
		applyHostStreamOverrides(map[string]any{
			"hostHeader": "cdn.example.com",
			"path":       "/override",
		}, stream)

		ts, ok := stream[transport].(map[string]any)
		if !ok {
			t.Fatalf("%s vanished", transport)
		}
		if ts["host"] != "cdn.example.com" {
			t.Errorf("%s host = %v, want the override", transport, ts["host"])
		}
		if ts["path"] != "/override" {
			t.Errorf("%s path = %v, want the override", transport, ts["path"])
		}
	}
}

// TestHostOverridesLeaveUnsetFieldsAlone: a Host record that sets only one of
// the two must not blank the other.
func TestHostOverridesLeaveUnsetFieldsAlone(t *testing.T) {
	stream := map[string]any{
		"wsSettings": map[string]any{"host": "origin.example.com", "path": "/original"},
	}
	applyHostStreamOverrides(map[string]any{"hostHeader": "cdn.example.com"}, stream)

	ts := stream["wsSettings"].(map[string]any)
	if ts["host"] != "cdn.example.com" {
		t.Errorf("host = %v, want the override", ts["host"])
	}
	if ts["path"] != "/original" {
		t.Errorf("path = %v; an unset override must not clear it", ts["path"])
	}
}

// TestHostOverridesSkipAbsentTransports guards against inventing a transport
// block the inbound does not use.
func TestHostOverridesSkipAbsentTransports(t *testing.T) {
	stream := map[string]any{"network": "tcp"}
	applyHostStreamOverrides(map[string]any{
		"hostHeader": "cdn.example.com",
		"path":       "/override",
	}, stream)

	for _, transport := range []string{"wsSettings", "httpupgradeSettings", "xhttpSettings"} {
		if _, present := stream[transport]; present {
			t.Errorf("created a %s block for a tcp inbound", transport)
		}
	}
}

package model

import (
	"encoding/json"
	"testing"
)

// realityStream is a trimmed but realistic stored streamSettings blob, with the
// minClientVer line dropped in verbatim so each case controls only that field.
func realityStream(minClientVerLine string) string {
	return `{
  "network": "tcp",
  "security": "reality",
  "realitySettings": {
    "show": false,
    "target": "www.cloudflare.com:443",
    "serverNames": ["www.cloudflare.com"],
    "privateKey": "SECRET-PRIVATE-KEY",
    "shortIds": ["0123abcd"],
    "maxTimediff": 0` + minClientVerLine + `
  }
}`
}

// TestBlankMinClientVerIsPinnedInStoredSettings is the fix the operator sees:
// the value the core enforces has to be in the stored inbound, because that is
// what the edit form renders. Pinning only the generated config left the field
// showing empty while clients were being refused.
func TestBlankMinClientVerIsPinnedInStoredSettings(t *testing.T) {
	for name, line := range map[string]string{
		"absent":     "",
		"empty":      `,"minClientVer": ""`,
		"whitespace": `,"minClientVer": "   "`,
	} {
		got, changed := PinRealityMinClientVerJSON(realityStream(line))
		if !changed {
			t.Errorf("%s: reported no change", name)
			continue
		}

		var stream map[string]any
		if err := json.Unmarshal([]byte(got), &stream); err != nil {
			t.Errorf("%s: rewrote settings into invalid JSON: %v", name, err)
			continue
		}
		reality := stream["realitySettings"].(map[string]any)
		if reality["minClientVer"] != RealityDefaultMinClientVer {
			t.Errorf("%s: minClientVer = %v, want %s", name, reality["minClientVer"], RealityDefaultMinClientVer)
		}
	}
}

// TestPinningKeepsEveryOtherRealityField guards the seeder: it rewrites stored
// settings for every REALITY inbound on the panel, so dropping a key here would
// silently destroy working inbounds. The private key especially is not
// recoverable once lost.
func TestPinningKeepsEveryOtherRealityField(t *testing.T) {
	got, changed := PinRealityMinClientVerJSON(realityStream(""))
	if !changed {
		t.Fatal("reported no change")
	}

	var stream map[string]any
	if err := json.Unmarshal([]byte(got), &stream); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if stream["network"] != "tcp" || stream["security"] != "reality" {
		t.Errorf("clobbered the stream envelope: %v", stream)
	}

	reality := stream["realitySettings"].(map[string]any)
	if reality["privateKey"] != "SECRET-PRIVATE-KEY" {
		t.Errorf("lost the private key: %v", reality["privateKey"])
	}
	if reality["target"] != "www.cloudflare.com:443" {
		t.Errorf("lost the target: %v", reality["target"])
	}
	if names, _ := reality["serverNames"].([]any); len(names) != 1 || names[0] != "www.cloudflare.com" {
		t.Errorf("lost serverNames: %v", reality["serverNames"])
	}
	if ids, _ := reality["shortIds"].([]any); len(ids) != 1 || ids[0] != "0123abcd" {
		t.Errorf("lost shortIds: %v", reality["shortIds"])
	}
}

// TestOperatorMinClientVerSurvivesPinning is the escape hatch. An operator whose
// users run older client apps sets 0.0.0 to lift the floor; overwriting it —
// on save, or on every panel restart via the seeder — would lock those users out
// with no way back.
func TestOperatorMinClientVerSurvivesPinning(t *testing.T) {
	for _, chosen := range []string{"0.0.0", "1.8.0", "25.9.11", RealityDefaultMinClientVer} {
		in := realityStream(`,"minClientVer": "` + chosen + `"`)
		got, changed := PinRealityMinClientVerJSON(in)
		if changed {
			t.Errorf("%s: rewrote an inbound that already had a floor", chosen)
		}
		if got != in {
			t.Errorf("%s: altered the stored settings", chosen)
		}
	}
}

// TestNonRealityStreamsAreNotPinned keeps the seeder off inbounds it has no
// business editing. An inbound switched from REALITY to TLS still carries an
// inert realitySettings block, and the LIKE query the seeder uses to find
// candidates matches it.
func TestNonRealityStreamsAreNotPinned(t *testing.T) {
	for name, in := range map[string]string{
		"switched to tls": `{"security":"tls","tlsSettings":{},"realitySettings":{"privateKey":"k"}}`,
		"no security":     `{"network":"tcp"}`,
		"empty":           "",
		"not json":        "definitely not json",
	} {
		got, changed := PinRealityMinClientVerJSON(in)
		if changed {
			t.Errorf("%s: reported a change", name)
		}
		if got != in {
			t.Errorf("%s: altered the settings", name)
		}
	}
}

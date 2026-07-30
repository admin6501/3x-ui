package service

import "testing"

// TestBlankMinClientVerGetsTheExplicitFloor.
//
// xray-core v26.7.28 made an empty "minClientVer" mean 26.3.27 rather than "no
// minimum". The panel writes that value itself so the floor is visible in the
// generated config and in the inbound's settings, rather than being supplied
// invisibly by the core — the difference between an operator being able to
// read why a client was refused and having to infer it from a version bump.
func TestBlankMinClientVerGetsTheExplicitFloor(t *testing.T) {
	for name, reality := range map[string]map[string]any{
		"absent": {"privateKey": "k"},
		"empty":  {"privateKey": "k", "minClientVer": ""},
		"blank":  {"privateKey": "k", "minClientVer": "   "},
	} {
		stream := map[string]any{"security": "reality", "realitySettings": reality}
		pinRealityMinClientVer(stream)

		got := stream["realitySettings"].(map[string]any)["minClientVer"]
		if got != realityDefaultMinClientVer {
			t.Errorf("%s: minClientVer = %v, want %s", name, got, realityDefaultMinClientVer)
		}
	}
}

// TestOperatorMinClientVerIsRespected is the escape hatch that matters.
//
// A panel whose users run older clients needs 0.0.0 to keep them connecting;
// overwriting an explicit value would take that away and leave the operator
// with no way to lower the floor.
func TestOperatorMinClientVerIsRespected(t *testing.T) {
	for _, chosen := range []string{"0.0.0", "1.8.0", "26.3.27"} {
		stream := map[string]any{
			"security":        "reality",
			"realitySettings": map[string]any{"minClientVer": chosen},
		}
		pinRealityMinClientVer(stream)

		if got := stream["realitySettings"].(map[string]any)["minClientVer"]; got != chosen {
			t.Errorf("minClientVer = %v, want the operator's own %s", got, chosen)
		}
	}
}

// TestNonRealityStreamsAreUntouched keeps the pin from inventing REALITY
// settings on inbounds that have none.
func TestNonRealityStreamsAreUntouched(t *testing.T) {
	for name, stream := range map[string]map[string]any{
		"tls":            {"security": "tls", "tlsSettings": map[string]any{}},
		"none":           {"security": "none"},
		"empty":          {},
		"reality is nil": {"security": "reality", "realitySettings": nil},
	} {
		pinRealityMinClientVer(stream)
		if rs, present := stream["realitySettings"]; present && rs != nil {
			if _, added := rs.(map[string]any)["minClientVer"]; added {
				t.Errorf("%s: added minClientVer to a non-REALITY stream", name)
			}
		}
	}
}

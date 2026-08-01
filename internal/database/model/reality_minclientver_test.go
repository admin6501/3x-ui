package model

import (
	"encoding/json"
	"testing"
)

func realityStream(t *testing.T, minClientVer string) string {
	t.Helper()
	reality := map[string]any{
		"dest":        "example.com:443",
		"serverNames": []string{"example.com"},
		"privateKey":  "k",
	}
	if minClientVer != "" {
		reality["minClientVer"] = minClientVer
	}
	raw, err := json.Marshal(map[string]any{
		"network":         "tcp",
		"security":        "reality",
		"realitySettings": reality,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(raw)
}

func realityMinClientVer(t *testing.T, streamSettings string) (string, bool) {
	t.Helper()
	var stream struct {
		Reality struct {
			MinClientVer *string `json:"minClientVer"`
		} `json:"realitySettings"`
	}
	if err := json.Unmarshal([]byte(streamSettings), &stream); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if stream.Reality.MinClientVer == nil {
		return "", false
	}
	return *stream.Reality.MinClientVer, true
}

// TestUnpinRemovesTheMigrationsValue is the point of the migration: an inbound
// carrying the floor a since-reverted panel version wrote goes back to having
// no opinion, which on the pinned core means no minimum at all.
func TestUnpinRemovesTheMigrationsValue(t *testing.T) {
	in := realityStream(t, PinnedRealityMinClientVer)
	out, changed := UnpinRealityMinClientVer(in)
	if !changed {
		t.Fatal("expected the pinned value to be removed")
	}
	if _, present := realityMinClientVer(t, out); present {
		t.Errorf("minClientVer still present: %s", out)
	}
}

// TestUnpinLeavesTheRestOfTheStreamIntact guards against the rewrite dropping
// neighbouring settings — this runs over live inbounds on upgrade.
func TestUnpinLeavesTheRestOfTheStreamIntact(t *testing.T) {
	out, changed := UnpinRealityMinClientVer(realityStream(t, PinnedRealityMinClientVer))
	if !changed {
		t.Fatal("expected a rewrite")
	}
	var stream map[string]any
	if err := json.Unmarshal([]byte(out), &stream); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if stream["security"] != "reality" || stream["network"] != "tcp" {
		t.Errorf("stream keys lost: %v", stream)
	}
	reality, _ := stream["realitySettings"].(map[string]any)
	if reality["dest"] != "example.com:443" || reality["privateKey"] != "k" {
		t.Errorf("reality keys lost: %v", reality)
	}
}

// TestUnpinLeavesOperatorChoicesAlone: the migration only reverses itself. A
// floor the operator typed — including the values that lift it — is theirs.
func TestUnpinLeavesOperatorChoicesAlone(t *testing.T) {
	for _, value := range []string{"0.0.0", "1.0.0", "25.9.11", "26.3.28"} {
		in := realityStream(t, value)
		out, changed := UnpinRealityMinClientVer(in)
		if changed {
			t.Errorf("minClientVer=%q was rewritten; only %q may be", value, PinnedRealityMinClientVer)
		}
		if got, _ := realityMinClientVer(t, out); got != value {
			t.Errorf("minClientVer=%q became %q", value, got)
		}
	}
}

// TestUnpinIgnoresNonReality: an inbound that moved to TLS can still carry an
// inert realitySettings block, and it is not this migration's business.
func TestUnpinIgnoresNonReality(t *testing.T) {
	in := `{"network":"tcp","security":"tls","realitySettings":{"minClientVer":"26.3.27"}}`
	out, changed := UnpinRealityMinClientVer(in)
	if changed || out != in {
		t.Errorf("non-reality stream was rewritten: %s", out)
	}
}

// TestUnpinIsIdempotentAndSafeOnJunk keeps a startup migration from failing or
// corrupting rows it cannot parse.
func TestUnpinIsIdempotentAndSafeOnJunk(t *testing.T) {
	once, _ := UnpinRealityMinClientVer(realityStream(t, PinnedRealityMinClientVer))
	twice, changed := UnpinRealityMinClientVer(once)
	if changed || twice != once {
		t.Error("second run changed an already-unpinned stream")
	}
	for _, junk := range []string{"", "not json", "{}", `{"security":"reality"}`} {
		out, changed := UnpinRealityMinClientVer(junk)
		if changed || out != junk {
			t.Errorf("junk %q was rewritten to %q", junk, out)
		}
	}
}

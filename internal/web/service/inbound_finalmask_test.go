package service

import (
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

const realityWithTcpMask = `{"network":"tcp","security":"reality","finalmask":{"tcp":[{"type":"fragment"}]}}`

// TestRealityWithTcpMaskIsRefused is the crash guard. xray-core dies on the
// first connection to this combination, taking every client on the panel with
// it — not just the misconfigured inbound.
func TestRealityWithTcpMaskIsRefused(t *testing.T) {
	err := validateFinalMaskRealityCombo(&model.Inbound{StreamSettings: realityWithTcpMask})
	if err == nil {
		t.Fatal("accepted a TCP finalmask on a REALITY inbound; xray-core would crash on first use")
	}
	if !strings.Contains(err.Error(), "REALITY") {
		t.Errorf("the error should name the conflict so the operator can act on it: %v", err)
	}
}

// TestHarmlessCombinationsAreAccepted keeps the guard from blocking valid
// configurations — a TCP mask without REALITY, and REALITY without a TCP mask,
// are both fine.
func TestHarmlessCombinationsAreAccepted(t *testing.T) {
	for name, stream := range map[string]string{
		"tcp mask, tls":            `{"network":"tcp","security":"tls","finalmask":{"tcp":[{"type":"fragment"}]}}`,
		"reality, no mask":         `{"network":"tcp","security":"reality"}`,
		"reality, udp mask only":   `{"network":"tcp","security":"reality","finalmask":{"udp":[{"type":"salamander"}]}}`,
		"reality, empty tcp array": `{"network":"tcp","security":"reality","finalmask":{"tcp":[]}}`,
		"no stream settings":       "",
		"malformed json":           "{not json",
	} {
		if err := validateFinalMaskRealityCombo(&model.Inbound{StreamSettings: stream}); err != nil {
			t.Errorf("%s was refused: %v", name, err)
		}
	}
	if err := validateFinalMaskRealityCombo(nil); err != nil {
		t.Errorf("nil inbound refused: %v", err)
	}
}

// TestStripRemovesOnlyTheTcpMask: the upgrade path must repair the crash
// without discarding obfuscation the operator configured on other transports.
func TestStripRemovesOnlyTheTcpMask(t *testing.T) {
	both := `{"network":"tcp","security":"reality","finalmask":{"tcp":[{"type":"fragment"}],"udp":[{"type":"salamander"}]}}`
	out, changed := model.StripTcpFinalMask(both)
	if !changed {
		t.Fatal("the TCP mask was left in place")
	}
	if model.StreamHasTcpFinalMask(out) {
		t.Errorf("TCP mask survived: %s", out)
	}
	if !strings.Contains(out, "salamander") {
		t.Errorf("the UDP mask was discarded along with it: %s", out)
	}
	// Still a REALITY inbound afterwards.
	if !model.StreamHasReality(out) {
		t.Errorf("security was altered: %s", out)
	}
}

// TestStripDropsAnEmptyFinalMask keeps the stored settings matching what the
// form would produce, rather than leaving an empty object behind.
func TestStripDropsAnEmptyFinalMask(t *testing.T) {
	out, changed := model.StripTcpFinalMask(realityWithTcpMask)
	if !changed {
		t.Fatal("nothing stripped")
	}
	if strings.Contains(out, "finalmask") {
		t.Errorf("an empty finalmask object was left behind: %s", out)
	}
}

// TestStripIsANoOpWhenThereIsNothingToDo guards the seeder against rewriting
// rows it has no business touching.
func TestStripIsANoOpWhenThereIsNothingToDo(t *testing.T) {
	for name, stream := range map[string]string{
		"no finalmask":   `{"network":"tcp","security":"reality"}`,
		"udp mask only":  `{"network":"tcp","security":"reality","finalmask":{"udp":[{"type":"salamander"}]}}`,
		"empty":          "",
		"malformed json": "{not json",
	} {
		out, changed := model.StripTcpFinalMask(stream)
		if changed {
			t.Errorf("%s: claimed a change", name)
		}
		if out != stream {
			t.Errorf("%s: mutated %q into %q", name, stream, out)
		}
	}
}

// TestRealityEditForcesARestart: xray-core does not rebuild a REALITY
// listener's authenticator on a runtime re-add, so a changed key or shortId
// looks applied in the panel while clients keep authenticating against the old
// parameters. Hot-swapping such an edit is silently wrong.
func TestRealityEditForcesARestart(t *testing.T) {
	const before = `{"network":"tcp","security":"reality","realitySettings":{"shortIds":["aa"]}}`
	const after = `{"network":"tcp","security":"reality","realitySettings":{"shortIds":["bb"]}}`

	if !realityAuthChanged(
		&model.Inbound{StreamSettings: before},
		&model.Inbound{StreamSettings: after},
	) {
		t.Error("a changed REALITY shortId must not be hot-swapped")
	}

	// Switching security on or off also changes what the listener authenticates
	// with.
	const plain = `{"network":"tcp","security":"tls"}`
	if !realityAuthChanged(&model.Inbound{StreamSettings: plain}, &model.Inbound{StreamSettings: after}) {
		t.Error("an inbound that starts using REALITY must restart")
	}
	if !realityAuthChanged(&model.Inbound{StreamSettings: before}, &model.Inbound{StreamSettings: plain}) {
		t.Error("an inbound that stops using REALITY must restart")
	}
}

// TestNonRealityEditsStillHotSwap keeps the guard from turning every save into
// a core restart, which would drop every connected client on an unrelated edit.
func TestNonRealityEditsStillHotSwap(t *testing.T) {
	const reality = `{"network":"tcp","security":"reality","realitySettings":{"shortIds":["aa"]}}`

	// Client-only edit: the stream is untouched.
	if realityAuthChanged(&model.Inbound{StreamSettings: reality}, &model.Inbound{StreamSettings: reality}) {
		t.Error("a client-only edit on a REALITY inbound must still hot-swap")
	}
	// Neither side uses REALITY.
	if realityAuthChanged(
		&model.Inbound{StreamSettings: `{"network":"ws","security":"tls"}`},
		&model.Inbound{StreamSettings: `{"network":"ws","security":"tls","wsSettings":{"path":"/new"}}`},
	) {
		t.Error("a non-REALITY stream change must still hot-swap")
	}
	if realityAuthChanged(nil, nil) {
		t.Error("nil inbounds must not force a restart")
	}
}

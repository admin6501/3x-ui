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

package xray

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/util/json_util"
)

// Disabling (or deleting) an inbound removes it from the generated config with
// no replacement carrying its tag. Xray's runtime DelInbound API only stops the
// listener; sessions already established on the inbound keep flowing until they
// close. So a pure inbound removal must NOT be hot-appliable — it has to force a
// full process restart, which is the only thing that actually drops those live
// client connections (reported bug: disabled inbound's clients stayed connected).
func TestComputeHotDiff_InboundRemovalForcesRestart(t *testing.T) {
	oldCfg := makeHotConfig()
	newCfg := makeHotConfig()
	// Drop inbound-1080 (index 1) entirely — simulates disabling/deleting it.
	newCfg.InboundConfigs = newCfg.InboundConfigs[:1]

	if _, ok := ComputeHotDiff(oldCfg, newCfg); ok {
		t.Fatal("a pure inbound removal must NOT be hot-appliable; it must force a full xray restart so live client sessions are dropped")
	}
}

// A genuine inbound edit (same tag, changed settings) stays hot-appliable — the
// removal-forces-restart rule must only fire for tags that are not re-added.
func TestComputeHotDiff_InboundEditStillHotAppliable(t *testing.T) {
	oldCfg := makeHotConfig()
	newCfg := makeHotConfig()
	newCfg.InboundConfigs[1].Settings = json_util.RawMessage(`{"clients":[{"email":"a"}]}`)

	diff, ok := ComputeHotDiff(oldCfg, newCfg)
	if !ok {
		t.Fatal("an inbound edit (tag re-added) must remain hot-appliable")
	}
	if len(diff.RemovedInboundTags) != 1 || diff.RemovedInboundTags[0] != "inbound-1080" {
		t.Fatalf("expected the edited inbound to be remove+re-added, got %v", diff.RemovedInboundTags)
	}
	if len(diff.AddedInbounds) != 1 {
		t.Fatalf("expected exactly one re-add, got %d", len(diff.AddedInbounds))
	}
}

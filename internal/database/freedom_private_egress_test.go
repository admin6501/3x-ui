package database

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestStockRulesGainThePrivateBlock is the security fix itself.
//
// The routing table blocks geoip:private, but domainStrategy AsIs means the
// router never resolves a domain — so a hostname whose A record points at a
// private address slips past that rule, and an allow-everything finalRules
// then lets a proxy client reach loopback services on the panel host,
// including xray's own gRPC API.
func TestStockRulesGainThePrivateBlock(t *testing.T) {
	for _, stock := range []string{
		`{"outbounds":[{"protocol":"freedom","settings":{"domainStrategy":"AsIs","finalRules":[{"action":"allow"}]},"tag":"direct"}]}`,
		// The older private-only-allow shape upgrades too.
		`{"outbounds":[{"protocol":"freedom","settings":{"finalRules":[{"action":"allow","ip":["geoip:private"]}]},"tag":"direct"}]}`,
	} {
		updated, changed, err := rewriteFreedomFinalRulesPrivateEgress(stock)
		if err != nil {
			t.Fatalf("rewrite: %v", err)
		}
		if !changed {
			t.Fatalf("stock rules were left unprotected: %s", stock)
		}
		var cfg map[string]any
		if err := json.Unmarshal([]byte(updated), &cfg); err != nil {
			t.Fatalf("result is not valid json: %v", err)
		}
		if !strings.Contains(updated, "geoip:private") || !strings.Contains(updated, `"block"`) {
			t.Errorf("no private block in the result:\n%s", updated)
		}
		// The block has to come first, or the allow above it wins.
		blockAt := strings.Index(updated, `"block"`)
		allowAt := strings.Index(updated, `"allow"`)
		if blockAt < 0 || allowAt < 0 || blockAt > allowAt {
			t.Errorf("block must precede allow:\n%s", updated)
		}
	}
}

// TestCustomisedRulesAreLeftAlone: an operator who narrowed the egress rules
// has made a deliberate choice, and a seeder must not overwrite it.
func TestCustomisedRulesAreLeftAlone(t *testing.T) {
	for _, custom := range []string{
		`{"outbounds":[{"protocol":"freedom","settings":{"finalRules":[{"action":"block","ip":["geoip:private"]},{"action":"allow"}]},"tag":"direct"}]}`,
		`{"outbounds":[{"protocol":"freedom","settings":{"finalRules":[{"action":"allow","domain":["example.com"]}]},"tag":"direct"}]}`,
		`{"outbounds":[{"protocol":"freedom","settings":{"finalRules":[{"action":"block"},{"action":"allow"}]},"tag":"direct"}]}`,
	} {
		if _, changed, err := rewriteFreedomFinalRulesPrivateEgress(custom); err != nil {
			t.Fatalf("rewrite: %v", err)
		} else if changed {
			t.Errorf("overwrote customised rules: %s", custom)
		}
	}
}

// TestNonFreedomOutboundsUntouched keeps the rewrite from reaching proxies.
func TestNonFreedomOutboundsUntouched(t *testing.T) {
	cfg := `{"outbounds":[{"protocol":"blackhole","settings":{"finalRules":[{"action":"allow"}]},"tag":"blocked"}]}`
	if _, changed, err := rewriteFreedomFinalRulesPrivateEgress(cfg); err != nil {
		t.Fatalf("rewrite: %v", err)
	} else if changed {
		t.Error("rewrote a non-freedom outbound")
	}
}

// TestMalformedInputIsNotDestructive: the seeder must degrade to a no-op
// rather than corrupt an operator's template.
func TestMalformedInputIsNotDestructive(t *testing.T) {
	for _, raw := range []string{"", "   ", "not json", `{"outbounds":"wrong type"}`} {
		out, changed, err := rewriteFreedomFinalRulesPrivateEgress(raw)
		if changed {
			t.Errorf("claimed a change for %q", raw)
		}
		if out != raw {
			t.Errorf("mutated %q into %q", raw, out)
		}
		_ = err
	}
}

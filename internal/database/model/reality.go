package model

import (
	"encoding/json"
	"strings"
)

// RealityDefaultMinClientVer is the floor xray-core v26.7.28 applies to a
// REALITY inbound that leaves minClientVer empty. Before that release an empty
// field meant "no minimum"; now it means this, so upgrading the core silently
// raised the floor and refused every client built on an older one.
//
// The panel writes the value rather than leaving the field blank. Behaviourally
// that matches what the core already does on its own — the point is that it
// becomes readable, in the generated config and in the inbound's own settings,
// so an operator can see why a client is being refused instead of inferring it
// from a version bump.
//
// An operator whose users have not updated their client apps can set 0.0.0 on
// the inbound to lift the floor again; that choice is never overwritten.
const RealityDefaultMinClientVer = "26.3.27"

// PinRealityMinClientVer writes the default floor into a decoded stream that
// carries no minClientVer of its own, reporting whether it changed anything.
// An explicit value — including 0.0.0 — is left alone.
func PinRealityMinClientVer(stream map[string]any) bool {
	reality, ok := stream["realitySettings"].(map[string]any)
	if !ok || reality == nil {
		return false
	}
	if current, _ := reality["minClientVer"].(string); strings.TrimSpace(current) != "" {
		return false
	}
	reality["minClientVer"] = RealityDefaultMinClientVer
	return true
}

// PinRealityMinClientVerJSON is PinRealityMinClientVer over a stored
// streamSettings blob, returning the rewritten JSON and whether it changed.
//
// Unlike the decoded form it insists the stream actually terminates in REALITY:
// this one writes to the database, and an inbound that switched to TLS can
// still carry an inert realitySettings block that should not be edited.
//
// Lives here rather than in the service layer because the upgrade seeder in
// internal/database needs it too, and database cannot import service.
func PinRealityMinClientVerJSON(streamSettings string) (string, bool) {
	if !StreamHasReality(streamSettings) {
		return streamSettings, false
	}
	var stream map[string]any
	if json.Unmarshal([]byte(streamSettings), &stream) != nil {
		return streamSettings, false
	}
	if !PinRealityMinClientVer(stream) {
		return streamSettings, false
	}
	rewritten, err := json.MarshalIndent(stream, "", "  ")
	if err != nil {
		return streamSettings, false
	}
	return string(rewritten), true
}

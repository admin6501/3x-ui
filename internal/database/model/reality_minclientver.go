package model

import (
	"encoding/json"
	"strings"
)

// PinnedRealityMinClientVer is the value a since-removed panel migration wrote
// into every stored REALITY inbound.
//
// The panel briefly pinned xray-core's own client-version floor into the
// inbound's settings so the edit form would show the operator what the core was
// enforcing. That feature was reverted, but reverting code does not un-write a
// database: inbounds saved while it was live still carry the value, and because
// the migration recorded itself in history_of_seeders it never ran again to
// notice.
//
// The pin is not inert. The floor it names belongs to newer cores — on the core
// this panel pins, an empty field means no minimum at all — so the stored value
// keeps refusing clients that the panel's own configuration says nothing about,
// and it follows the inbound across core downgrades. Upstream never writes this
// field, which is why an inbound created there does not behave this way.
const PinnedRealityMinClientVer = "26.3.27"

// UnpinRealityMinClientVer removes the migration's floor from a stored
// streamSettings blob, returning the rewritten JSON and whether it changed.
//
// Only the exact value the migration wrote is removed. Anything else — a floor
// the operator chose, including 0.0.0 or 1.0.0 to lift it — is theirs and is
// left alone, so re-running this is safe and so is setting 26.3.27 back
// deliberately afterwards (it will not survive a re-run, which is the one
// accepted cost of not being able to tell the two apart).
func UnpinRealityMinClientVer(streamSettings string) (string, bool) {
	if streamSettings == "" || !StreamHasReality(streamSettings) {
		return streamSettings, false
	}
	var stream map[string]any
	if json.Unmarshal([]byte(streamSettings), &stream) != nil {
		return streamSettings, false
	}
	reality, ok := stream["realitySettings"].(map[string]any)
	if !ok || reality == nil {
		return streamSettings, false
	}
	current, _ := reality["minClientVer"].(string)
	if strings.TrimSpace(current) != PinnedRealityMinClientVer {
		return streamSettings, false
	}
	// Delete rather than blank it: an absent field and an empty string mean the
	// same thing to the core, and the panel's schema defaults it back to "".
	delete(reality, "minClientVer")
	rewritten, err := json.Marshal(stream)
	if err != nil {
		return streamSettings, false
	}
	return string(rewritten), true
}

package model

import "encoding/json"

// StreamHasReality reports whether an inbound's stream terminates in REALITY.
func StreamHasReality(streamSettings string) bool {
	if streamSettings == "" {
		return false
	}
	var stream struct {
		Security string `json:"security"`
	}
	if json.Unmarshal([]byte(streamSettings), &stream) != nil {
		return false
	}
	return stream.Security == "reality"
}

// StreamHasTcpFinalMask reports whether a TCP finalmask is configured.
func StreamHasTcpFinalMask(streamSettings string) bool {
	if streamSettings == "" {
		return false
	}
	var stream struct {
		FinalMask struct {
			Tcp []any `json:"tcp"`
		} `json:"finalmask"`
	}
	if json.Unmarshal([]byte(streamSettings), &stream) != nil {
		return false
	}
	return len(stream.FinalMask.Tcp) > 0
}

// StripTcpFinalMask removes finalmask.tcp from a stream settings blob, leaving
// every other finalmask transport in place. Returns the rewritten JSON and
// whether anything changed.
//
// Lives here rather than in the service layer because the upgrade seeder in
// internal/database needs it too, and database cannot import service.
func StripTcpFinalMask(streamSettings string) (string, bool) {
	if streamSettings == "" {
		return streamSettings, false
	}
	var stream map[string]any
	if json.Unmarshal([]byte(streamSettings), &stream) != nil {
		return streamSettings, false
	}
	finalmask, ok := stream["finalmask"].(map[string]any)
	if !ok {
		return streamSettings, false
	}
	if _, present := finalmask["tcp"]; !present {
		return streamSettings, false
	}
	delete(finalmask, "tcp")
	// An empty finalmask object is noise; drop it so the stored settings match
	// what the form would produce.
	if len(finalmask) == 0 {
		delete(stream, "finalmask")
	} else {
		stream["finalmask"] = finalmask
	}
	rewritten, err := json.MarshalIndent(stream, "", "  ")
	if err != nil {
		return streamSettings, false
	}
	return string(rewritten), true
}

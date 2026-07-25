package model

import (
	"encoding/json"
	"slices"
)

// HostMirrorSubType is the subscription format the panel's own share links
// represent. Hosts excluded from it are left out of the externalProxy mirror
// written into an inbound's streamSettings.
const HostMirrorSubType = "raw"

// HostsToExternalProxyEntries projects the mirrorable hosts of one inbound onto
// legacy externalProxy entries: enabled hosts, in the given order, that are not
// excluded from the raw subscription. A host with no usable port (its own and
// the inbound's are both 0) can't produce a share link and is skipped.
//
// `dest` is left as the host's own address: an override-only host (blank
// address) inherits the inbound's address, which is resolved per request (node
// address, public host, ...), so the link generators fall back to it.
func HostsToExternalProxyEntries(hosts []*Host, inboundPort int) []any {
	entries := make([]any, 0, len(hosts))
	for _, h := range hosts {
		if h == nil || h.IsDisabled {
			continue
		}
		if slices.Contains(h.ExcludeFromSubTypes, HostMirrorSubType) {
			continue
		}
		if h.Port == 0 && inboundPort == 0 {
			continue
		}
		entries = append(entries, h.ToExternalProxyEntry("", inboundPort))
	}
	return entries
}

// StreamSettingsWithExternalProxy returns the stream settings JSON with its
// externalProxy array replaced by entries, plus whether anything actually
// changed (so a no-op sync doesn't touch the row).
func StreamSettingsWithExternalProxy(streamSettings string, entries []any) (string, bool, error) {
	stream := map[string]any{}
	if streamSettings != "" {
		if err := json.Unmarshal([]byte(streamSettings), &stream); err != nil {
			return "", false, err
		}
	}
	previous, _ := json.Marshal(stream["externalProxy"])
	next, err := json.Marshal(entries)
	if err != nil {
		return "", false, err
	}
	if string(previous) == string(next) {
		return streamSettings, false, nil
	}
	stream["externalProxy"] = entries
	out, err := json.MarshalIndent(stream, "", "  ")
	if err != nil {
		return "", false, err
	}
	return string(out), true, nil
}

// ToExternalProxyEntry projects a Host onto the legacy `externalProxy` entry
// shape (the array that still lives in an inbound's streamSettings and that the
// share-link renderers consume).
//
// Only the fields the legacy entry actually has are emitted — host-only extras
// (path/hostHeader/mux/sockopt/finalMask/…) are added by the subscription layer
// on top of this projection, so the persisted mirror written by the panel stays
// a valid externalProxy array.
//
// `defaultDest`/`defaultPort` fill in for a host that leaves address/port blank
// (an override-only host inherits the inbound's own address and port).
func (h *Host) ToExternalProxyEntry(defaultDest string, defaultPort int) map[string]any {
	dest := h.Address
	if dest == "" {
		dest = defaultDest
	}
	port := h.Port
	if port == 0 {
		port = defaultPort
	}
	ep := map[string]any{
		"forceTls": HostSecurityToForceTls(h.Security),
		"dest":     dest,
		"port":     float64(port),
		"remark":   h.Remark,
	}
	sni := h.Sni
	if h.OverrideSniFromAddress {
		sni = dest
	}
	if !h.KeepSniBlank && sni != "" {
		ep["sni"] = sni
	}
	if h.Fingerprint != "" {
		ep["fingerprint"] = h.Fingerprint
	}
	if alpn := nonEmptyAnySlice(h.Alpn); len(alpn) > 0 {
		ep["alpn"] = alpn
	}
	if pins := nonEmptyAnySlice(h.PinnedPeerCertSha256); len(pins) > 0 {
		ep["pinnedPeerCertSha256"] = pins
	}
	if h.EchConfigList != "" {
		ep["echConfigList"] = h.EchConfigList
	}
	if h.VerifyPeerCertByName != "" {
		ep["verifyPeerCertByName"] = h.VerifyPeerCertByName
	}
	return ep
}

// HostSecurityToForceTls maps Host.Security onto the externalProxy forceTls
// vocabulary. "reality"/"same"/"" all keep the inbound's base security ("same")
// — reality parameters can only come from the inbound itself.
func HostSecurityToForceTls(security string) string {
	switch security {
	case "tls", "none":
		return security
	default:
		return "same"
	}
}

func nonEmptyAnySlice(in []string) []any {
	out := make([]any, 0, len(in))
	for _, s := range in {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

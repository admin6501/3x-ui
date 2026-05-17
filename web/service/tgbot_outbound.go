package service

import (
	"encoding/json"
	"fmt"
	"strings"
)

// TgBotOutboundPrefix marks the tgBotProxy setting value as referring to an
// Xray outbound tag (instead of a literal SOCKS5 URL). Format: "outbound:<tag>".
const TgBotOutboundPrefix = "outbound:"

// TgBotSocksInboundTag identifies the auto-managed local SOCKS5 inbound that
// the Telegram bot dials into when the admin selected an Xray outbound as the
// proxy. The tag is intentionally distinctive so the injection logic can find
// (and remove) the inbound/rule on later edits without misidentifying any
// admin-authored entries.
const TgBotSocksInboundTag = "x-ui-tgbot-socks"

// TgBotSocksInboundPort is the localhost port the auto-managed SOCKS5 inbound
// listens on. Kept fixed so the bot can resolve "outbound:<tag>" to a stable
// dialer URL without storing the port in settings.
const TgBotSocksInboundPort = 62792

// TgBotSocksDialerURL is the SOCKS5 URL the bot uses internally when the
// "outbound:<tag>" form has been resolved to the auto-managed inbound.
const TgBotSocksDialerURL = "socks5://127.0.0.1:62792"

// ParseTgBotOutboundTag returns the outbound tag if `proxyValue` is in the
// "outbound:<tag>" form, otherwise an empty string. Whitespace and a leading
// "/" (e.g. accidental "outbound://tag") are tolerated so admin typos in the
// settings page don't silently fail.
func ParseTgBotOutboundTag(proxyValue string) string {
	if !strings.HasPrefix(proxyValue, TgBotOutboundPrefix) {
		return ""
	}
	tag := strings.TrimPrefix(proxyValue, TgBotOutboundPrefix)
	tag = strings.TrimLeft(tag, "/")
	return strings.TrimSpace(tag)
}

// EnsureTgBotSocksInbound rewrites the Xray template so it contains (or no
// longer contains) the auto-managed SOCKS5 inbound + routing rule used by the
// Telegram bot proxy feature.
//
//   - If `outboundTag` is empty, the inbound and matching routing rule are
//     removed from the template (idempotent: no-op when they aren't there).
//   - Otherwise the inbound is added if missing, the routing rule is added or
//     updated to point at `outboundTag`, and the rule is placed at the front
//     of the routing rules list so it can't be shadowed by broader rules.
//
// The function returns the (possibly unchanged) template and `true` if any
// modification was made. Callers persist the new template via
// XraySettingService.SaveXraySetting and trigger an Xray restart only when
// `changed` is true.
func EnsureTgBotSocksInbound(template string, outboundTag string) (string, bool, error) {
	var cfg map[string]any
	if err := json.Unmarshal([]byte(template), &cfg); err != nil {
		return template, false, fmt.Errorf("parse xray template: %w", err)
	}

	changed := false
	inbounds, _ := cfg["inbounds"].([]any)
	hasInbound := false
	filteredInbounds := make([]any, 0, len(inbounds))
	for _, ib := range inbounds {
		ibMap, ok := ib.(map[string]any)
		if !ok {
			filteredInbounds = append(filteredInbounds, ib)
			continue
		}
		if tag, _ := ibMap["tag"].(string); tag == TgBotSocksInboundTag {
			if outboundTag == "" {
				changed = true
				continue
			}
			hasInbound = true
		}
		filteredInbounds = append(filteredInbounds, ib)
	}

	if outboundTag != "" && !hasInbound {
		filteredInbounds = append(filteredInbounds, map[string]any{
			"tag":      TgBotSocksInboundTag,
			"listen":   "127.0.0.1",
			"port":     TgBotSocksInboundPort,
			"protocol": "socks",
			"settings": map[string]any{
				"auth": "noauth",
				"udp":  true,
			},
		})
		changed = true
	}
	cfg["inbounds"] = filteredInbounds

	routing, _ := cfg["routing"].(map[string]any)
	if routing == nil {
		routing = map[string]any{}
	}
	rules, _ := routing["rules"].([]any)
	filteredRules := make([]any, 0, len(rules))
	for _, r := range rules {
		rMap, ok := r.(map[string]any)
		if !ok {
			filteredRules = append(filteredRules, r)
			continue
		}
		if isTgBotRule(rMap) {
			changed = true
			continue
		}
		filteredRules = append(filteredRules, r)
	}
	if outboundTag != "" {
		newRule := map[string]any{
			"type":        "field",
			"inboundTag":  []any{TgBotSocksInboundTag},
			"outboundTag": outboundTag,
		}
		filteredRules = append([]any{newRule}, filteredRules...)
		changed = true
	}
	routing["rules"] = filteredRules
	cfg["routing"] = routing

	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return template, false, fmt.Errorf("marshal xray template: %w", err)
	}
	return string(out), changed, nil
}

// isTgBotRule recognises the auto-managed routing rule by its single-element
// inboundTag list pointing at TgBotSocksInboundTag. Rules with the same
// inbound tag but additional fields are still treated as ours: there is no
// legitimate reason for an admin-authored rule to reference this tag.
func isTgBotRule(rule map[string]any) bool {
	switch tags := rule["inboundTag"].(type) {
	case []any:
		for _, t := range tags {
			if s, _ := t.(string); s == TgBotSocksInboundTag {
				return true
			}
		}
	case string:
		return tags == TgBotSocksInboundTag
	}
	return false
}

package service

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v2/util/json_util"
	"github.com/mhsanaei/3x-ui/v2/xray"
)

// tgBotOutboundInbound is the tag used for the synthetic local SOCKS5
// inbound that exposes the chosen Xray outbound to the Telegram bot.
const tgBotOutboundInbound = "tgbot-out"

// tgBotSocks tracks the local SOCKS5 endpoint used by the Telegram bot
// when an Xray outbound is selected. The XrayService writes the port
// when it builds the runtime config; the Tgbot service reads it when
// it constructs the fasthttp client. A value of 0 means "no Xray
// routing currently active".
var tgBotSocks struct {
	mu   sync.RWMutex
	port int
}

// setTgBotSocksPort records the local SOCKS5 port that the running
// Xray instance is listening on for Telegram bot egress. Pass 0 to
// signal that Xray-based bot routing is no longer active.
func setTgBotSocksPort(port int) {
	tgBotSocks.mu.Lock()
	tgBotSocks.port = port
	tgBotSocks.mu.Unlock()
}

// GetTgBotSocksPort returns the local SOCKS5 port the Telegram bot
// should dial when routing through an Xray outbound, or 0 if no Xray
// routing is currently configured.
func GetTgBotSocksPort() int {
	tgBotSocks.mu.RLock()
	defer tgBotSocks.mu.RUnlock()
	return tgBotSocks.port
}

// injectTgBotOutboundRouting mutates the supplied Xray config so that
// traffic arriving on a fresh local SOCKS5 inbound is sent out via the
// outbound whose tag matches outboundTag. If outboundTag does not
// resolve to a known outbound the config is returned unchanged and the
// caller should fall back to direct/proxy connectivity.
//
// Why a SOCKS5 inbound: the Telegram bot library (telego + fasthttp)
// has a built-in SOCKS5 dialer hook (fasthttpproxy.FasthttpSocksDialer).
// Exposing the chosen outbound as a local SOCKS5 server is the
// least-invasive way to reuse that hook, avoids a second Xray process,
// and keeps the bot's connection lifecycle tied to Xray's restart cycle
// — exactly what users expect when they edit either setting.
//
// The new routing rule is inserted right after the api->api rule (which
// EnsureStatsRouting pins at index 0) so it always wins over a generic
// catch-all the admin may have added later.
func injectTgBotOutboundRouting(cfg *xray.Config, outboundTag string, port int) bool {
	if cfg == nil || outboundTag == "" || port <= 0 {
		return false
	}

	// Verify the outbound exists; if it's been removed since the setting
	// was saved, do nothing — we don't want to spawn a SOCKS inbound
	// that routes to a missing tag (xray would error out at startup).
	var outbounds []map[string]any
	if len(cfg.OutboundConfigs) > 0 {
		_ = json.Unmarshal(cfg.OutboundConfigs, &outbounds)
	}
	found := false
	for _, ob := range outbounds {
		if tag, _ := ob["tag"].(string); tag == outboundTag {
			found = true
			break
		}
	}
	if !found {
		return false
	}

	// Build the new SOCKS inbound (loopback only, no auth — anyone with
	// shell access on the panel host already controls the bot).
	cfg.InboundConfigs = append(cfg.InboundConfigs, xray.InboundConfig{
		Tag:      tgBotOutboundInbound,
		Listen:   json_util.RawMessage(`"127.0.0.1"`),
		Port:     port,
		Protocol: "socks",
		Settings: json_util.RawMessage(`{"auth":"noauth","udp":true}`),
	})

	// Read the existing routing block (it may already contain user
	// rules) and prepend our rule after the api rule.
	var routing map[string]any
	if len(cfg.RouterConfig) > 0 {
		_ = json.Unmarshal(cfg.RouterConfig, &routing)
	}
	if routing == nil {
		routing = map[string]any{"domainStrategy": "AsIs"}
	}
	rules, _ := routing["rules"].([]any)

	tgRule := map[string]any{
		"type":        "field",
		"inboundTag":  []string{tgBotOutboundInbound},
		"outboundTag": outboundTag,
	}

	// Place after the api rule (index 0) if present, else at the front.
	insertAt := 0
	if len(rules) > 0 {
		if first, ok := rules[0].(map[string]any); ok {
			if outTag, _ := first["outboundTag"].(string); outTag == "api" {
				insertAt = 1
			}
		}
	}
	newRules := make([]any, 0, len(rules)+1)
	newRules = append(newRules, rules[:insertAt]...)
	newRules = append(newRules, tgRule)
	newRules = append(newRules, rules[insertAt:]...)
	routing["rules"] = newRules

	routingJSON, err := json.Marshal(routing)
	if err != nil {
		return false
	}
	cfg.RouterConfig = json_util.RawMessage(routingJSON)
	return true
}


// waitForTgBotSocksPort waits up to `timeout` for the Xray-managed
// local SOCKS5 endpoint to be both registered (port != 0) and actually
// accepting TCP connections. Returns the port on success, or 0 if it
// never came up — the caller should then fall back to direct/proxy.
//
// The two-stage check matters: when the panel boots, the Tgbot service
// can race ahead of XrayService.RestartXray; without waiting for an
// accepted TCP handshake we'd hand fasthttp a proxy that will refuse
// the very first Telegram API call.
func waitForTgBotSocksPort(timeout time.Duration) int {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		port := GetTgBotSocksPort()
		if port > 0 {
			conn, err := net.DialTimeout("tcp",
				fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond)
			if err == nil {
				_ = conn.Close()
				return port
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return 0
}

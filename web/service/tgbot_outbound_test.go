package service

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseTgBotOutboundTag(t *testing.T) {
	cases := map[string]string{
		"":                          "",
		"socks5://1.2.3.4:1080":     "",
		"outbound:":                 "",
		"outbound:my-tag":           "my-tag",
		"outbound://my-tag":         "my-tag",
		"outbound:  my-tag  ":       "my-tag",
		"outbound:vps-japan":        "vps-japan",
		"outbound:foo bar":          "foo bar",
	}
	for input, want := range cases {
		got := ParseTgBotOutboundTag(input)
		if got != want {
			t.Errorf("ParseTgBotOutboundTag(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestEnsureTgBotSocksInbound_Add(t *testing.T) {
	template := `{
		"inbounds": [{"tag":"api","listen":"127.0.0.1","port":62789,"protocol":"tunnel"}],
		"outbounds": [{"tag":"direct","protocol":"freedom"},{"tag":"japan","protocol":"vless"}],
		"routing": {"rules":[{"type":"field","inboundTag":["api"],"outboundTag":"api"}]}
	}`

	out, changed, err := EnsureTgBotSocksInbound(template, "japan")
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}
	if !strings.Contains(out, TgBotSocksInboundTag) {
		t.Fatal("expected output to contain auto inbound tag")
	}
	if !strings.Contains(out, `"outboundTag": "japan"`) {
		t.Fatal("expected routing rule pointing to japan")
	}

	var cfg map[string]any
	if err := json.Unmarshal([]byte(out), &cfg); err != nil {
		t.Fatal(err)
	}
	inbounds := cfg["inbounds"].([]any)
	if len(inbounds) != 2 {
		t.Fatalf("expected 2 inbounds, got %d", len(inbounds))
	}
}

func TestEnsureTgBotSocksInbound_Remove(t *testing.T) {
	template := `{
		"inbounds": [
			{"tag":"api","port":62789,"protocol":"tunnel"},
			{"tag":"x-ui-tgbot-socks","listen":"127.0.0.1","port":62792,"protocol":"socks"}
		],
		"outbounds": [{"tag":"japan","protocol":"vless"}],
		"routing": {"rules":[
			{"type":"field","inboundTag":["x-ui-tgbot-socks"],"outboundTag":"japan"},
			{"type":"field","inboundTag":["api"],"outboundTag":"api"}
		]}
	}`

	out, changed, err := EnsureTgBotSocksInbound(template, "")
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}
	if strings.Contains(out, TgBotSocksInboundTag) {
		t.Fatalf("expected no auto inbound tag in output, got: %s", out)
	}
}

func TestEnsureTgBotSocksInbound_Update(t *testing.T) {
	template := `{
		"inbounds": [
			{"tag":"x-ui-tgbot-socks","listen":"127.0.0.1","port":62792,"protocol":"socks"}
		],
		"outbounds": [{"tag":"japan","protocol":"vless"},{"tag":"us","protocol":"vless"}],
		"routing": {"rules":[
			{"type":"field","inboundTag":["x-ui-tgbot-socks"],"outboundTag":"japan"}
		]}
	}`

	out, changed, err := EnsureTgBotSocksInbound(template, "us")
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}
	if !strings.Contains(out, `"outboundTag": "us"`) {
		t.Fatalf("expected rule to point to us, got: %s", out)
	}
	if strings.Contains(out, `"outboundTag": "japan"`) && strings.Contains(out, `"inboundTag": [`) {
		// Check we don't have duplicate rules
		var cfg map[string]any
		_ = json.Unmarshal([]byte(out), &cfg)
		routing := cfg["routing"].(map[string]any)
		rules := routing["rules"].([]any)
		for _, r := range rules {
			rm := r.(map[string]any)
			if isTgBotRule(rm) {
				if rm["outboundTag"] != "us" {
					t.Fatalf("auto rule should point to us, got: %v", rm["outboundTag"])
				}
			}
		}
	}
}

func TestEnsureTgBotSocksInbound_Idempotent(t *testing.T) {
	template := `{
		"inbounds": [{"tag":"api","port":62789,"protocol":"tunnel"}],
		"outbounds": [{"tag":"japan","protocol":"vless"}],
		"routing": {"rules":[]}
	}`

	out1, _, _ := EnsureTgBotSocksInbound(template, "japan")
	out2, changed2, _ := EnsureTgBotSocksInbound(out1, "japan")
	// Second call adds the rule again (since we remove and re-add) — that's
	// OK as long as there's still only one auto-rule.
	_ = changed2
	var cfg map[string]any
	_ = json.Unmarshal([]byte(out2), &cfg)
	routing := cfg["routing"].(map[string]any)
	rules := routing["rules"].([]any)
	autoCount := 0
	for _, r := range rules {
		if isTgBotRule(r.(map[string]any)) {
			autoCount++
		}
	}
	if autoCount != 1 {
		t.Fatalf("expected exactly 1 auto-rule after re-apply, got %d", autoCount)
	}
}

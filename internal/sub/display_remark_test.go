package sub

import (
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

// In every display context (the subscription info page's per-config copy rows
// and the panel's client link/QR views) the client identity must still show —
// previously only the inbound name was rendered, so {{EMAIL}} was dropped
// (reported bug). The name-only template is expanded, so the volatile usage
// info (traffic/expiry) is stripped but the email survives.
func TestGenHostRemarkDisplayKeepsEmail(t *testing.T) {
	s := &SubService{
		remarkTemplate:   "{{EMAIL}}|{{INBOUND}}|📊{{TRAFFIC_LEFT}}|⏳{{DAYS_LEFT}}D",
		subscriptionBody: false, // display context
		usageShown:       map[string]bool{},
		statsByEmail:     map[string]xray.ClientTraffic{},
	}
	ib := &model.Inbound{Remark: "Germany"}
	client := model.Client{Email: "john", Enable: true}

	got := s.genHostRemark(ib, client, "CDN", "tcp")
	if !strings.Contains(got, "john") {
		t.Fatalf("display remark %q must contain the client email 'john'", got)
	}
	if !strings.Contains(got, "Germany") {
		t.Fatalf("display remark %q must contain the inbound name 'Germany'", got)
	}
	// Usage info is stripped in the name-only display form.
	if strings.Contains(got, "📊") || strings.Contains(got, "D") && strings.Contains(got, "⏳") {
		t.Fatalf("display remark %q should not carry the volatile usage/expiry info", got)
	}
}

// genDisplayRemark falls back to "" (so callers use the config name) when no
// template is configured, and when the template has no name part at all.
func TestGenDisplayRemarkFallback(t *testing.T) {
	ib := &model.Inbound{Remark: "Germany"}
	client := model.Client{Email: "john", Enable: true}

	noTmpl := &SubService{statsByEmail: map[string]xray.ClientTraffic{}}
	if got := noTmpl.genDisplayRemark(ib, client, "", ""); got != "" {
		t.Fatalf("no template should yield empty display remark, got %q", got)
	}

	// Info-only template: name-only form is empty, so display remark is empty
	// and the caller falls back to the config name.
	infoOnly := &SubService{
		remarkTemplate: "{{TRAFFIC_LEFT}}",
		statsByEmail:   map[string]xray.ClientTraffic{},
	}
	if got := infoOnly.genHostRemark(ib, client, "", ""); got != "Germany" {
		t.Fatalf("info-only template should fall back to config name 'Germany', got %q", got)
	}
}

// The subscription body itself still emits the full template (usage info on the
// first link) — the display-context change must not regress that path.
func TestGenHostRemarkBodyStillFull(t *testing.T) {
	s := &SubService{
		remarkTemplate:   "{{EMAIL}}|{{INBOUND}}|⏳{{DAYS_LEFT}}D",
		subscriptionBody: true,
		usageShown:       map[string]bool{},
		statsByEmail:     map[string]xray.ClientTraffic{},
	}
	ib := &model.Inbound{Remark: "Germany"}
	client := model.Client{Email: "john", Enable: true, ExpiryTime: 0}

	got := s.genHostRemark(ib, client, "CDN", "tcp")
	if !strings.Contains(got, "john") || !strings.Contains(got, "Germany") {
		t.Fatalf("subscription-body remark %q must contain email and inbound name", got)
	}
}

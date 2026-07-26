package service

import (
	"encoding/json"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/web/entity"
)

// TestSubBrandingDefaultsAreInert pins the shipped defaults: a panel that has
// never touched branding must serve the stock subscription page, which is what
// the subscription controller decides from these two settings.
func TestSubBrandingDefaultsAreInert(t *testing.T) {
	setupSettingTestDB(t)
	s := &SettingService{}

	enabled, err := s.GetSubBrandingEnable()
	if err != nil {
		t.Fatalf("GetSubBrandingEnable: %v", err)
	}
	if enabled {
		t.Error("branding must be off by default")
	}

	doc, err := s.GetSubBranding()
	if err != nil {
		t.Fatalf("GetSubBranding: %v", err)
	}
	if doc != "" {
		t.Errorf("branding document must start empty, got %q", doc)
	}
}

// TestSubBrandingRoundTrip covers the path the editor uses: the document it
// writes is stored verbatim and read back byte for byte, so a saved design is
// never silently reshaped by the settings layer.
func TestSubBrandingRoundTrip(t *testing.T) {
	setupSettingTestDB(t)
	s := &SettingService{}

	document := `{"brandName":"Acme VPN","primaryColor":"#00b96b","showApps":false,"customCss":".brand-name{letter-spacing:1px}"}`
	all := &entity.AllSetting{
		SubBrandingEnable: true,
		SubBranding:       document,
		// Fields the update path insists on having a sane value for.
		WebPort:       2053,
		SubPort:       2096,
		SessionMaxAge: 60,
		SmtpPort:      587,
		LdapPort:      389,
		PageSize:      25,
	}
	if err := s.UpdateAllSetting(all); err != nil {
		t.Fatalf("UpdateAllSetting: %v", err)
	}

	enabled, err := s.GetSubBrandingEnable()
	if err != nil || !enabled {
		t.Fatalf("GetSubBrandingEnable = (%v, %v), want (true, nil)", enabled, err)
	}
	stored, err := s.GetSubBranding()
	if err != nil {
		t.Fatalf("GetSubBranding: %v", err)
	}
	if stored != document {
		t.Fatalf("stored document changed:\n got %s\nwant %s", stored, document)
	}
	// It must still be the JSON the page parses — a mangled document would be
	// dropped there and the branding would silently vanish.
	var parsed map[string]any
	if err := json.Unmarshal([]byte(stored), &parsed); err != nil {
		t.Fatalf("stored document is not valid JSON: %v", err)
	}
	if parsed["brandName"] != "Acme VPN" {
		t.Errorf("brandName = %v, want Acme VPN", parsed["brandName"])
	}
}

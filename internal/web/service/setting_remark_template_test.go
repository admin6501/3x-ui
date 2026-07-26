package service

import (
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/web/entity"
)

// TestRestoreBlankDefaultsRemarkTemplate pins the half of the Remark Template
// contract that the boot-time seeder deliberately does not handle: clearing the
// field in the UI and saving must store the default template again, not a blank
// value. Blanking it happens on save, so this is where it has to be healed.
func TestRestoreBlankDefaultsRemarkTemplate(t *testing.T) {
	svc := &SettingService{}
	def := defaultValueMap["remarkTemplate"]
	if def == "" {
		t.Fatal("default remarkTemplate must not be empty")
	}
	if !strings.Contains(strings.ToUpper(def), "EMAIL") {
		t.Fatalf("default remarkTemplate must carry the client-identity token, got %q", def)
	}

	for _, blank := range []string{"", "   ", "\t\n"} {
		all := &entity.AllSetting{RemarkTemplate: blank}
		svc.restoreBlankDefaults(all)
		if all.RemarkTemplate != def {
			t.Errorf("blank template %q: got %q, want the default %q", blank, all.RemarkTemplate, def)
		}
	}

	// A real template — including one without the email token — is the admin's
	// choice and must survive the save untouched.
	for _, custom := range []string{"{{INBOUND}}|📊{{TRAFFIC_LEFT}}", "{{EMAIL}}-{{INBOUND}}"} {
		all := &entity.AllSetting{RemarkTemplate: custom}
		svc.restoreBlankDefaults(all)
		if all.RemarkTemplate != custom {
			t.Errorf("custom template %q was rewritten to %q", custom, all.RemarkTemplate)
		}
	}
}

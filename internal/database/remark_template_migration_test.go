package database

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// TestSeedRemarkTemplateEmail verifies the one-time migration prepends the
// {{EMAIL}} token to an existing Remark Template that lacks it, is idempotent,
// and leaves templates that already reference the email untouched.
func TestSeedRemarkTemplateEmail(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dir)
	if err := InitDB(filepath.Join(dir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = CloseDB() })

	rerun := func() {
		db.Where("seeder_name = ?", "RemarkTemplateEmailTokenV2").Delete(&model.HistoryOfSeeders{})
		if err := seedRemarkTemplateEmail(); err != nil {
			t.Fatalf("seedRemarkTemplateEmail: %v", err)
		}
	}
	read := func() string {
		var s model.Setting
		db.Model(model.Setting{}).Where("key = ?", "remarkTemplate").First(&s)
		return s.Value
	}

	// Existing panel with a template missing the email token.
	db.Where("key = ?", "remarkTemplate").Delete(&model.Setting{})
	db.Create(&model.Setting{Key: "remarkTemplate", Value: "{{INBOUND}}|📊{{TRAFFIC_LEFT}}"})
	rerun()
	if !strings.HasPrefix(read(), "{{EMAIL}}|") {
		t.Errorf("expected {{EMAIL}} prepended, got %q", read())
	}

	// Idempotent: re-running must not prepend twice.
	rerun()
	if strings.Count(strings.ToUpper(read()), "EMAIL") != 1 {
		t.Errorf("migration not idempotent, got %q", read())
	}

	// A template that already has the email token (any brace style) is untouched.
	db.Model(model.Setting{}).Where("key = ?", "remarkTemplate").Update("value", "{EMAIL}")
	rerun()
	if read() != "{EMAIL}" {
		t.Errorf("existing email template must be left alone, got %q", read())
	}

	// A cleared value is *not* the seeder's business: restoring the default on a
	// blank template belongs to SettingService.UpdateAllSetting
	// (restoreBlankDefaults, covered by TestRestoreBlankDefaultsRemarkTemplate),
	// so a boot-time migration must leave the stored value alone.
	db.Model(model.Setting{}).Where("key = ?", "remarkTemplate").Update("value", "")
	rerun()
	if read() != "" {
		t.Errorf("empty template must be left alone by the seeder, got %q", read())
	}

	// Same for a whitespace-only value.
	db.Model(model.Setting{}).Where("key = ?", "remarkTemplate").Update("value", "   ")
	rerun()
	if read() != "   " {
		t.Errorf("blank template must be left alone by the seeder, got %q", read())
	}

	// Missing row -> stays missing. An unset key resolves to the in-code default
	// (which carries {{EMAIL}}) and is persisted the next time settings are saved.
	db.Where("key = ?", "remarkTemplate").Delete(&model.Setting{})
	rerun()
	var count int64
	db.Model(model.Setting{}).Where("key = ?", "remarkTemplate").Count(&count)
	if count != 0 {
		t.Errorf("missing template must not be created by the seeder, got %d row(s)", count)
	}
}

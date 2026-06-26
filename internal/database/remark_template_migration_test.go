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
		db.Where("seeder_name = ?", "RemarkTemplateEmailToken").Delete(&model.HistoryOfSeeders{})
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
}

package locale

import (
	"encoding/json"
	"testing"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

// TestTranslationFilesParse: go-i18n reserves a handful of leaf names (id, hash,
// description, the plural forms). A block that uses one as an ordinary key makes
// the whole file fail to parse, and every string in every language silently
// becomes its own message id. One bad key is a total outage, so it is worth a
// test rather than a code review.
func TestTranslationFilesParse(t *testing.T) {
	b := i18n.NewBundle(language.MustParse("en-US"))
	b.RegisterUnmarshalFunc("json", json.Unmarshal)
	if err := loadTranslationsFromDisk(b); err != nil {
		t.Fatalf("translation files do not parse: %v", err)
	}
	if len(b.LanguageTags()) < len(SupportedLangs) {
		t.Errorf("bundle carries %d languages, want at least %d", len(b.LanguageTags()), len(SupportedLangs))
	}
	// A key from each namespace, to catch a block that parsed but landed nowhere.
	loc := i18n.NewLocalizer(b, "fa-IR", DefaultLang)
	for _, key := range []string{"username", "tgbot.unlimited", "shopbot.unit.unlimited", "shopbot.btn.wallet"} {
		msg, err := loc.Localize(&i18n.LocalizeConfig{MessageID: key})
		if err != nil || msg == "" || msg == key {
			t.Errorf("Localize(%q) = %q, err %v", key, msg, err)
		}
	}
}

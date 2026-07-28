package locale

import (
	"sync"

	"github.com/nicksnyder/go-i18n/v2/i18n"
)

// SupportedLangs is every language the panel ships translations for, in the same
// order the settings picker shows them. The sales bot picks one of these.
var SupportedLangs = []string{
	"en-US", "fa-IR", "ar-EG", "es-ES", "id-ID", "ja-JP", "pt-BR",
	"ru-RU", "tr-TR", "uk-UA", "vi-VN", "zh-CN", "zh-TW",
}

// DefaultLang is what an unset or unrecognised language setting resolves to.
const DefaultLang = "en-US"

// IsSupportedLang reports whether a language tag is one the panel ships.
func IsSupportedLang(lang string) bool {
	for _, l := range SupportedLangs {
		if l == lang {
			return true
		}
	}
	return false
}

var (
	langLocalizersMu sync.Mutex
	langLocalizers   = map[string]*i18n.Localizer{}
)

// ForLang returns a localizer pinned to one language, cached across calls.
//
// The panel's other localizers are globals reassigned from a setting or a
// request header; a bot that has to render the same message set in a language of
// its own — and, to route its reply keyboard, has to read every language at once
// — needs them addressable individually instead.
//
// Every localizer falls back to English, so a key translated in only some
// languages still renders rather than coming back empty.
func ForLang(lang string) *i18n.Localizer {
	if !IsSupportedLang(lang) {
		lang = DefaultLang
	}
	langLocalizersMu.Lock()
	defer langLocalizersMu.Unlock()
	if loc, ok := langLocalizers[lang]; ok {
		return loc
	}
	// InitLocalizer may not have run (a test, or the sub server on its own).
	ensureBundle()
	loc := i18n.NewLocalizer(i18nBundle, lang, DefaultLang)
	langLocalizers[lang] = loc
	return loc
}

func resetLangLocalizers() {
	langLocalizersMu.Lock()
	defer langLocalizersMu.Unlock()
	clear(langLocalizers)
}

// Translate localizes one key through an explicit localizer. Unlike I18n it
// returns the key rather than an empty string when the key is missing, so a
// gap in a translation file shows up as something readable instead of a blank
// message.
func Translate(loc *i18n.Localizer, key string, params ...string) string {
	if loc == nil {
		return key
	}
	msg, err := loc.Localize(&i18n.LocalizeConfig{
		MessageID:    key,
		TemplateData: createTemplateData(params),
	})
	if err != nil || msg == "" {
		return key
	}
	return msg
}

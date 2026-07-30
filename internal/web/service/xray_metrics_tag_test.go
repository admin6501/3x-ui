package service

import "testing"

// TestObservatoryAcceptsNonAsciiTags: outbound tags are operator-chosen
// labels. The old ASCII-only filter silently dropped every outbound whose tag
// was written in another script — it simply never appeared in the observatory,
// with nothing explaining why.
func TestObservatoryAcceptsNonAsciiTags(t *testing.T) {
	for _, tag := range []string{
		"direct", "proxy-1", "node_2.eu",
		"خروجی-ایران", "прокси", "出口", "サーバー",
		"tag with spaces", "emoji-🚀",
	} {
		if !validObsTag(tag) {
			t.Errorf("rejected a legitimate tag %q", tag)
		}
	}
}

// TestObservatoryRejectsWhatCouldCorrupt keeps the checks that actually
// matter: bounds, valid UTF-8, and no control characters.
func TestObservatoryRejectsWhatCouldCorrupt(t *testing.T) {
	long := make([]byte, maxObsTagLength+1)
	for i := range long {
		long[i] = 'a'
	}
	for name, tag := range map[string]string{
		"empty":           "",
		"too long":        string(long),
		"newline":         "tag\nwith-newline",
		"carriage return": "tag\rwith-cr",
		"null byte":       "tag\x00null",
		"invalid utf-8":   "tag\xff\xfe",
		"escape sequence": "tag\x1b[31m",
	} {
		if validObsTag(tag) {
			t.Errorf("accepted %s: %q", name, tag)
		}
	}
}

package salesbot

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestNoRawTranslationKeyReachesTheUser scans the package's own source for a
// translation-key constant used anywhere other than inside a localizing call.
//
// This is a source scan rather than a behavioural test because that is what the
// bug actually looks like: the key resolves perfectly well, the copy is
// translated in all thirteen languages, and every assertion about the
// translations passes — but a caller passed the key itself to Telegram, so a
// Persian buyer saw the literal text "ibtn.pause" on a button. Nothing about the
// rendered strings can catch that; only the call site can.
//
// A comparison (`buttonKeyFor(text) == btnCancel`) is a legitimate use of the
// key as a key, and so is the declaration itself.
func TestNoRawTranslationKeyReachesTheUser(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(files) < 5 {
		t.Fatalf("found %d source files; the scan is looking in the wrong place", len(files))
	}

	keyIdent := regexp.MustCompile(`\b((?:btn|msg)[A-Z]\w*)`)
	// The calls that turn a key into text. Anything else is a leak.
	localizers := []string{"b.m(", "tt.s(", "tt.f(", "t.s(", "t.f("}

	var leaks []string
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") || path == "i18n.go" {
			continue // tests may hold keys; i18n.go declares them
		}
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		// A function whose name ends in "Key" returns a key on purpose; its
		// caller is the one that has to localize. Skip its whole body.
		inKeyFunc := false
		for n, line := range strings.Split(string(src), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(line, "func ") {
				inKeyFunc = strings.Contains(line, "Key(") || strings.Contains(line, "Key()")
			}
			if line == "}" {
				inKeyFunc = false
			}
			switch {
			case inKeyFunc,
				strings.HasPrefix(trimmed, "//"),
				strings.HasPrefix(trimmed, "case "),
				strings.Contains(line, "buttonKeyFor("):
				continue
			}
			for _, ident := range keyIdent.FindAllString(line, -1) {
				rest := line
				for _, call := range localizers {
					rest = strings.ReplaceAll(rest, call+ident, "")
					rest = strings.ReplaceAll(rest, call+" "+ident, "")
				}
				if regexp.MustCompile(`\b` + ident + `\b`).MatchString(rest) {
					leaks = append(leaks, path+":"+itoa(n+1)+": "+ident+" — "+trimmed)
				}
			}
		}
	}
	for _, leak := range leaks {
		t.Errorf("translation key used outside a localizing call: %s", leak)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

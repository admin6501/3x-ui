package salesbot

import (
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	xuilogger "github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"

	"github.com/op/go-logging"
)

// The bot logs when it skips a malformed admin id; without a logger backend
// that call panics on a nil logger, so the package needs one initialised.
func TestMain(m *testing.M) {
	xuilogger.InitLogger(logging.ERROR)
	m.Run()
}

// TestParseNumberAcceptsWhatPeopleActuallyType — an admin setting a price in a
// Persian keyboard types Persian digits, and copies prices with separators in
// them. Rejecting those would make the package wizard unusable.
func TestParseNumberAcceptsWhatPeopleActuallyType(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"500000", 500000, true},
		{"۵۰۰۰۰۰", 500000, true},  // Persian digits
		{"٥٠٠٠٠٠", 500000, true},  // Arabic-Indic digits
		{"500,000", 500000, true}, // ASCII separator
		{"۵۰۰٬۰۰۰", 500000, true}, // Persian thousands separator
		{"500 000", 500000, true}, // space
		{" 42 ", 42, true},        // surrounding whitespace
		{"0", 0, true},            // zero means unlimited, and must parse
		{"", 0, false},            // empty
		{"abc", 0, false},         // not a number
		{"12abc", 0, false},       // partially a number is still not one
		{"-5", 0, false},          // no negative quotas
	}
	for _, tc := range cases {
		got, ok := parseNumber(tc.in)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("parseNumber(%q) = %d,%v; want %d,%v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

// TestEscapeKeepsMarkupOutOfMessages: a config's name and the currency label
// are admin- or user-influenced text that lands inside an HTML-parse-mode
// message. Unescaped, a stray "<" breaks every message it appears in.
func TestEscapeKeepsMarkupOutOfMessages(t *testing.T) {
	card := configCard("<b>evil</b>&co", 10, 0, 0, true, "تومان", "")
	if strings.Contains(card, "<b>evil</b>&co") {
		t.Error("the config name was interpolated as raw markup")
	}
	if !strings.Contains(card, "&lt;b&gt;evil&lt;/b&gt;&amp;co") {
		t.Errorf("name not escaped in:\n%s", card)
	}
}

// TestQuotaWordingSpeaksUnlimited: the panel stores 0 for "no limit", and a
// buyer shown a bare "0 گیگابایت" would think they bought nothing.
func TestQuotaWordingSpeaksUnlimited(t *testing.T) {
	if got := quotaGB(0); got != "نامحدود" {
		t.Errorf("quotaGB(0) = %q, want نامحدود", got)
	}
	if got := quotaGB(100); !strings.Contains(got, "گیگابایت") {
		t.Errorf("quotaGB(100) = %q, want a gigabyte figure", got)
	}
}

// TestPricesReadAsPersian — prices are the thing buyers scan for, so they are
// grouped and rendered in Persian digits.
func TestPricesReadAsPersian(t *testing.T) {
	if got := faNum(1500000); got != "۱٬۵۰۰٬۰۰۰" {
		t.Errorf("faNum(1500000) = %q", got)
	}
	if got := faNum(0); got != "۰" {
		t.Errorf("faNum(0) = %q", got)
	}
	if got := faNum(-42); got != "-۴۲" {
		t.Errorf("faNum(-42) = %q", got)
	}
}

// TestProgressBarClamps keeps a reseller who overshot their quota from getting
// a bar longer than the bar.
func TestProgressBarClamps(t *testing.T) {
	full := progressBar(200, 100)
	if strings.Count(full, "█") != 10 || strings.Count(full, "░") != 0 {
		t.Errorf("over-quota bar = %q, want a full bar", full)
	}
	empty := progressBar(0, 100)
	if strings.Count(empty, "█") != 0 || strings.Count(empty, "░") != 10 {
		t.Errorf("empty bar = %q", empty)
	}
	if progressBar(5, 0) != "" {
		t.Error("an unlimited quota has no bar to draw")
	}
}

// TestSplitForTelegramNeverExceedsTheLimit: Telegram rejects an over-long
// message outright, which would silently drop a buyer's package list.
func TestSplitForTelegramNeverExceedsTheLimit(t *testing.T) {
	const limit = 3500
	long := strings.Repeat("پاراگراف نمونه برای تست تقسیم پیام.\n\n", 400)
	chunks := splitForTelegram(long)
	if len(chunks) < 2 {
		t.Fatalf("expected the message to be split, got %d chunk(s)", len(chunks))
	}
	for i, chunk := range chunks {
		if len(chunk) > limit {
			t.Errorf("chunk %d is %d bytes, over the %d limit", i, len(chunk), limit)
		}
	}
	// A single paragraph longer than the limit still has to be cut.
	for _, chunk := range splitForTelegram(strings.Repeat("x", limit*2+5)) {
		if len(chunk) > limit {
			t.Errorf("unbroken text was not hard-cut: %d bytes", len(chunk))
		}
	}
	if got := splitForTelegram("short"); len(got) != 1 || got[0] != "short" {
		t.Errorf("a short message must pass through untouched, got %v", got)
	}
}

// TestAdminIdParsingIgnoresJunk: the admin list is typed by hand into a
// settings field, so a trailing comma or a stray space must not lose an admin.
func TestAdminIdParsingIgnoresJunk(t *testing.T) {
	ids := parseAdminIds(" 123 , 456,, notanid ,789 ")
	want := []int64{123, 456, 789}
	if len(ids) != len(want) {
		t.Fatalf("parsed %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Errorf("parsed %v, want %v", ids, want)
			break
		}
	}
	if got := parseAdminIds(""); len(got) != 0 {
		t.Errorf("empty list parsed to %v", got)
	}
}

// TestOnlyConfiguredAdminsAreAdmins is the bot's authorisation check; the admin
// menu creates accounts and moves money, so a non-admin must never match.
func TestOnlyConfiguredAdminsAreAdmins(t *testing.T) {
	b := &Bot{states: newStateStore(), adminIds: []int64{111, 222}}
	if !b.isAdmin(111) || !b.isAdmin(222) {
		t.Error("a configured admin was rejected")
	}
	if b.isAdmin(333) || b.isAdmin(0) {
		t.Error("a stranger was accepted as admin")
	}
	empty := &Bot{states: newStateStore()}
	if empty.isAdmin(111) {
		t.Error("with no admins configured, nobody may run the admin side")
	}
}

// TestStateStoreIsPerChat keeps one buyer's half-finished order from leaking
// into another's conversation.
func TestStateStoreIsPerChat(t *testing.T) {
	s := newStateStore()
	s.set(1, &state{step: stepTopUpReceipt, orderId: 7})
	s.set(2, &state{step: stepBuyVolume})

	first, ok := s.get(1)
	if !ok || first.orderId != 7 || first.step != stepTopUpReceipt {
		t.Fatalf("chat 1 state = %+v", first)
	}
	second, ok := s.get(2)
	if !ok || second.step != stepBuyVolume {
		t.Fatalf("chat 2 state = %+v", second)
	}

	// The caller gets a copy: mutating it must not rewrite the stored state.
	first.orderId = 99
	again, _ := s.get(1)
	if again.orderId != 7 {
		t.Error("state store handed out a live pointer")
	}

	s.clear(1)
	if _, ok := s.get(1); ok {
		t.Error("cleared state came back")
	}
	if _, ok := s.get(2); !ok {
		t.Error("clearing one chat wiped another")
	}

	s.reset()
	if _, ok := s.get(2); ok {
		t.Error("reset left state behind")
	}
}

// TestSignedNumbersForBalanceCorrections: an admin fixing a balance downwards
// types a negative number, and may well type it with Persian digits.
func TestSignedNumbersForBalanceCorrections(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"50000", 50000, true},
		{"-50000", -50000, true},
		{"−۵۰۰۰۰", -50000, true}, // Unicode minus with Persian digits
		{"-۵۰٬۰۰۰", -50000, true},
		{"0", 0, true},
		{"abc", 0, false},
		{"-", 0, false},
	}
	for _, tc := range cases {
		got, ok := parseSignedNumber(tc.in)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("parseSignedNumber(%q) = %d,%v; want %d,%v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

// TestLinkHasHost: when the panel can name no address for an inbound, the share
// link builder still emits a link — with nothing where the server should be.
// Sending that to a buyer is worse than sending nothing, so it has to be caught.
func TestLinkHasHost(t *testing.T) {
	good := []string{
		"vless://11111111-1111-1111-1111-111111111111@example.com:443?type=tcp#node",
		"vmess://eyJhZGQiOiJ4In0=",
		"trojan://secret@1.2.3.4:443#tag",
		"ss://abcd@host:8388/?plugin=x#tag",
	}
	for _, link := range good {
		if !linkHasHost(link) {
			t.Errorf("linkHasHost(%q) = false, want true", link)
		}
	}
	bad := []string{
		"vless://11111111-1111-1111-1111-111111111111@:443?type=tcp#node",
		"trojan://secret@#tag",
		"ss://abcd@:8388#tag",
		"vless://11111111-1111-1111-1111-111111111111@/path",
		"not-a-link",
		"",
	}
	for _, link := range bad {
		if linkHasHost(link) {
			t.Errorf("linkHasHost(%q) = true, want false", link)
		}
	}
}

// TestSubscriptionLinkJoin: subURI is empty on a default install, and the old
// code turned that into a config card with no link at all — the bug reported in
// issue #24. An empty base must be reported as "no link" so the caller falls
// back to the direct config links instead of sending a buyer nothing.
func TestSubscriptionLinkJoin(t *testing.T) {
	cases := []struct {
		base, subID, want string
	}{
		{"https://sub.example.com:2096/sub/", "abc123", "https://sub.example.com:2096/sub/abc123"},
		{"https://sub.example.com:2096/sub", "abc123", "https://sub.example.com:2096/sub/abc123"},
		{"  https://sub.example.com/sub/  ", "abc123", "https://sub.example.com/sub/abc123"},
		{"", "abc123", ""},
		{"   ", "abc123", ""},
		{"https://sub.example.com/sub/", "", ""},
	}
	for _, tc := range cases {
		if got := joinSubLink(tc.base, tc.subID); got != tc.want {
			t.Errorf("joinSubLink(%q, %q) = %q, want %q", tc.base, tc.subID, got, tc.want)
		}
	}
}

// TestConfigCardOmitsAnEmptyLink: a card built without a subscription link must
// not show an empty "🔗 لینک اشتراک" heading — that is what made the reported
// message look like the bot had sent a broken link rather than none.
func TestConfigCardOmitsAnEmptyLink(t *testing.T) {
	if got := configCard("user@example.com", 10, 0, 0, true, "تومان", "   "); strings.Contains(got, "لینک اشتراک") {
		t.Errorf("blank link still drew a link section:\n%s", got)
	}
	got := configCard("user@example.com", 10, 0, 0, true, "تومان", "https://s.example.com/sub/x")
	if !strings.Contains(got, "https://s.example.com/sub/x") {
		t.Errorf("link missing from card:\n%s", got)
	}
}

// TestChannelNamesNormalise: the join-channel setting is typed by hand, so a
// link, a bare name and an @name all have to reach the same chat.
func TestChannelNamesNormalise(t *testing.T) {
	for _, in := range []string{"mychan", "@mychan", "t.me/mychan", "https://t.me/mychan", " @mychan "} {
		if got := normalizeChannel(in); got != "@mychan" {
			t.Errorf("normalizeChannel(%q) = %q, want @mychan", in, got)
		}
	}
}

// TestDiscountCodesNormaliseForMatching: buyers type a code off a poster, in
// whatever case and with a stray space. Matching has to be forgiving or the
// promotion looks broken.
func TestDiscountCodesNormaliseForMatching(t *testing.T) {
	for _, in := range []string{"NOWRUZ", "nowruz", " NoWruz ", "\tnowruz\n"} {
		if got := service.NormalizeDiscountCode(in); got != "NOWRUZ" {
			t.Errorf("NormalizeDiscountCode(%q) = %q, want NOWRUZ", in, got)
		}
	}
	if got := service.NormalizeDiscountCode("   "); got != "" {
		t.Errorf("a blank code normalised to %q, want empty", got)
	}
}

// TestDiscountButtonLabelShowsWhyACodeIsDead: an admin scanning the code list
// needs to see at a glance which codes are still working.
func TestDiscountButtonLabelShowsWhyACodeIsDead(t *testing.T) {
	live := &model.DiscountCode{Code: "LIVE", Percent: 20, Enabled: true}
	if got := discountButtonLabel(live); !strings.HasPrefix(got, "🟢") {
		t.Errorf("live code label = %q", got)
	}
	off := &model.DiscountCode{Code: "OFF", Percent: 20, Enabled: false}
	if got := discountButtonLabel(off); !strings.HasPrefix(got, "⛔️") {
		t.Errorf("disabled code label = %q", got)
	}
	expired := &model.DiscountCode{Code: "OLD", Percent: 20, Enabled: true,
		ExpiresAt: time.Now().AddDate(0, 0, -1).UnixMilli()}
	if got := discountButtonLabel(expired); !strings.HasPrefix(got, "⌛️") {
		t.Errorf("expired code label = %q", got)
	}
	spent := &model.DiscountCode{Code: "SPENT", Percent: 20, Enabled: true, MaxUses: 3, Used: 3}
	if got := discountButtonLabel(spent); !strings.HasPrefix(got, "🔚") {
		t.Errorf("used-up code label = %q", got)
	}
}

// TestConfigAndUserLabelsEscapeNothingButStayReadable: these are button
// captions, not HTML, so they must carry the raw name — Telegram does not parse
// markup in a button — while still telling the two states apart.
func TestListLabelsCarryStateAndName(t *testing.T) {
	paused := &model.BotConfig{Email: "tg1_abcd", VolumeGB: 20, Active: false, Paused: true}
	if got := configButtonLabel(paused); !strings.HasPrefix(got, "⏸") || !strings.Contains(got, "tg1_abcd") {
		t.Errorf("paused config label = %q", got)
	}
	off := &model.BotConfig{Email: "tg2_efgh", VolumeGB: 20, Active: false}
	if got := configButtonLabel(off); !strings.HasPrefix(got, "⛔️") {
		t.Errorf("suspended config label = %q", got)
	}

	blocked := &model.BotUser{TelegramId: 42, FirstName: "Ali", Username: "ali", Blocked: true}
	got := userButtonLabel(blocked, "تومان")
	if !strings.HasPrefix(got, "⛔️") || !strings.Contains(got, "Ali") || !strings.Contains(got, "@ali") {
		t.Errorf("blocked user label = %q", got)
	}
	// A user who never set a name still has to be pickable.
	anon := &model.BotUser{TelegramId: 4242}
	if got := userButtonLabel(anon, "تومان"); !strings.Contains(got, "4242") {
		t.Errorf("anonymous user label = %q", got)
	}
}

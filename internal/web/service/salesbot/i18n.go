package salesbot

import (
	"fmt"
	"strings"
	"sync"

	"github.com/mhsanaei/3x-ui/v3/internal/web/locale"
)

// keyPrefix namespaces the bot's strings inside the panel's translation files,
// so the shop's copy is translated alongside everything else rather than in a
// second set of files nobody remembers to update.
const keyPrefix = "shopbot."

// tr renders the bot's copy in one language. Every string a buyer or a shop
// admin sees goes through it, including numbers and byte counts — a Persian
// shop wants Persian digits, an English one does not, and that decision belongs
// with the language rather than scattered through the message builders.
type tr struct {
	lang string
}

// newTr returns a translator for a language tag, falling back to the panel's
// default for anything unrecognised.
func newTr(lang string) *tr {
	if !locale.IsSupportedLang(lang) {
		lang = locale.DefaultLang
	}
	return &tr{lang: lang}
}

// s localizes one key. Params are "name==value" pairs, matching the panel's
// existing i18n convention.
func (t *tr) s(key string, params ...string) string {
	return locale.Translate(locale.ForLang(t.lang), keyPrefix+key, params...)
}

// f localizes a key whose translation carries fmt verbs. Keeping the verbs in
// the translated string is what lets a language reorder them.
func (t *tr) f(key string, args ...any) string {
	return fmt.Sprintf(t.s(key), args...)
}

// easternDigits are the languages whose readers expect their own numerals.
var easternDigits = map[string][]rune{
	"fa-IR": {'۰', '۱', '۲', '۳', '۴', '۵', '۶', '۷', '۸', '۹'},
	"ar-EG": {'٠', '١', '٢', '٣', '٤', '٥', '٦', '٧', '٨', '٩'},
}

// groupSeparator is the thousands mark. Persian and Arabic use their own.
func (t *tr) groupSeparator() rune {
	switch t.lang {
	case "fa-IR":
		return '٬'
	case "ar-EG":
		return '٬'
	default:
		return ','
	}
}

// num renders an integer with thousands separators, in the language's own
// numerals. Prices are the thing a buyer scans for, so they have to read
// naturally.
func (t *tr) num(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	digits := fmt.Sprintf("%d", n)
	sep := t.groupSeparator()
	var grouped strings.Builder
	for i, r := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			grouped.WriteRune(sep)
		}
		grouped.WriteRune(r)
	}
	out := t.digits(grouped.String())
	if neg {
		return "-" + out
	}
	return out
}

// digits rewrites ASCII digits into the language's numerals, leaving everything
// else alone.
func (t *tr) digits(s string) string {
	table, ok := easternDigits[t.lang]
	if !ok {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(table[r-'0'])
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// percent renders a percentage with the language's numerals and sign.
func (t *tr) percent(n int64) string {
	return t.f("fmt.percent", t.num(n))
}

// quota states a traffic cap, speaking the panel's "0 means unlimited"
// convention rather than showing a bare zero.
func (t *tr) quota(gb int64) string {
	if gb <= 0 {
		return t.s("unit.unlimited")
	}
	return t.f("fmt.gigabytes", t.num(gb))
}

// bytes formats a byte count at the largest unit that keeps it readable.
func (t *tr) bytes(n int64) string {
	const unit = 1024
	if n < unit {
		return t.f("fmt.bytes", t.num(n))
	}
	keys := []string{"unit.kb", "unit.mb", "unit.gb", "unit.tb"}
	value := float64(n)
	idx := -1
	for value >= unit && idx < len(keys)-1 {
		value /= unit
		idx++
	}
	return t.digits(fmt.Sprintf("%.2f", value)) + " " + t.s(keys[idx])
}

// progressBar draws a ten-segment usage bar, which reads far better on a phone
// than a bare percentage.
func (t *tr) progressBar(used, total float64) string {
	if total <= 0 {
		return ""
	}
	ratio := used / total
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	filled := int(ratio*10 + 0.5)
	return strings.Repeat("█", filled) + strings.Repeat("░", 10-filled) +
		" " + t.percent(int64(ratio*100+0.5))
}

// ------------------------------------------------------- button routing --

// buttonKeys are every reply-keyboard caption the router has to recognise. The
// captions themselves are translated, so the switch matches on these stable keys
// instead of on the text.
var buttonKeys = []string{
	"btn.wallet", "btn.topUp", "btn.buyConfig", "btn.myConfigs", "btn.ledger",
	"btn.prices", "btn.support", "btn.help", "btn.mainMenu", "btn.back",
	"btn.cancel", "btn.skip", "btn.joined", "btn.admin",
	"btn.adminTopUps", "btn.adminUsers", "btn.adminConfigs", "btn.adminStats",
	"btn.adminCodes", "btn.adminBroadcast", "btn.adminExit",
}

var (
	buttonIndexOnce sync.Once
	buttonIndex     map[string]string
)

// buildButtonIndex maps every caption in every shipped language back to its key.
//
// Matching only the current language would leave a keyboard rendered before a
// language change permanently dead: Telegram keeps showing the old captions
// until the user reopens the menu, and every tap would fall through to the help
// text. Reading all of them costs one map lookup per message and makes the
// switch survive the change.
func buildButtonIndex() {
	buttonIndex = make(map[string]string, len(buttonKeys)*len(locale.SupportedLangs))
	for _, lang := range locale.SupportedLangs {
		t := newTr(lang)
		for _, key := range buttonKeys {
			caption := strings.TrimSpace(t.s(key))
			if caption == "" || caption == keyPrefix+key {
				continue
			}
			// First language to claim a caption keeps it. Identical captions
			// across languages mean the same button anyway.
			if _, taken := buttonIndex[caption]; !taken {
				buttonIndex[caption] = key
			}
		}
	}
}

// buttonKeyFor maps what the user tapped back to a stable key, or "" if the text
// is not one of the bot's buttons.
func buttonKeyFor(text string) string {
	buttonIndexOnce.Do(buildButtonIndex)
	return buttonIndex[strings.TrimSpace(text)]
}

// esc escapes the three characters Telegram's HTML parse mode cares about, so a
// config name containing "<" can never break the message or inject markup.
func esc(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}

// Translation keys, named after the constants they replaced so call sites read
// the same as before. A reply-keyboard key is both the caption's key and what
// buttonKeyFor returns, so the router switches on these directly.
const (
	btnWallet         = "btn.wallet"
	btnTopUp          = "btn.topUp"
	btnBuyCfg         = "btn.buyConfig"
	btnMyCfgs         = "btn.myConfigs"
	btnLedger         = "btn.ledger"
	btnPrices         = "btn.prices"
	btnJoined         = "btn.joined"
	btnSupport        = "btn.support"
	btnHelp           = "btn.help"
	btnAdmin          = "btn.admin"
	btnBack           = "btn.back"
	btnCancel         = "btn.cancel"
	btnSkip           = "btn.skip"
	btnMainMenu       = "btn.mainMenu"
	btnAdminTop       = "btn.adminTopUps"
	btnAdminUsr       = "btn.adminUsers"
	btnAdminCfg       = "btn.adminConfigs"
	btnAdminStats     = "btn.adminStats"
	btnAdminCodes     = "btn.adminCodes"
	btnAdminBroadcast = "btn.adminBroadcast"
	btnAdminExit      = "btn.adminExit"
)

// Inline-keyboard captions. These travel in callback data by their own action
// name, so they never have to be matched back from their text.
const (
	btnCfgLinks     = "ibtn.links"
	btnCfgAddVol    = "ibtn.addVolume"
	btnCfgPause     = "ibtn.pause"
	btnCfgResume    = "ibtn.resume"
	btnCfgDelete    = "ibtn.delete"
	btnCfgDeleteYes = "ibtn.deleteYes"
	btnCfgBack      = "ibtn.back"
	btnHaveCode     = "ibtn.haveCode"
	btnNewCode      = "ibtn.newCode"
	btnNameAuto     = "ibtn.nameAuto"
	btnNameCustom   = "ibtn.nameCustom"
	btnJoinChannel  = "ibtn.joinChannel"
	btnApprove      = "ibtn.approve"
	btnReject       = "ibtn.reject"
	btnAdjust       = "ibtn.adjustBalance"
	btnBlock        = "ibtn.block"
	btnUnblock      = "ibtn.unblock"
	btnUserConfigs  = "ibtn.userConfigs"
	btnUserLedger   = "ibtn.userLedger"
	btnDisable      = "ibtn.disable"
	btnEnable       = "ibtn.enable"
)

const (
	msgShopWelcome        = "msg.welcome"
	msgMustJoin           = "msg.mustJoin"
	msgNotJoinedYet       = "msg.notJoinedYet"
	msgJoinOk             = "msg.joinOk"
	msgAskTopUp           = "msg.askTopUp"
	msgAskVolume          = "msg.askVolume"
	msgNoConfigs          = "msg.noConfigs"
	msgNoLedger           = "msg.noLedger"
	msgNeedBalance        = "msg.needBalance"
	msgShopNoInbound      = "msg.noInbound"
	msgBlocked            = "msg.blocked"
	msgTopUpSent          = "msg.topUpSent"
	msgVolumeBad          = "msg.volumeBad"
	msgNoLinkYet          = "msg.noLinkYet"
	msgSuspended          = "msg.suspended"
	msgShopHelp           = "msg.help"
	msgPickConfig         = "msg.pickConfig"
	msgConfigGone         = "msg.configGone"
	msgConfigPaused       = "msg.configPaused"
	msgConfigResumed      = "msg.configResumed"
	msgConfigNeedsFunds   = "msg.configNeedsFunds"
	msgAskAddVolume       = "msg.askAddVolume"
	msgConfirmDelete      = "msg.confirmDelete"
	msgConfigDeleted      = "msg.configDeleted"
	msgPickUser           = "msg.pickUser"
	msgUserGone           = "msg.userGone"
	msgAskDiscountCode    = "msg.askDiscountCode"
	msgDiscountApplied    = "msg.discountApplied"
	msgDiscountUnknown    = "msg.discountUnknown"
	msgDiscountExpired    = "msg.discountExpired"
	msgDiscountUsedUp     = "msg.discountUsedUp"
	msgDiscountAlready    = "msg.discountAlready"
	msgNoDiscounts        = "msg.noDiscounts"
	msgPickDiscount       = "msg.pickDiscount"
	msgDiscountGone       = "msg.discountGone"
	msgAskNewDiscount     = "msg.askNewDiscount"
	msgDiscountFormatBad  = "msg.discountFormatBad"
	msgDiscountPercentBad = "msg.discountPercentBad"
	msgDiscountExists     = "msg.discountExists"
	msgDiscountCreated    = "msg.discountCreated"
	msgDiscountDeleted    = "msg.discountDeleted"
	msgReceiptOnlyPic     = "msg.receiptOnlyPic"
	msgNoSupport          = "msg.noSupport"
	msgNotAdmin           = "msg.notAdmin"
	msgAdminWelcome       = "msg.adminWelcome"
	msgLeftAdmin          = "msg.leftAdmin"
	msgSomethingWrong     = "msg.somethingWrong"
	msgOrderGone          = "msg.orderGone"
	msgAlreadyDecided     = "msg.alreadyDecided"
	msgAskRejectNote      = "msg.askRejectNote"
	msgAskBroadcast       = "msg.askBroadcast"
	msgCancelled          = "msg.cancelled"
	msgReceiptFirst       = "msg.receiptFirst"
	msgAmountOutOfRange   = "msg.amountOutOfRange"
	msgNoPendingTopUps    = "msg.noPendingTopUps"
	msgNoShopUsers        = "msg.noShopUsers"
	msgUserNoConfigs      = "msg.userNoConfigs"
	msgUserNoTx           = "msg.userNoTx"
	msgNoConfigsYet       = "msg.noConfigsYet"
	msgConfigCreated      = "msg.configCreated"
	msgMainMenu           = "msg.mainMenu"
	msgAskNameChoice      = "msg.askNameChoice"
	msgAskName            = "msg.askName"
	msgNameInvalid        = "msg.nameInvalid"
	msgNameTaken          = "msg.nameTaken"
)

// t returns a translator for the language the shop is configured to speak.
func (b *Bot) t() *tr {
	lang, _ := b.settingService.GetSalesBotLang()
	return newTr(lang)
}

// m localizes one key in the shop's language — the short form used wherever a
// message or a button caption is needed inline.
func (b *Bot) m(key string, params ...string) string {
	return b.t().s(key, params...)
}

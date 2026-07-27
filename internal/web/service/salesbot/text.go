// Package salesbot: every string the bot says, in one place. The bot talks to
// buyers in Persian, so the copy lives here as plain constants rather than in
// the panel's i18n bundles — those are for the panel UI and are picked per
// admin, while the bot's audience is the shop's customers.
package salesbot

import (
	"fmt"
	"strings"
)

// Buyer-facing button captions. They double as the reply-keyboard router keys,
// so changing one changes both the label and what it matches.
const (
	btnSupport  = "📞 پشتیبانی"
	btnHelp     = "❓ راهنما"
	btnAdmin    = "🛠 مدیریت"
	btnBack     = "🔙 بازگشت"
	btnCancel   = "✖️ انصراف"
	btnSkip     = "⏭ رد کردن"
	btnMainMenu = "🏠 منوی اصلی"
)

// Admin-side button captions.
const (
	btnAdminStats     = "📈 آمار فروش"
	btnAdminBroadcast = "📢 پیام همگانی"
	btnAdminExit      = "🚪 خروج از مدیریت"
)

const (
	msgReceiptOnlyPic = "لطفاً رسید را به صورت عکس ارسال کنید."
	msgNoSupport      = "اطلاعات پشتیبانی هنوز ثبت نشده است."
	msgNotAdmin       = "این بخش فقط برای مدیر است."
	msgAdminWelcome   = "پنل مدیریت فروشگاه 🛠"
	msgLeftAdmin      = "از بخش مدیریت خارج شدید."
	msgSomethingWrong = "خطایی رخ داد. لطفاً دوباره تلاش کنید."
	msgOrderGone      = "این درخواست دیگر در دسترس نیست."
	msgAlreadyDecided = "برای این درخواست قبلاً تصمیم‌گیری شده است."
	msgAskRejectNote  = "دلیل رد کردن را بنویسید (برای کاربر ارسال می‌شود)، یا «⏭ رد کردن» را بزنید."
	msgAskBroadcast   = "متنی که می‌خواهید برای همه‌ی کاربران ارسال شود را بنویسید:"
	msgCancelled      = "لغو شد."
)

// quotaGB and quotaCount speak the panel's "0 means unlimited" convention in
// the buyer's language instead of showing a bare zero.
func quotaGB(gb int64) string {
	if gb <= 0 {
		return "نامحدود"
	}
	return faNum(gb) + " گیگابایت"
}

// faNum renders a number with thousands separators in Persian digits, which is
// how prices are read in this market.
func faNum(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	digits := fmt.Sprintf("%d", n)
	var grouped strings.Builder
	for i, r := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			grouped.WriteRune('٬')
		}
		grouped.WriteRune(r)
	}
	out := toPersianDigits(grouped.String())
	if neg {
		return "-" + out
	}
	return out
}

var persianDigits = []rune{'۰', '۱', '۲', '۳', '۴', '۵', '۶', '۷', '۸', '۹'}

func toPersianDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(persianDigits[r-'0'])
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// esc escapes the three characters Telegram's HTML parse mode cares about, so a
// package name containing "<" can never break the message or inject markup.
func esc(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}

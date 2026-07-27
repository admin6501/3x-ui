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
	btnBuy      = "🛒 خرید پنل نمایندگی"
	btnMyPanel  = "👤 پنل من"
	btnTopUp    = "🔄 افزایش سهمیه"
	btnOrders   = "🧾 سفارش‌های من"
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
	btnAdminOrders    = "🧾 سفارش‌های در انتظار"
	btnAdminPackages  = "📦 مدیریت پکیج‌ها"
	btnAdminResellers = "👥 نمایندگان"
	btnAdminStats     = "📈 آمار فروش"
	btnAdminBroadcast = "📢 پیام همگانی"
	btnAdminExit      = "🚪 خروج از مدیریت"
)

const (
	msgDefaultWelcome = "به ربات فروش پنل نمایندگی خوش آمدید 🌟\n\n" +
		"از این ربات می‌توانید پنل نمایندگی تهیه کنید، سهمیه‌ی خود را افزایش دهید و وضعیت مصرفتان را ببینید.\n" +
		"برای شروع یکی از گزینه‌های زیر را انتخاب کنید."

	msgChoosePackage   = "یکی از پکیج‌های زیر را انتخاب کنید:"
	msgNoPackages      = "در حال حاضر پکیجی برای فروش تعریف نشده است. لطفاً بعداً دوباره سر بزنید."
	msgOrderCancelled  = "سفارش لغو شد."
	msgSendReceipt     = "پس از واریز، تصویر رسید پرداخت را همین‌جا ارسال کنید 📸"
	msgReceiptOnlyPic  = "لطفاً رسید را به صورت عکس ارسال کنید."
	msgReceiptGot      = "رسید شما دریافت شد ✅\n\nسفارش برای بررسی به مدیر ارسال شد. نتیجه از همین‌جا به شما اطلاع داده می‌شود."
	msgNoOrders        = "هنوز سفارشی ثبت نکرده‌اید."
	msgNoAccount       = "شما هنوز پنل نمایندگی ندارید. از گزینه‌ی «🛒 خرید پنل نمایندگی» می‌توانید تهیه کنید."
	msgNoSupport       = "اطلاعات پشتیبانی هنوز ثبت نشده است."
	msgNotAdmin        = "این بخش فقط برای مدیر است."
	msgAdminWelcome    = "پنل مدیریت ربات فروش 🛠"
	msgLeftAdmin       = "از بخش مدیریت خارج شدید."
	msgNoPendingOrders = "سفارشی در انتظار بررسی نیست ✅"
	msgSomethingWrong  = "خطایی رخ داد. لطفاً دوباره تلاش کنید."
	msgOrderGone       = "این سفارش دیگر در دسترس نیست."
	msgAlreadyDecided  = "برای این سفارش قبلاً تصمیم‌گیری شده است."
	msgNoReceiptYet    = "این سفارش هنوز رسید پرداخت ندارد."
	msgAskRejectNote   = "دلیل رد کردن سفارش را بنویسید (برای خریدار ارسال می‌شود)، یا «⏭ رد کردن» را بزنید."
	msgAskBroadcast    = "متنی که می‌خواهید برای همه‌ی خریداران ارسال شود را بنویسید:"
	msgCancelled       = "لغو شد."

	msgHelp = "<b>راهنمای ربات</b>\n\n" +
		"🛒 <b>خرید پنل نمایندگی</b> — یکی از پکیج‌ها را انتخاب می‌کنید، هزینه را واریز می‌کنید و رسید را می‌فرستید. " +
		"پس از تأیید مدیر، نام کاربری و رمز پنل شما همین‌جا ارسال می‌شود.\n\n" +
		"👤 <b>پنل من</b> — آدرس پنل، نام کاربری و میزان مصرف و سهمیه‌ی شما.\n\n" +
		"🔄 <b>افزایش سهمیه</b> — اگر سهمیه‌ی ترافیک یا تعداد کاربرتان رو به اتمام است، " +
		"با خرید دوباره‌ی یک پکیج به سهمیه‌ی فعلی‌تان اضافه می‌شود؛ حساب و کاربرانتان دست‌نخورده می‌مانند.\n\n" +
		"🧾 <b>سفارش‌های من</b> — تاریخچه و وضعیت سفارش‌های شما."
)

// packageCard renders one package the way a buyer sees it in the list.
func packageCard(name, description string, price int64, currency string, trafficGB int64, clientQuota, durationDays int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<b>%s</b>\n", esc(name))
	if strings.TrimSpace(description) != "" {
		fmt.Fprintf(&b, "%s\n", esc(description))
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "📶 ترافیک: <b>%s</b>\n", quotaGB(trafficGB))
	fmt.Fprintf(&b, "👥 تعداد کاربر: <b>%s</b>\n", quotaCount(clientQuota))
	if durationDays > 0 {
		fmt.Fprintf(&b, "🗓 مدت: <b>%s روز</b>\n", faNum(int64(durationDays)))
	}
	fmt.Fprintf(&b, "💰 قیمت: <b>%s %s</b>", faNum(price), esc(currency))
	return b.String()
}

// payInstructions is the screen a buyer sees between picking a package and
// uploading a receipt.
func payInstructions(orderId int, price int64, currency, payText string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "🧾 سفارش شماره <b>%s</b> ثبت شد.\n\n", faNum(int64(orderId)))
	fmt.Fprintf(&b, "💰 مبلغ قابل پرداخت: <b>%s %s</b>\n\n", faNum(price), esc(currency))
	if strings.TrimSpace(payText) != "" {
		b.WriteString(esc(payText))
		b.WriteString("\n\n")
	} else {
		b.WriteString("اطلاعات پرداخت هنوز توسط مدیر ثبت نشده است؛ لطفاً با پشتیبانی تماس بگیرید.\n\n")
	}
	b.WriteString(msgSendReceipt)
	return b.String()
}

// credentialsMessage is the payoff: what a buyer gets after approval.
func credentialsMessage(panelUrl, username, password string, trafficGB int64, clientQuota int) string {
	var b strings.Builder
	b.WriteString("🎉 سفارش شما تأیید شد!\n\n")
	b.WriteString("<b>مشخصات پنل نمایندگی شما:</b>\n")
	if strings.TrimSpace(panelUrl) != "" {
		fmt.Fprintf(&b, "🔗 آدرس پنل: %s\n", esc(panelUrl))
	}
	fmt.Fprintf(&b, "👤 نام کاربری: <code>%s</code>\n", esc(username))
	fmt.Fprintf(&b, "🔑 رمز عبور: <code>%s</code>\n\n", esc(password))
	fmt.Fprintf(&b, "📶 سهمیه ترافیک: <b>%s</b>\n", quotaGB(trafficGB))
	fmt.Fprintf(&b, "👥 سقف کاربر: <b>%s</b>\n\n", quotaCount(clientQuota))
	b.WriteString("⚠️ لطفاً پس از اولین ورود، رمز عبور خود را تغییر دهید و این پیام را در جای امنی نگه دارید.")
	return b.String()
}

// topUpMessage is the payoff for a repeat purchase.
func topUpMessage(username string, trafficGB int64, clientQuota int) string {
	var b strings.Builder
	b.WriteString("✅ سهمیه‌ی شما افزایش یافت!\n\n")
	fmt.Fprintf(&b, "👤 نام کاربری: <code>%s</code>\n", esc(username))
	fmt.Fprintf(&b, "📶 سهمیه ترافیک جدید: <b>%s</b>\n", quotaGB(trafficGB))
	fmt.Fprintf(&b, "👥 سقف کاربر جدید: <b>%s</b>", quotaCount(clientQuota))
	return b.String()
}

// quotaGB and quotaCount speak the panel's "0 means unlimited" convention in
// the buyer's language instead of showing a bare zero.
func quotaGB(gb int64) string {
	if gb <= 0 {
		return "نامحدود"
	}
	return faNum(gb) + " گیگابایت"
}

func quotaCount(n int) string {
	if n <= 0 {
		return "نامحدود"
	}
	return faNum(int64(n)) + " کاربر"
}

// statusLabel turns an order status into something a buyer understands.
func statusLabel(status string) string {
	switch status {
	case "pending":
		return "⏳ در انتظار پرداخت"
	case "review":
		return "🔎 در حال بررسی"
	case "approved":
		return "✅ تأیید شده"
	case "rejected":
		return "❌ رد شده"
	}
	return status
}

func kindLabel(kind string) string {
	if kind == "renew" {
		return "افزایش سهمیه"
	}
	return "خرید جدید"
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

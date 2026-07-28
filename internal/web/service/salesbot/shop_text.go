package salesbot

import (
	"fmt"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// Buyer-facing buttons for the wallet shop. They double as router keys.
const (
	btnWallet   = "💰 کیف پول من"
	btnTopUp    = "➕ شارژ کیف پول"
	btnBuyCfg   = "🛒 خرید کانفیگ"
	btnMyCfgs   = "📱 کانفیگ‌های من"
	btnLedger   = "💳 تراکنش‌ها"
	btnPrices   = "🏷 تعرفه"
	btnJoined   = "✅ عضو شدم"
	btnAdminTop = "💳 درخواست‌های شارژ"
	btnAdminUsr = "👥 کاربران"
	btnAdminCfg = "📱 کانفیگ‌ها"
)

// Inline buttons on one config's own management screen.
const (
	btnCfgLinks     = "🔗 لینک‌ها"
	btnCfgAddVol    = "➕ افزودن حجم"
	btnCfgPause     = "⏸ غیرفعال کردن"
	btnCfgResume    = "▶️ فعال کردن"
	btnCfgDelete    = "🗑 حذف"
	btnCfgDeleteYes = "🗑 بله، حذف کن"
	btnCfgBack      = "🔙 بازگشت"
	btnHaveCode     = "🏷 کد تخفیف دارم"
	btnNewCode      = "➕ کد تخفیف جدید"
)

const (
	msgShopWelcome = "به فروشگاه خوش آمدید 🌟\n\n" +
		"اینجا اول کیف پولتان را شارژ می‌کنید، بعد هر مقدار حجم که بخواهید کانفیگ می‌سازید.\n" +
		"<b>هزینه فقط بابت چیزی است که واقعاً مصرف می‌کنید</b> — اگر از ۱۰ گیگ فقط ۱ گیگ استفاده کنید، تنها بهای همان ۱ گیگ از کیف پولتان کم می‌شود."

	msgMustJoin      = "برای استفاده از ربات، ابتدا در کانال زیر عضو شوید و سپس «✅ عضو شدم» را بزنید:"
	msgNotJoinedYet  = "هنوز عضویت شما تأیید نشد. لطفاً در کانال عضو شوید و دوباره تلاش کنید."
	msgJoinOk        = "عضویت تأیید شد ✅"
	msgAskTopUp      = "مبلغ شارژ را به تومان بنویسید:"
	msgAskVolume     = "چند گیگابایت می‌خواهید؟ عدد را بنویسید:"
	msgNoConfigs     = "هنوز کانفیگی نساخته‌اید."
	msgNoLedger      = "هنوز تراکنشی ندارید."
	msgNeedBalance   = "موجودی کیف پول شما برای ساخت کانفیگ کافی نیست. ابتدا کیف پول را شارژ کنید."
	msgShopNoInbound = "فروشگاه هنوز راه‌اندازی نشده است. لطفاً با پشتیبانی تماس بگیرید."
	msgBlocked       = "دسترسی شما به فروشگاه مسدود شده است."
	msgTopUpSent     = "رسید شما دریافت شد ✅\n\nپس از تأیید مدیر، مبلغ به کیف پولتان اضافه می‌شود و همین‌جا خبرتان می‌کنیم."
	msgVolumeBad     = "لطفاً یک عدد درست بنویسید."
	msgNoLinkYet     = "⚠️ کانفیگ ساخته شد ولی لینک آن آماده نشد. لطفاً با پشتیبانی تماس بگیرید — مدیر هم خبردار شد."

	msgPickConfig       = "📱 <b>کانفیگ‌های شما</b>\n\nروی هرکدام بزنید تا اطلاعات و تنظیماتش باز شود:"
	msgConfigGone       = "این کانفیگ دیگر وجود ندارد."
	msgConfigPaused     = "⏸ کانفیگ غیرفعال شد. هر وقت خواستید دوباره فعالش کنید."
	msgConfigResumed    = "▶️ کانفیگ دوباره فعال شد."
	msgConfigNeedsFunds = "کانفیگ از حالت توقف خارج شد، ولی تا شارژ کیف پول روشن نمی‌شود."
	msgAskAddVolume     = "چند گیگابایت به این کانفیگ اضافه شود؟ عدد را بنویسید.\n\n💡 بابت افزودن حجم چیزی کم نمی‌شود؛ هزینه همچنان فقط بابت مصرف واقعی است."
	msgConfirmDelete    = "آیا از حذف کانفیگ <code>%s</code> مطمئن هستید؟ این کار برگشت‌پذیر نیست."
	msgConfigDeleted    = "🗑 کانفیگ حذف شد."

	msgPickUser = "👥 <b>کاربران فروشگاه</b>\n\nروی هرکدام بزنید تا صفحه‌ی مدیریتش باز شود:"
	msgUserGone = "این کاربر پیدا نشد."

	msgAskDiscountCode = "کد تخفیف را بنویسید:"
	msgDiscountApplied = "🏷 کد <code>%s</code> اعمال شد!\n\n" +
		"🎁 تخفیف: <b>%s٪</b> — معادل <b>%s %s</b> هدیه\n" +
		"🧮 پس از تأیید مدیر، مجموع <b>%s %s</b> به کیف پولتان اضافه می‌شود."
	msgDiscountUnknown = "چنین کد تخفیفی وجود ندارد. دوباره امتحان کنید یا «انصراف» را بزنید."
	msgDiscountExpired = "این کد تخفیف منقضی شده است."
	msgDiscountUsedUp  = "ظرفیت این کد تخفیف تمام شده است."
	msgDiscountAlready = "شما قبلاً از این کد استفاده کرده‌اید."

	msgNoDiscounts    = "🏷 هنوز کد تخفیفی نساخته‌اید."
	msgPickDiscount   = "🏷 <b>کدهای تخفیف</b>\n\nروی هرکدام بزنید تا جزئیات و تنظیماتش باز شود:"
	msgDiscountGone   = "این کد تخفیف دیگر وجود ندارد."
	msgAskNewDiscount = "کد جدید را در یک خط بنویسید:\n\n" +
		"<code>کد درصد [تعداد‌مجاز] [روز‌اعتبار]</code>\n\n" +
		"مثال‌ها:\n" +
		"<code>NOWRUZ 20</code> — ۲۰٪ هدیه، بدون محدودیت\n" +
		"<code>VIP 30 50</code> — ۳۰٪ هدیه، فقط ۵۰ بار\n" +
		"<code>HAFTE 15 100 7</code> — ۱۵٪ هدیه، ۱۰۰ بار، تا ۷ روز دیگر\n\n" +
		"💡 هر کاربر از هر کد فقط یک‌بار می‌تواند استفاده کند."
	msgDiscountFormatBad  = "قالب درست نیست. حداقل باید کد و درصد را بنویسید، مثل <code>NOWRUZ 20</code>."
	msgDiscountPercentBad = "درصد باید عددی بین ۱ تا ۱۰۰ باشد."
	msgDiscountExists     = "کدی با همین نام از قبل وجود دارد."
	msgDiscountCreated    = "✅ کد تخفیف ساخته شد."
	msgDiscountDeleted    = "🗑 کد تخفیف حذف شد."
	msgSuspended          = "⛔️ <b>کیف پول شما خالی شد</b>\n\nکانفیگ‌های شما موقتاً غیرفعال شدند. به‌محض شارژ کیف پول، دوباره خودکار فعال می‌شوند."

	msgShopHelp = "<b>راهنما</b>\n\n" +
		"۱️⃣ <b>شارژ کیف پول</b> — مبلغ را می‌نویسید، رسید واریز را می‌فرستید و پس از تأیید مدیر موجودی‌تان اضافه می‌شود.\n\n" +
		"۲️⃣ <b>خرید کانفیگ</b> — حجم دلخواه را می‌گویید و کانفیگ بلافاصله ساخته می‌شود. " +
		"در این لحظه هیچ پولی کم نمی‌شود.\n\n" +
		"۳️⃣ <b>پرداخت به‌ازای مصرف</b> — هر چند دقیقه یک‌بار مصرف شما اندازه‌گیری می‌شود و فقط بهای همان مقدار از کیف پولتان کم می‌شود.\n\n" +
		"۴️⃣ اگر موجودی تمام شود کانفیگ‌ها خاموش می‌شوند و با شارژ دوباره، خودکار روشن می‌شوند."
)

// walletCard is the buyer's balance screen.
func walletCard(balance, paid, spent int64, currency string, configs int) string {
	var b strings.Builder
	b.WriteString("<b>کیف پول شما</b>\n\n")
	fmt.Fprintf(&b, "💰 موجودی: <b>%s %s</b>\n", faNum(balance), esc(currency))
	fmt.Fprintf(&b, "📥 مجموع شارژ: %s %s\n", faNum(paid), esc(currency))
	fmt.Fprintf(&b, "📤 مجموع مصرف: %s %s\n", faNum(spent), esc(currency))
	fmt.Fprintf(&b, "📱 کانفیگ‌های فعال: %s\n", faNum(int64(configs)))
	if balance <= 0 {
		b.WriteString("\n⚠️ موجودی شما تمام شده است؛ کانفیگ‌هایتان تا شارژ بعدی غیرفعال می‌مانند.")
	}
	return b.String()
}

// priceCard tells a buyer exactly what they will be charged for.
func priceCard(perGB, perDay int64, currency string, minTopUp, maxTopUp, minBalance, maxVolume int64) string {
	var b strings.Builder
	b.WriteString("<b>تعرفه</b>\n\n")
	if perGB > 0 {
		fmt.Fprintf(&b, "📶 هر گیگابایت مصرف: <b>%s %s</b>\n", faNum(perGB), esc(currency))
	} else {
		b.WriteString("📶 مصرف ترافیک: <b>رایگان</b>\n")
	}
	if perDay > 0 {
		fmt.Fprintf(&b, "🗓 اجاره روزانه هر کانفیگ: <b>%s %s</b>\n", faNum(perDay), esc(currency))
	}
	b.WriteString("\n")
	if minTopUp > 0 {
		fmt.Fprintf(&b, "➕ حداقل شارژ: %s %s\n", faNum(minTopUp), esc(currency))
	}
	if maxTopUp > 0 {
		fmt.Fprintf(&b, "➕ حداکثر شارژ: %s %s\n", faNum(maxTopUp), esc(currency))
	}
	if minBalance > 0 {
		fmt.Fprintf(&b, "🔒 حداقل موجودی برای ساخت کانفیگ: %s %s\n", faNum(minBalance), esc(currency))
	}
	if maxVolume > 0 {
		fmt.Fprintf(&b, "📦 حداکثر حجم هر کانفیگ: %s گیگابایت\n", faNum(maxVolume))
	}
	b.WriteString("\n💡 هزینه فقط بابت مصرف واقعی کم می‌شود، نه بابت حجمی که انتخاب می‌کنید.")
	return b.String()
}

// configCard is one config as its owner sees it.
func configCard(email string, volumeGB, usedBytes, cost int64, active bool, currency, link string) string {
	var b strings.Builder
	mark := "🟢"
	if !active {
		mark = "⛔️"
	}
	fmt.Fprintf(&b, "%s <b>%s</b>\n", mark, esc(email))
	fmt.Fprintf(&b, "📶 مصرف: <b>%s</b> از %s\n", humanBytes(usedBytes), quotaGB(volumeGB))
	if volumeGB > 0 {
		b.WriteString(progressBar(float64(usedBytes), float64(volumeGB)*bytesPerGB) + "\n")
	}
	fmt.Fprintf(&b, "💳 هزینه تا اینجا: <b>%s %s</b>\n", faNum(cost), esc(currency))
	if strings.TrimSpace(link) != "" {
		fmt.Fprintf(&b, "\n🔗 لینک اشتراک:\n<code>%s</code>", esc(link))
	}
	return b.String()
}

// configDetailCard is one config's own screen — the fuller view behind a name in
// the config list, with the state spelled out rather than left to an icon.
func configDetailCard(cfg *model.BotConfig, usedBytes, cost int64, currency string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "📱 <b>%s</b>\n\n", esc(cfg.Email))

	switch {
	case cfg.Paused:
		b.WriteString("⏸ وضعیت: <b>غیرفعال شده توسط شما</b>\n")
	case cfg.Active:
		b.WriteString("🟢 وضعیت: <b>فعال</b>\n")
	default:
		b.WriteString("⛔️ وضعیت: <b>خاموش — کیف پول خالی است</b>\n")
	}

	fmt.Fprintf(&b, "📦 حجم: <b>%s</b>\n", quotaGB(cfg.VolumeGB))
	fmt.Fprintf(&b, "📶 مصرف: <b>%s</b>\n", humanBytes(usedBytes))
	if cfg.VolumeGB > 0 {
		b.WriteString(progressBar(float64(usedBytes), float64(cfg.VolumeGB)*bytesPerGB) + "\n")
		if remaining := cfg.VolumeGB*bytesPerGB - usedBytes; remaining > 0 {
			fmt.Fprintf(&b, "🔋 باقی‌مانده: <b>%s</b>\n", humanBytes(remaining))
		} else {
			b.WriteString("🔋 باقی‌مانده: <b>تمام شده</b>\n")
		}
	}
	fmt.Fprintf(&b, "\n💳 هزینه تا اینجا: <b>%s %s</b>\n", faNum(cost), esc(currency))
	if cfg.CreatedAt > 0 {
		fmt.Fprintf(&b, "🗓 ساخته شده: %s\n", faDate(cfg.CreatedAt))
	}
	return b.String()
}

// faDate renders a millisecond timestamp as a Persian-digit date.
func faDate(ms int64) string {
	return toPersianDigits(time.UnixMilli(ms).Format("2006-01-02"))
}

// userDetailCard is one shop user's own screen for an admin.
func userDetailCard(u *model.BotUser, configs int, pendingTopUps int64, currency string) string {
	var b strings.Builder
	name := strings.TrimSpace(u.FirstName)
	if u.Username != "" {
		name = strings.TrimSpace(name + " @" + u.Username)
	}
	if name == "" {
		name = "بدون نام"
	}
	fmt.Fprintf(&b, "👤 <b>%s</b>\n🆔 <code>%d</code>\n\n", esc(name), u.TelegramId)
	if u.Blocked {
		b.WriteString("⛔️ وضعیت: <b>مسدود</b>\n")
	} else {
		b.WriteString("🟢 وضعیت: <b>آزاد</b>\n")
	}
	fmt.Fprintf(&b, "💰 موجودی: <b>%s %s</b>\n", faNum(u.Balance), esc(currency))
	fmt.Fprintf(&b, "📥 مجموع شارژ: %s %s\n", faNum(u.TotalPaid), esc(currency))
	fmt.Fprintf(&b, "📤 مجموع مصرف: %s %s\n", faNum(u.TotalSpent), esc(currency))
	fmt.Fprintf(&b, "📱 کانفیگ‌ها: %s\n", faNum(int64(configs)))
	if pendingTopUps > 0 {
		fmt.Fprintf(&b, "🔎 شارژ در انتظار تأیید: <b>%s</b>\n", faNum(pendingTopUps))
	}
	if u.Balance <= 0 {
		b.WriteString("\n⚠️ موجودی این کاربر تمام شده و کانفیگ‌هایش خاموش‌اند.")
	}
	return b.String()
}

// discountCard is one code's own screen for an admin.
func discountCard(c *model.DiscountCode, currency string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "🏷 <b>%s</b>\n\n", esc(c.Code))
	fmt.Fprintf(&b, "🎁 هدیه: <b>%s٪</b> از مبلغ شارژ\n", faNum(int64(c.Percent)))
	if c.MaxBonus > 0 {
		fmt.Fprintf(&b, "🔒 سقف هدیه هر بار: %s %s\n", faNum(c.MaxBonus), esc(currency))
	}
	if c.MaxUses > 0 {
		fmt.Fprintf(&b, "🔢 استفاده: <b>%s</b> از %s بار\n", faNum(int64(c.Used)), faNum(int64(c.MaxUses)))
	} else {
		fmt.Fprintf(&b, "🔢 استفاده: <b>%s</b> بار (بدون محدودیت)\n", faNum(int64(c.Used)))
	}
	if c.ExpiresAt > 0 {
		fmt.Fprintf(&b, "⌛️ اعتبار تا: %s\n", faDate(c.ExpiresAt))
	} else {
		b.WriteString("⌛️ اعتبار: بدون تاریخ انقضا\n")
	}

	switch {
	case !c.Enabled:
		b.WriteString("\n⛔️ این کد غیرفعال است.")
	case c.ExpiresAt > 0 && nowMilli() > c.ExpiresAt:
		b.WriteString("\n⌛️ این کد منقضی شده است.")
	case c.MaxUses > 0 && c.Used >= c.MaxUses:
		b.WriteString("\n🔚 ظرفیت این کد تمام شده است.")
	default:
		b.WriteString("\n✅ این کد هم‌اکنون قابل استفاده است.")
	}
	return b.String()
}

// topUpInstructions is the screen between naming an amount and sending a receipt.
func topUpInstructions(id int, amount int64, currency, payText, code string, bonus int64) string {
	var b strings.Builder
	fmt.Fprintf(&b, "💳 درخواست شارژ شماره <b>%s</b> ثبت شد.\n\n", faNum(int64(id)))
	fmt.Fprintf(&b, "💰 مبلغ واریزی: <b>%s %s</b>\n", faNum(amount), esc(currency))
	if strings.TrimSpace(code) != "" && bonus > 0 {
		fmt.Fprintf(&b, "🏷 کد <code>%s</code>: <b>%s %s</b> هدیه\n", esc(code), faNum(bonus), esc(currency))
		fmt.Fprintf(&b, "🧮 مجموع افزوده به کیف پول: <b>%s %s</b>\n", faNum(amount+bonus), esc(currency))
	}
	b.WriteString("\n")
	if strings.TrimSpace(payText) != "" {
		b.WriteString(esc(payText))
		b.WriteString("\n\n")
	} else {
		b.WriteString("اطلاعات پرداخت هنوز توسط مدیر ثبت نشده است؛ لطفاً با پشتیبانی تماس بگیرید.\n\n")
	}
	b.WriteString("پس از واریز، تصویر رسید را همین‌جا ارسال کنید 📸")
	return b.String()
}

// txLine renders one ledger entry.
func txLine(amount, balance int64, kind, details, currency string) string {
	sign := "➕"
	if amount < 0 {
		sign = "➖"
		amount = -amount
	}
	label := map[string]string{
		"topup":  "شارژ کیف پول",
		"usage":  "مصرف ترافیک",
		"rent":   "اجاره روزانه",
		"adjust": "اصلاح توسط مدیر",
		"refund": "بازگشت وجه",
		"bonus":  "هدیه کد تخفیف",
	}[kind]
	if label == "" {
		label = kind
	}
	line := fmt.Sprintf("%s <b>%s %s</b> — %s", sign, faNum(amount), esc(currency), label)
	if strings.TrimSpace(details) != "" {
		line += fmt.Sprintf("\n   <i>%s</i>", esc(details))
	}
	line += fmt.Sprintf("\n   موجودی پس از آن: %s %s", faNum(balance), esc(currency))
	return line
}

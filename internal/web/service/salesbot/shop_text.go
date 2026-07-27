package salesbot

import (
	"fmt"
	"strings"
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
	msgSuspended     = "⛔️ <b>کیف پول شما خالی شد</b>\n\nکانفیگ‌های شما موقتاً غیرفعال شدند. به‌محض شارژ کیف پول، دوباره خودکار فعال می‌شوند."

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

// topUpInstructions is the screen between naming an amount and sending a receipt.
func topUpInstructions(id int, amount int64, currency, payText string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "💳 درخواست شارژ شماره <b>%s</b> ثبت شد.\n\n", faNum(int64(id)))
	fmt.Fprintf(&b, "💰 مبلغ: <b>%s %s</b>\n\n", faNum(amount), esc(currency))
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

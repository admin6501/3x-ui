package salesbot

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

// Conversation steps. Each is a single question the bot is waiting an answer
// to; the reply advances to the next one.
const (
	stepAwaitReceipt = "await_receipt"
	stepRejectNote   = "reject_note"
	stepBroadcast    = "broadcast"
	stepPkgName      = "pkg_name"
	stepPkgDesc      = "pkg_desc"
	stepPkgPrice     = "pkg_price"
	stepPkgTraffic   = "pkg_traffic"
	stepPkgClients   = "pkg_clients"
	stepPkgDuration  = "pkg_duration"
	stepPkgInbounds  = "pkg_inbounds"
)

// service errors re-exported so customer.go can match on them without importing
// the service package for one symbol.
var service_ErrNoAccountToRenew = service.ErrNoAccountToRenew

// adminService is the shared instance the bot uses for reseller stats and
// enable/disable actions.
func (b *Bot) adminService() *service.AdminService {
	return &service.AdminService{}
}

func (b *Bot) adminMenu() telego.ReplyMarkup {
	return tu.Keyboard(
		[]telego.KeyboardButton{tu.KeyboardButton(btnAdminOrders), tu.KeyboardButton(btnAdminPackages)},
		[]telego.KeyboardButton{tu.KeyboardButton(btnAdminResellers), tu.KeyboardButton(btnAdminStats)},
		[]telego.KeyboardButton{tu.KeyboardButton(btnAdminBroadcast)},
		[]telego.KeyboardButton{tu.KeyboardButton(btnAdminExit)},
	).WithResizeKeyboard()
}

func (b *Bot) onAdminMenu(msg telego.Message) {
	chatId := msg.Chat.ID
	if !b.isAdmin(chatId) {
		b.send(chatId, msgNotAdmin, b.mainMenu(chatId))
		return
	}
	b.states.clear(chatId)
	pending := b.salesService.CountPendingReview()
	text := msgAdminWelcome
	if pending > 0 {
		text += fmt.Sprintf("\n\n🔔 <b>%s سفارش</b> در انتظار بررسی است.", faNum(pending))
	}
	b.send(chatId, text, b.adminMenu())
}

func (b *Bot) onAdminButton(msg telego.Message) {
	chatId := msg.Chat.ID
	if !b.isAdmin(chatId) {
		b.send(chatId, msgNotAdmin, b.mainMenu(chatId))
		return
	}
	switch strings.TrimSpace(msg.Text) {
	case btnAdminOrders:
		b.showPendingOrders(chatId)
	case btnAdminPackages:
		b.showPackageAdmin(chatId)
	case btnAdminResellers:
		b.showResellers(chatId)
	case btnAdminStats:
		b.showStats(chatId)
	case btnAdminBroadcast:
		b.states.set(chatId, &state{step: stepBroadcast})
		b.send(chatId, msgAskBroadcast, cancelKeyboard())
	case btnAdminExit:
		b.states.clear(chatId)
		b.send(chatId, msgLeftAdmin, b.mainMenu(chatId))
	}
}

func cancelKeyboard() telego.ReplyMarkup {
	return tu.Keyboard([]telego.KeyboardButton{tu.KeyboardButton(btnCancel)}).WithResizeKeyboard()
}

// ------------------------------------------------------------- orders --

func (b *Bot) showPendingOrders(chatId int64) {
	orders, err := b.salesService.ListOrders(model.OrderReview, 20)
	if err != nil || len(orders) == 0 {
		b.send(chatId, msgNoPendingOrders, b.adminMenu())
		return
	}
	currency, _ := b.settingService.GetSalesBotCurrency()
	for _, o := range orders {
		caption := fmt.Sprintf(
			"🧾 <b>سفارش #%s</b>\n👤 %s\n🆔 <code>%d</code>\n📦 %s (%s)\n💰 %s %s",
			faNum(int64(o.Id)), esc(o.TelegramName), o.TelegramId,
			esc(o.PackageName), kindLabel(o.Kind), faNum(o.Price), esc(currency),
		)
		kb := tu.InlineKeyboard(tu.InlineKeyboardRow(
			tu.InlineKeyboardButton("✅ تأیید").WithCallbackData(fmt.Sprintf("ok:%d", o.Id)),
			tu.InlineKeyboardButton("❌ رد").WithCallbackData(fmt.Sprintf("no:%d", o.Id)),
		))
		if o.ReceiptFileId != "" {
			b.sendPhoto(chatId, o.ReceiptFileId, caption, kb)
			continue
		}
		b.send(chatId, caption, kb)
	}
}

// approveOrder is the moment money becomes access: it creates or tops up the
// buyer's reseller account and sends them their credentials.
func (b *Bot) approveOrder(adminId int64, orderId int) {
	result, err := b.salesService.ApproveOrder(nil, orderId, true, b.inboundService, b.xrayService)
	if err != nil {
		switch err {
		case service.ErrOrderNotPending:
			b.send(adminId, msgAlreadyDecided)
		case service.ErrNoReceipt:
			b.send(adminId, msgNoReceiptYet)
		case service.ErrOrderNotFound:
			b.send(adminId, msgOrderGone)
		default:
			b.send(adminId, msgSomethingWrong+"\n<code>"+esc(err.Error())+"</code>")
		}
		return
	}

	panelUrl, _ := b.settingService.GetSalesBotPanelUrl()
	buyer := result.Order.TelegramId
	if result.IsNew {
		b.send(buyer, credentialsMessage(panelUrl, result.Username, result.Password, result.TrafficGB, result.ClientQuota), b.mainMenu(buyer))
	} else {
		b.send(buyer, topUpMessage(result.Username, result.TrafficGB, result.ClientQuota), b.mainMenu(buyer))
	}
	b.send(adminId, fmt.Sprintf("✅ سفارش #%s تأیید شد و به خریدار اطلاع داده شد.\n👤 حساب: <code>%s</code>",
		faNum(int64(orderId)), esc(result.Username)))
}

func (b *Bot) rejectOrder(adminId int64, orderId int, note string) {
	order, err := b.salesService.RejectOrder(orderId, note)
	if err != nil {
		b.send(adminId, msgAlreadyDecided)
		return
	}
	text := fmt.Sprintf("❌ سفارش شماره <b>%s</b> تأیید نشد.", faNum(int64(order.Id)))
	if strings.TrimSpace(note) != "" {
		text += "\n📝 دلیل: " + esc(note)
	}
	text += "\n\nدر صورت نیاز با پشتیبانی در تماس باشید."
	b.send(order.TelegramId, text, b.mainMenu(order.TelegramId))
	b.send(adminId, fmt.Sprintf("سفارش #%s رد شد.", faNum(int64(orderId))), b.adminMenu())
}

// ----------------------------------------------------------- packages --

func (b *Bot) showPackageAdmin(chatId int64) {
	packages, err := b.salesService.ListPackages()
	currency, _ := b.settingService.GetSalesBotCurrency()
	addRow := tu.InlineKeyboardRow(
		tu.InlineKeyboardButton("➕ پکیج جدید").WithCallbackData("pkgnew"),
	)
	if err != nil || len(packages) == 0 {
		b.send(chatId, "هنوز پکیجی تعریف نشده است.", tu.InlineKeyboard(addRow))
		return
	}
	var rows [][]telego.InlineKeyboardButton
	var body strings.Builder
	body.WriteString("<b>پکیج‌های تعریف‌شده</b>\n\n")
	for _, p := range packages {
		mark := "🟢"
		if !p.Enable {
			mark = "🔴"
		}
		fmt.Fprintf(&body, "%s <b>%s</b> — %s %s\n📶 %s | 👥 %s\n\n",
			mark, esc(p.Name), faNum(p.Price), esc(currency),
			quotaGB(p.TrafficGB), quotaCount(p.ClientQuota))
		toggle := "خاموش"
		if !p.Enable {
			toggle = "روشن"
		}
		rows = append(rows, tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(fmt.Sprintf("%s %s", mark, p.Name)).WithCallbackData(fmt.Sprintf("pkgtoggle:%d", p.Id)),
			tu.InlineKeyboardButton(toggle).WithCallbackData(fmt.Sprintf("pkgtoggle:%d", p.Id)),
			tu.InlineKeyboardButton("🗑").WithCallbackData(fmt.Sprintf("pkgdel:%d", p.Id)),
		))
	}
	rows = append(rows, addRow)
	b.send(chatId, body.String(), tu.InlineKeyboard(rows...))
}

// startPackageWizard walks an admin through creating a package one question at
// a time, which is the only workable way to build a multi-field record in chat.
func (b *Bot) startPackageWizard(chatId int64) {
	b.states.set(chatId, &state{step: stepPkgName})
	b.send(chatId, "نام پکیج را بنویسید (مثلاً «نمایندگی برنزی»):", cancelKeyboard())
}

// ---------------------------------------------------------- resellers --

func (b *Bot) showResellers(chatId int64) {
	accounts, err := b.salesService.ListSoldAccounts()
	if err != nil || len(accounts) == 0 {
		b.send(chatId, "هنوز هیچ پنل نمایندگی‌ای از طریق ربات فروخته نشده است.", b.adminMenu())
		return
	}
	adminSvc := b.adminService()
	var rows [][]telego.InlineKeyboardButton
	var body strings.Builder
	body.WriteString("<b>نمایندگان</b>\n\n")
	for i := range accounts {
		u := &accounts[i]
		stats := adminSvc.GetResellerStats(u)
		mark := "🟢"
		if u.Disabled {
			mark = "⛔️"
		}
		fmt.Fprintf(&body, "%s <b>%s</b> (🆔 <code>%d</code>)\n📶 %s از %s | 👥 %s از %s\n\n",
			mark, esc(u.Username), u.TelegramId,
			humanBytes(stats.TrafficUsedBytes), quotaGB(stats.TrafficQuotaGB),
			faNum(int64(stats.CurrentClients)), quotaCount(stats.ClientQuota))
		action := "⛔️ غیرفعال"
		if u.Disabled {
			action = "✅ فعال"
		}
		rows = append(rows, tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(u.Username).WithCallbackData(fmt.Sprintf("rsoff:%d", u.Id)),
			tu.InlineKeyboardButton(action).WithCallbackData(fmt.Sprintf("rsoff:%d", u.Id)),
			tu.InlineKeyboardButton("♻️ ریست").WithCallbackData(fmt.Sprintf("rsreset:%d", u.Id)),
		))
	}
	b.send(chatId, body.String(), tu.InlineKeyboard(rows...))
}

func (b *Bot) showStats(chatId int64) {
	stats := b.salesService.Stats()
	currency, _ := b.settingService.GetSalesBotCurrency()
	var body strings.Builder
	body.WriteString("<b>آمار فروش</b>\n\n")
	fmt.Fprintf(&body, "💰 درآمد کل: <b>%s %s</b>\n", faNum(stats.Revenue), esc(currency))
	fmt.Fprintf(&body, "✅ سفارش‌های تأییدشده: <b>%s</b>\n", faNum(stats.ApprovedOrders))
	fmt.Fprintf(&body, "🔎 در انتظار بررسی: <b>%s</b>\n", faNum(stats.PendingReview))
	fmt.Fprintf(&body, "❌ ردشده: <b>%s</b>\n", faNum(stats.RejectedOrders))
	fmt.Fprintf(&body, "🧑‍🤝‍🧑 خریداران: <b>%s</b>\n", faNum(stats.Buyers))
	fmt.Fprintf(&body, "👥 نمایندگان فعال در پنل: <b>%s</b>", faNum(stats.Resellers))
	b.send(chatId, body.String(), b.adminMenu())
}

func (b *Bot) broadcast(adminId int64, text string) {
	ids := b.salesService.BuyerIds()
	sent := 0
	for _, id := range ids {
		if id == 0 {
			continue
		}
		b.send(id, "📢 <b>اطلاعیه</b>\n\n"+esc(text))
		sent++
	}
	b.send(adminId, fmt.Sprintf("پیام برای <b>%s</b> نفر ارسال شد.", faNum(int64(sent))), b.adminMenu())
}

// --------------------------------------------------- admin callbacks --

func (b *Bot) onAdminCallback(q telego.CallbackQuery) {
	chatId := q.From.ID
	if !b.isAdmin(chatId) {
		b.answer(q.ID, msgNotAdmin)
		return
	}
	parts := strings.Split(q.Data, ":")
	action := parts[0]
	arg := 0
	if len(parts) > 1 {
		arg, _ = strconv.Atoi(parts[1])
	}

	switch action {
	case "ok":
		b.answer(q.ID, "")
		b.approveOrder(chatId, arg)
	case "no":
		b.answer(q.ID, "")
		b.states.set(chatId, &state{step: stepRejectNote, orderId: arg})
		b.send(chatId, msgAskRejectNote, tu.Keyboard(
			[]telego.KeyboardButton{tu.KeyboardButton(btnSkip), tu.KeyboardButton(btnCancel)},
		).WithResizeKeyboard())
	case "pkgnew":
		b.answer(q.ID, "")
		b.startPackageWizard(chatId)
	case "pkgtoggle":
		pkg, err := b.salesService.GetPackage(arg)
		if err != nil {
			b.answer(q.ID, msgSomethingWrong)
			return
		}
		if err := b.salesService.SetPackageEnabled(arg, !pkg.Enable); err != nil {
			b.answer(q.ID, msgSomethingWrong)
			return
		}
		b.answer(q.ID, "انجام شد")
		b.showPackageAdmin(chatId)
	case "pkgdel":
		if err := b.salesService.DeletePackage(arg); err != nil {
			b.answer(q.ID, msgSomethingWrong)
			return
		}
		b.answer(q.ID, "حذف شد")
		b.showPackageAdmin(chatId)
	case "rsoff":
		account, err := b.salesService.AccountByID(arg)
		if err != nil {
			b.answer(q.ID, msgSomethingWrong)
			return
		}
		if err := b.adminService().SetAdminEnabled(nil, arg, account.Disabled, b.inboundService, b.xrayService); err != nil {
			b.answer(q.ID, msgSomethingWrong)
			return
		}
		b.answer(q.ID, "انجام شد")
		b.showResellers(chatId)
	case "rsreset":
		if err := b.adminService().ResetResellerTraffic(nil, arg, b.inboundService, b.xrayService); err != nil {
			b.answer(q.ID, msgSomethingWrong)
			return
		}
		b.answer(q.ID, "ترافیک صفر شد")
		b.showResellers(chatId)
	default:
		b.answer(q.ID, "")
	}
}

// ------------------------------------------------ conversation stepping --

// handleConversation feeds a plain message into whichever step the chat is on.
// Returns true when the message was consumed by a flow.
func (b *Bot) handleConversation(msg telego.Message, st *state) bool {
	chatId := msg.Chat.ID
	text := strings.TrimSpace(msg.Text)

	if text == btnCancel {
		b.states.clear(chatId)
		menu := b.mainMenu(chatId)
		if b.isAdmin(chatId) {
			menu = b.adminMenu()
		}
		b.send(chatId, msgCancelled, menu)
		return true
	}

	switch st.step {
	case stepAwaitReceipt:
		// Only a photo advances this step; a stray message gets a nudge.
		b.send(chatId, msgReceiptOnlyPic)
		return true

	case stepRejectNote:
		note := text
		if text == btnSkip {
			note = ""
		}
		b.states.clear(chatId)
		b.rejectOrder(chatId, st.orderId, note)
		return true

	case stepBroadcast:
		b.states.clear(chatId)
		b.broadcast(chatId, text)
		return true

	case stepPkgName:
		st.pkg.Name = text
		st.step = stepPkgDesc
		b.states.set(chatId, st)
		b.send(chatId, "توضیح کوتاه پکیج را بنویسید (یا «⏭ رد کردن»):", skipKeyboard())
		return true

	case stepPkgDesc:
		if text != btnSkip {
			st.pkg.Description = text
		}
		st.step = stepPkgPrice
		b.states.set(chatId, st)
		b.send(chatId, "قیمت پکیج را به عدد بنویسید (مثلاً 500000):", cancelKeyboard())
		return true

	case stepPkgPrice:
		value, ok := parseNumber(text)
		if !ok {
			b.send(chatId, "لطفاً فقط عدد بنویسید.")
			return true
		}
		st.pkg.Price = value
		st.step = stepPkgTraffic
		b.states.set(chatId, st)
		b.send(chatId, "سهمیه ترافیک به گیگابایت (۰ = نامحدود):", cancelKeyboard())
		return true

	case stepPkgTraffic:
		value, ok := parseNumber(text)
		if !ok {
			b.send(chatId, "لطفاً فقط عدد بنویسید.")
			return true
		}
		st.pkg.TrafficGB = value
		st.step = stepPkgClients
		b.states.set(chatId, st)
		b.send(chatId, "سقف تعداد کاربر (۰ = نامحدود):", cancelKeyboard())
		return true

	case stepPkgClients:
		value, ok := parseNumber(text)
		if !ok {
			b.send(chatId, "لطفاً فقط عدد بنویسید.")
			return true
		}
		st.pkg.ClientQuota = int(value)
		st.step = stepPkgDuration
		b.states.set(chatId, st)
		b.send(chatId, "مدت اعتبار به روز (۰ = بدون محدودیت زمانی):", cancelKeyboard())
		return true

	case stepPkgDuration:
		value, ok := parseNumber(text)
		if !ok {
			b.send(chatId, "لطفاً فقط عدد بنویسید.")
			return true
		}
		st.pkg.DurationDays = int(value)
		st.step = stepPkgInbounds
		b.states.set(chatId, st)
		b.send(chatId, "شناسه‌ی ورودی‌هایی که به نماینده داده می‌شود را با کاما بنویسید (مثلاً 1,3,5) یا «⏭ رد کردن»:\n\n"+
			b.inboundHint(), skipKeyboard())
		return true

	case stepPkgInbounds:
		if text != btnSkip {
			st.pkg.Inbounds = text
		}
		b.states.clear(chatId)
		b.finishPackageWizard(chatId, st.pkg)
		return true
	}
	return false
}

func skipKeyboard() telego.ReplyMarkup {
	return tu.Keyboard(
		[]telego.KeyboardButton{tu.KeyboardButton(btnSkip), tu.KeyboardButton(btnCancel)},
	).WithResizeKeyboard()
}

// inboundHint lists the panel's inbounds so the admin does not have to leave
// Telegram to look their ids up.
func (b *Bot) inboundHint() string {
	if b.inboundService == nil {
		return ""
	}
	inbounds, err := b.inboundService.GetInbounds(0)
	if err != nil || len(inbounds) == 0 {
		return ""
	}
	var out strings.Builder
	out.WriteString("<b>ورودی‌های موجود:</b>\n")
	for i, ib := range inbounds {
		if i >= 25 {
			out.WriteString("…\n")
			break
		}
		remark := ib.Remark
		if strings.TrimSpace(remark) == "" {
			remark = string(ib.Protocol)
		}
		fmt.Fprintf(&out, "<code>%d</code> — %s (:%d)\n", ib.Id, esc(remark), ib.Port)
	}
	return out.String()
}

func (b *Bot) finishPackageWizard(chatId int64, draft pkgDraft) {
	pkg := &model.ResellerPackage{
		Name:            draft.Name,
		Description:     draft.Description,
		Price:           draft.Price,
		TrafficGB:       draft.TrafficGB,
		ClientQuota:     draft.ClientQuota,
		DurationDays:    draft.DurationDays,
		AllowedInbounds: draft.Inbounds,
		Enable:          true,
	}
	if err := b.salesService.SavePackage(pkg); err != nil {
		b.send(chatId, msgSomethingWrong+"\n<code>"+esc(err.Error())+"</code>", b.adminMenu())
		return
	}
	currency, _ := b.settingService.GetSalesBotCurrency()
	b.send(chatId, "✅ پکیج ساخته شد:\n\n"+
		packageCard(pkg.Name, pkg.Description, pkg.Price, currency, pkg.TrafficGB, pkg.ClientQuota, pkg.DurationDays),
		b.adminMenu())
}

// parseNumber accepts Persian and Arabic-Indic digits as well as ASCII, and
// tolerates the separators people type into prices.
func parseNumber(s string) (int64, bool) {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= '۰' && r <= '۹':
			b.WriteRune('0' + (r - '۰'))
		case r >= '٠' && r <= '٩':
			b.WriteRune('0' + (r - '٠'))
		case r == ',' || r == '٬' || r == '،' || r == ' ' || r == '_':
			// thousands separators — ignore
		default:
			return 0, false
		}
	}
	if b.Len() == 0 {
		return 0, false
	}
	value, err := strconv.ParseInt(b.String(), 10, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

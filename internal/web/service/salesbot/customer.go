package salesbot

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
)

// registerHandlers wires the whole bot: commands, reply-keyboard buttons,
// inline callbacks and the free-text steps of the multi-step flows.
func (b *Bot) registerHandlers(h *th.BotHandler) {
	h.HandleMessage(func(ctx *th.Context, msg telego.Message) error {
		go b.guard(msg.Chat.ID, func() { b.onStart(msg) })
		return nil
	}, th.CommandEqual("start"))

	h.HandleMessage(func(ctx *th.Context, msg telego.Message) error {
		go b.guard(msg.Chat.ID, func() { b.onAdminMenu(msg) })
		return nil
	}, th.CommandEqual("admin"))

	h.HandleMessage(func(ctx *th.Context, msg telego.Message) error {
		go b.guard(msg.Chat.ID, func() { b.send(msg.Chat.ID, b.idCard(msg)) })
		return nil
	}, th.CommandEqual("id"))

	h.HandleCallbackQuery(func(ctx *th.Context, q telego.CallbackQuery) error {
		go b.guard(q.From.ID, func() { b.onCallback(q) })
		return nil
	}, th.AnyCallbackQueryWithMessage())

	// Everything else: reply-keyboard buttons, receipt photos and the free-text
	// steps of whichever conversation this chat is in.
	h.HandleMessage(func(ctx *th.Context, msg telego.Message) error {
		go b.guard(msg.Chat.ID, func() { b.onMessage(msg) })
		return nil
	}, th.AnyMessage())
}

// guard keeps a panic in one handler from taking the whole bot down with it.
func (b *Bot) guard(chatId int64, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			logger.Warning("sales bot: handler panicked:", r)
			b.send(chatId, msgSomethingWrong)
		}
	}()
	fn()
}

// idCard answers /id — buyers need their numeric id to be added as an admin,
// and it is the first thing a shop owner looks for when setting the bot up.
func (b *Bot) idCard(msg telego.Message) string {
	return fmt.Sprintf("🆔 شناسه عددی شما: <code>%d</code>", msg.Chat.ID)
}

// ------------------------------------------------------------ main menu --

func (b *Bot) mainMenu(chatId int64) telego.ReplyMarkup {
	rows := [][]telego.KeyboardButton{
		{tu.KeyboardButton(btnBuy)},
		{tu.KeyboardButton(btnMyPanel), tu.KeyboardButton(btnTopUp)},
		{tu.KeyboardButton(btnOrders), tu.KeyboardButton(btnSupport)},
		{tu.KeyboardButton(btnHelp)},
	}
	if b.isAdmin(chatId) {
		rows = append(rows, []telego.KeyboardButton{tu.KeyboardButton(btnAdmin)})
	}
	return tu.Keyboard(rows...).WithResizeKeyboard()
}

func (b *Bot) onStart(msg telego.Message) {
	b.states.clear(msg.Chat.ID)
	welcome, _ := b.settingService.GetSalesBotWelcome()
	if strings.TrimSpace(welcome) == "" {
		welcome = msgDefaultWelcome
	} else {
		welcome = esc(welcome)
	}
	b.send(msg.Chat.ID, welcome, b.mainMenu(msg.Chat.ID))
}

// onMessage routes a plain message: first the conversation the chat is in,
// then the reply-keyboard buttons.
func (b *Bot) onMessage(msg telego.Message) {
	chatId := msg.Chat.ID

	// A receipt is a photo, and only means anything mid-order.
	if len(msg.Photo) > 0 {
		b.onReceiptPhoto(msg)
		return
	}

	if st, ok := b.states.get(chatId); ok {
		if b.handleConversation(msg, st) {
			return
		}
	}

	switch strings.TrimSpace(msg.Text) {
	case btnBuy:
		b.showPackages(chatId, model.OrderKindNew)
	case btnTopUp:
		b.showPackages(chatId, model.OrderKindRenew)
	case btnMyPanel:
		b.showMyPanel(chatId)
	case btnOrders:
		b.showMyOrders(chatId)
	case btnSupport:
		b.showSupport(chatId)
	case btnHelp:
		b.send(chatId, msgHelp, b.mainMenu(chatId))
	case btnMainMenu, btnBack:
		b.states.clear(chatId)
		b.send(chatId, "منوی اصلی 🏠", b.mainMenu(chatId))
	case btnCancel:
		b.states.clear(chatId)
		b.send(chatId, msgCancelled, b.mainMenu(chatId))
	case btnAdmin:
		b.onAdminMenu(msg)
	case btnAdminOrders, btnAdminPackages, btnAdminResellers, btnAdminStats, btnAdminBroadcast, btnAdminExit:
		b.onAdminButton(msg)
	default:
		// Anything unrecognised: show the menu rather than silence.
		b.send(chatId, msgHelp, b.mainMenu(chatId))
	}
}

// ------------------------------------------------------------- buying --

func (b *Bot) showPackages(chatId int64, kind string) {
	if kind == model.OrderKindRenew {
		if _, err := b.salesService.AccountOf(chatId); err != nil {
			b.send(chatId, msgNoAccount, b.mainMenu(chatId))
			return
		}
	}
	packages, err := b.salesService.ListPackagesForSale()
	if err != nil || len(packages) == 0 {
		b.send(chatId, msgNoPackages, b.mainMenu(chatId))
		return
	}
	currency, _ := b.settingService.GetSalesBotCurrency()

	var rows [][]telego.InlineKeyboardButton
	var body strings.Builder
	body.WriteString(msgChoosePackage)
	body.WriteString("\n\n")
	for _, p := range packages {
		body.WriteString(packageCard(p.Name, p.Description, p.Price, currency, p.TrafficGB, p.ClientQuota, p.DurationDays))
		body.WriteString("\n\n➖➖➖\n\n")
		label := fmt.Sprintf("%s — %s %s", p.Name, faNum(p.Price), currency)
		rows = append(rows, tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(label).WithCallbackData(fmt.Sprintf("buy:%s:%d", kind, p.Id)),
		))
	}
	b.send(chatId, strings.TrimRight(body.String(), "\n➖ "), tu.InlineKeyboard(rows...))
}

func (b *Bot) startOrder(chatId int64, name string, packageId int, kind string) {
	order, err := b.salesService.CreateOrder(chatId, name, packageId, kind)
	if err != nil {
		switch err {
		case service_ErrNoAccountToRenew:
			b.send(chatId, msgNoAccount, b.mainMenu(chatId))
		default:
			b.send(chatId, msgSomethingWrong, b.mainMenu(chatId))
		}
		return
	}
	currency, _ := b.settingService.GetSalesBotCurrency()
	payText, _ := b.settingService.GetSalesBotPayText()

	b.states.set(chatId, &state{step: stepAwaitReceipt, orderId: order.Id})
	cancelKb := tu.InlineKeyboard(tu.InlineKeyboardRow(
		tu.InlineKeyboardButton(btnCancel).WithCallbackData(fmt.Sprintf("cancel:%d", order.Id)),
	))
	b.send(chatId, payInstructions(order.Id, order.Price, currency, payText), cancelKb)
}

// onReceiptPhoto accepts the payment proof and hands the order to the admins.
func (b *Bot) onReceiptPhoto(msg telego.Message) {
	chatId := msg.Chat.ID
	st, ok := b.states.get(chatId)
	if !ok || st.step != stepAwaitReceipt {
		b.send(chatId, "برای ارسال رسید، ابتدا یک پکیج را انتخاب کنید.", b.mainMenu(chatId))
		return
	}
	// The last photo in the array is the highest resolution Telegram kept.
	fileId := msg.Photo[len(msg.Photo)-1].FileID
	order, err := b.salesService.AttachReceipt(st.orderId, fileId)
	if err != nil {
		b.send(chatId, msgOrderGone, b.mainMenu(chatId))
		b.states.clear(chatId)
		return
	}
	b.states.clear(chatId)
	b.send(chatId, msgReceiptGot, b.mainMenu(chatId))
	b.pushOrderToAdmins(order, msg.From)
}

// pushOrderToAdmins forwards the receipt with approve/reject buttons.
func (b *Bot) pushOrderToAdmins(order *model.ResellerOrder, from *telego.User) {
	currency, _ := b.settingService.GetSalesBotCurrency()
	who := order.TelegramName
	if from != nil {
		who = strings.TrimSpace(from.FirstName + " " + from.LastName)
		if from.Username != "" {
			who += " (@" + from.Username + ")"
		}
	}
	caption := fmt.Sprintf(
		"🧾 <b>سفارش جدید #%s</b>\n\n👤 خریدار: %s\n🆔 <code>%d</code>\n📦 پکیج: %s\n🔁 نوع: %s\n💰 مبلغ: %s %s",
		faNum(int64(order.Id)), esc(who), order.TelegramId, esc(order.PackageName),
		kindLabel(order.Kind), faNum(order.Price), esc(currency),
	)
	kb := tu.InlineKeyboard(tu.InlineKeyboardRow(
		tu.InlineKeyboardButton("✅ تأیید").WithCallbackData(fmt.Sprintf("ok:%d", order.Id)),
		tu.InlineKeyboardButton("❌ رد").WithCallbackData(fmt.Sprintf("no:%d", order.Id)),
	))
	for _, adminId := range b.admins() {
		if order.ReceiptFileId != "" {
			b.sendPhoto(adminId, order.ReceiptFileId, caption, kb)
			continue
		}
		b.send(adminId, caption, kb)
	}
}

// --------------------------------------------------------- buyer screens --

func (b *Bot) showMyPanel(chatId int64) {
	account, err := b.salesService.AccountOf(chatId)
	if err != nil {
		b.send(chatId, msgNoAccount, b.mainMenu(chatId))
		return
	}
	panelUrl, _ := b.settingService.GetSalesBotPanelUrl()
	stats := b.adminService().GetResellerStats(account)

	var body strings.Builder
	body.WriteString("<b>پنل نمایندگی شما</b>\n\n")
	if strings.TrimSpace(panelUrl) != "" {
		fmt.Fprintf(&body, "🔗 آدرس پنل: %s\n", esc(panelUrl))
	}
	fmt.Fprintf(&body, "👤 نام کاربری: <code>%s</code>\n", esc(account.Username))
	if account.Disabled {
		body.WriteString("\n⛔️ <b>حساب شما غیرفعال است.</b> برای پیگیری با پشتیبانی تماس بگیرید.\n")
	}
	body.WriteString("\n")
	fmt.Fprintf(&body, "📶 ترافیک مصرفی: <b>%s</b> از <b>%s</b>\n",
		humanBytes(stats.TrafficUsedBytes), quotaGB(stats.TrafficQuotaGB))
	if stats.TrafficQuotaGB > 0 {
		body.WriteString(progressBar(float64(stats.TrafficUsedBytes), float64(stats.TrafficQuotaGB)*bytesPerGB) + "\n")
	}
	fmt.Fprintf(&body, "\n👥 کاربران فعلی: <b>%s</b> از <b>%s</b>\n",
		faNum(int64(stats.CurrentClients)), quotaCount(stats.ClientQuota))
	if stats.ClientQuota > 0 {
		body.WriteString(progressBar(float64(stats.CurrentClients), float64(stats.ClientQuota)) + "\n")
	}
	fmt.Fprintf(&body, "\n🧮 مجموع کاربران ساخته‌شده: <b>%s</b>", faNum(int64(stats.ClientsCreatedTotal)))
	b.send(chatId, body.String(), b.mainMenu(chatId))
}

func (b *Bot) showMyOrders(chatId int64) {
	orders, err := b.salesService.ListOrdersOf(chatId, 10)
	if err != nil || len(orders) == 0 {
		b.send(chatId, msgNoOrders, b.mainMenu(chatId))
		return
	}
	currency, _ := b.settingService.GetSalesBotCurrency()
	var body strings.Builder
	body.WriteString("<b>سفارش‌های شما</b>\n\n")
	for _, o := range orders {
		fmt.Fprintf(&body, "🧾 <b>#%s</b> — %s\n📦 %s (%s)\n💰 %s %s\n",
			faNum(int64(o.Id)), statusLabel(o.Status), esc(o.PackageName), kindLabel(o.Kind),
			faNum(o.Price), esc(currency))
		if o.Status == model.OrderRejected && strings.TrimSpace(o.Note) != "" {
			fmt.Fprintf(&body, "📝 دلیل: %s\n", esc(o.Note))
		}
		body.WriteString("\n")
	}
	b.send(chatId, body.String(), b.mainMenu(chatId))
}

func (b *Bot) showSupport(chatId int64) {
	support, _ := b.settingService.GetSalesBotSupport()
	if strings.TrimSpace(support) == "" {
		b.send(chatId, msgNoSupport, b.mainMenu(chatId))
		return
	}
	b.send(chatId, "📞 پشتیبانی: "+esc(support), b.mainMenu(chatId))
}

// ----------------------------------------------------------- callbacks --

func (b *Bot) onCallback(q telego.CallbackQuery) {
	chatId := q.From.ID
	parts := strings.Split(q.Data, ":")
	action := parts[0]

	switch action {
	case "buy":
		if len(parts) != 3 {
			b.answer(q.ID, "")
			return
		}
		packageId, err := strconv.Atoi(parts[2])
		if err != nil {
			b.answer(q.ID, "")
			return
		}
		b.answer(q.ID, "")
		name := strings.TrimSpace(q.From.FirstName + " " + q.From.LastName)
		b.startOrder(chatId, name, packageId, parts[1])

	case "cancel":
		if len(parts) != 2 {
			return
		}
		orderId, _ := strconv.Atoi(parts[1])
		if err := b.salesService.CancelOrder(orderId, chatId); err != nil {
			b.answer(q.ID, msgAlreadyDecided)
			return
		}
		b.states.clear(chatId)
		b.answer(q.ID, msgOrderCancelled)
		b.send(chatId, msgOrderCancelled, b.mainMenu(chatId))

	default:
		b.onAdminCallback(q)
	}
}

const bytesPerGB = 1024 * 1024 * 1024

// progressBar draws a ten-segment usage bar, which reads far better on a phone
// than a bare percentage.
func progressBar(used, total float64) string {
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
		fmt.Sprintf(" %s٪", faNum(int64(ratio*100+0.5)))
}

// humanBytes formats a byte count for a Persian reader.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return faNum(n) + " بایت"
	}
	units := []string{"کیلوبایت", "مگابایت", "گیگابایت", "ترابایت"}
	value := float64(n)
	idx := -1
	for value >= unit && idx < len(units)-1 {
		value /= unit
		idx++
	}
	return toPersianDigits(fmt.Sprintf("%.2f", value)) + " " + units[idx]
}

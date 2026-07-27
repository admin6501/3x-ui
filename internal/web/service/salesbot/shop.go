package salesbot

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

// Conversation steps of the wallet shop.
const (
	stepTopUpAmount  = "topup_amount"
	stepTopUpReceipt = "topup_receipt"
	stepBuyVolume    = "buy_volume"
	stepAdjustAmount = "adjust_amount"
)

// ------------------------------------------------------------- join gate --

// requireChannel keeps the shop closed to anyone who has not joined the
// configured channel. Returns true when the user may proceed. With no channel
// configured, or when Telegram cannot answer, it lets the user through — a
// broken membership check must not lock the whole shop.
func (b *Bot) requireChannel(userId int64) bool {
	channel, _ := b.settingService.GetShopJoinChannel()
	channel = strings.TrimSpace(channel)
	if channel == "" {
		return true
	}
	api := b.client()
	if api == nil {
		return true
	}
	chatId := tu.Username(normalizeChannel(channel))
	if id, err := strconv.ParseInt(channel, 10, 64); err == nil {
		chatId = tu.ID(id)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	member, err := api.GetChatMember(ctx, &telego.GetChatMemberParams{ChatID: chatId, UserID: userId})
	if err != nil {
		logger.Warning("shop: membership check failed, letting the user through:", err)
		return true
	}
	switch member.MemberStatus() {
	case telego.MemberStatusCreator, telego.MemberStatusAdministrator, telego.MemberStatusMember:
		return true
	}
	return false
}

// normalizeChannel accepts "@name", "name" or a t.me link and returns the bare
// username Telegram wants.
func normalizeChannel(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "t.me/")
	s = strings.TrimPrefix(s, "telegram.me/")
	return "@" + strings.TrimPrefix(s, "@")
}

// promptJoin shows the channel gate.
func (b *Bot) promptJoin(chatId int64) {
	channel, _ := b.settingService.GetShopJoinChannel()
	name := normalizeChannel(channel)
	link := "https://t.me/" + strings.TrimPrefix(name, "@")
	kb := tu.InlineKeyboard(
		tu.InlineKeyboardRow(tu.InlineKeyboardButton("📢 عضویت در کانال").WithURL(link)),
		tu.InlineKeyboardRow(tu.InlineKeyboardButton(btnJoined).WithCallbackData("joined")),
	)
	b.send(chatId, msgMustJoin+"\n\n"+esc(name), kb)
}

// -------------------------------------------------------------- screens --

func (b *Bot) shopMenu(chatId int64) telego.ReplyMarkup {
	rows := [][]telego.KeyboardButton{
		{tu.KeyboardButton(btnWallet), tu.KeyboardButton(btnBuyCfg)},
		{tu.KeyboardButton(btnMyCfgs), tu.KeyboardButton(btnLedger)},
		{tu.KeyboardButton(btnPrices), tu.KeyboardButton(btnSupport)},
		{tu.KeyboardButton(btnHelp)},
	}
	if b.isAdmin(chatId) {
		rows = append(rows, []telego.KeyboardButton{tu.KeyboardButton(btnAdmin)})
	}
	return tu.Keyboard(rows...).WithResizeKeyboard()
}

func (b *Bot) currency() string {
	c, _ := b.settingService.GetSalesBotCurrency()
	return c
}

func (b *Bot) showWallet(chatId int64) {
	user, err := b.shopService.User(chatId, "", "")
	if err != nil {
		b.send(chatId, msgSomethingWrong, b.shopMenu(chatId))
		return
	}
	configs, _ := b.shopService.ListConfigs(chatId)
	active := 0
	for _, cfg := range configs {
		if cfg.Active {
			active++
		}
	}
	kb := tu.InlineKeyboard(tu.InlineKeyboardRow(
		tu.InlineKeyboardButton(btnTopUp).WithCallbackData("topup"),
	))
	b.send(chatId, walletCard(user.Balance, user.TotalPaid, user.TotalSpent, b.currency(), active), kb)
}

func (b *Bot) showPrices(chatId int64) {
	perGB, _ := b.settingService.GetShopPricePerGB()
	perDay, _ := b.settingService.GetShopPricePerDay()
	minTop, _ := b.settingService.GetShopMinTopUp()
	maxTop, _ := b.settingService.GetShopMaxTopUp()
	minBal, _ := b.settingService.GetShopMinBalance()
	maxVol, _ := b.settingService.GetShopMaxVolumeGB()
	b.send(chatId, priceCard(perGB, perDay, b.currency(), minTop, maxTop, minBal, maxVol), b.shopMenu(chatId))
}

func (b *Bot) askTopUpAmount(chatId int64) {
	minTop, _ := b.settingService.GetShopMinTopUp()
	maxTop, _ := b.settingService.GetShopMaxTopUp()
	prompt := msgAskTopUp
	if minTop > 0 || maxTop > 0 {
		prompt += "\n\n"
		if minTop > 0 {
			prompt += fmt.Sprintf("حداقل: <b>%s %s</b>\n", faNum(minTop), esc(b.currency()))
		}
		if maxTop > 0 {
			prompt += fmt.Sprintf("حداکثر: <b>%s %s</b>", faNum(maxTop), esc(b.currency()))
		}
	}
	b.states.set(chatId, &state{step: stepTopUpAmount})
	b.send(chatId, prompt, cancelKeyboard())
}

func (b *Bot) startTopUp(chatId int64, name string, amount int64) {
	row, err := b.shopService.RequestTopUp(chatId, name, amount)
	if err != nil {
		switch err {
		case service.ErrTopUpTooSmall, service.ErrTopUpTooLarge:
			b.send(chatId, "مبلغ خارج از محدوده مجاز است. لطفاً دوباره تلاش کنید.", b.shopMenu(chatId))
		default:
			b.send(chatId, msgSomethingWrong, b.shopMenu(chatId))
		}
		b.states.clear(chatId)
		return
	}
	payText, _ := b.settingService.GetSalesBotPayText()
	b.states.set(chatId, &state{step: stepTopUpReceipt, orderId: row.Id})
	kb := tu.InlineKeyboard(tu.InlineKeyboardRow(
		tu.InlineKeyboardButton(btnCancel).WithCallbackData("topupcancel"),
	))
	b.send(chatId, topUpInstructions(row.Id, row.Amount, b.currency(), payText), kb)
}

// onTopUpReceipt takes the payment proof and queues the top-up for an admin.
func (b *Bot) onTopUpReceipt(msg telego.Message, st *state) {
	chatId := msg.Chat.ID
	fileId := msg.Photo[len(msg.Photo)-1].FileID
	row, err := b.shopService.AttachTopUpReceipt(st.orderId, fileId)
	if err != nil {
		b.states.clear(chatId)
		b.send(chatId, msgOrderGone, b.shopMenu(chatId))
		return
	}
	b.states.clear(chatId)
	b.send(chatId, msgTopUpSent, b.shopMenu(chatId))

	who := strings.TrimSpace(msg.From.FirstName + " " + msg.From.LastName)
	if msg.From.Username != "" {
		who += " (@" + msg.From.Username + ")"
	}
	caption := fmt.Sprintf(
		"💳 <b>درخواست شارژ #%s</b>\n\n👤 %s\n🆔 <code>%d</code>\n💰 مبلغ: <b>%s %s</b>",
		faNum(int64(row.Id)), esc(who), row.TelegramId, faNum(row.Amount), esc(b.currency()),
	)
	kb := tu.InlineKeyboard(tu.InlineKeyboardRow(
		tu.InlineKeyboardButton("✅ تأیید").WithCallbackData(fmt.Sprintf("topok:%d", row.Id)),
		tu.InlineKeyboardButton("❌ رد").WithCallbackData(fmt.Sprintf("topno:%d", row.Id)),
	))
	for _, adminId := range b.admins() {
		if row.ReceiptFileId != "" {
			b.sendPhoto(adminId, row.ReceiptFileId, caption, kb)
			continue
		}
		b.send(adminId, caption, kb)
	}
}

func (b *Bot) askVolume(chatId int64) {
	user, err := b.shopService.User(chatId, "", "")
	if err != nil {
		b.send(chatId, msgSomethingWrong, b.shopMenu(chatId))
		return
	}
	if user.Blocked {
		b.send(chatId, msgBlocked, b.shopMenu(chatId))
		return
	}
	minBal, _ := b.settingService.GetShopMinBalance()
	if user.Balance <= 0 || user.Balance < minBal {
		kb := tu.InlineKeyboard(tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(btnTopUp).WithCallbackData("topup"),
		))
		b.send(chatId, msgNeedBalance, kb)
		return
	}
	perGB, _ := b.settingService.GetShopPricePerGB()
	maxVol, _ := b.settingService.GetShopMaxVolumeGB()
	prompt := msgAskVolume
	if perGB > 0 {
		prompt += fmt.Sprintf("\n\n💡 هر گیگ مصرفی <b>%s %s</b>. با موجودی فعلی شما تا حدود <b>%s گیگابایت</b> مصرف قابل پرداخت است.",
			faNum(perGB), esc(b.currency()), faNum(user.Balance/perGB))
	}
	if maxVol > 0 {
		prompt += fmt.Sprintf("\n📦 حداکثر حجم مجاز: <b>%s گیگابایت</b>", faNum(maxVol))
	}
	b.states.set(chatId, &state{step: stepBuyVolume})
	b.send(chatId, prompt, cancelKeyboard())
}

func (b *Bot) createConfig(chatId int64, volumeGB int64) {
	cfg, err := b.shopService.CreateConfig(b.inboundService, chatId, volumeGB)
	if err != nil {
		switch err {
		case service.ErrInsufficientFund:
			b.send(chatId, msgNeedBalance, b.shopMenu(chatId))
		case service.ErrVolumeTooLarge:
			maxVol, _ := b.settingService.GetShopMaxVolumeGB()
			b.send(chatId, fmt.Sprintf("حداکثر حجم مجاز <b>%s گیگابایت</b> است.", faNum(maxVol)), b.shopMenu(chatId))
		case service.ErrVolumeInvalid:
			b.send(chatId, msgVolumeBad, b.shopMenu(chatId))
		case service.ErrNoShopInbound:
			b.send(chatId, msgShopNoInbound, b.shopMenu(chatId))
		case service.ErrUserBlocked:
			b.send(chatId, msgBlocked, b.shopMenu(chatId))
		default:
			b.send(chatId, msgSomethingWrong, b.shopMenu(chatId))
		}
		return
	}
	b.send(chatId, "✅ کانفیگ شما ساخته شد:", b.shopMenu(chatId))
	b.sendConfig(chatId, cfg, 0, 0)
}

// publicHost is the address the shop puts into the links it hands out. The
// panel has no incoming request to infer it from when the bot builds a link, so
// it comes from the shop's own setting.
func (b *Bot) publicHost() string {
	raw, _ := b.settingService.GetSalesBotPanelUrl()
	if host := hostFromPanelUrl(raw); host != "" {
		return host
	}
	// Nothing set for the shop: fall back to whatever the panel already knows
	// about its own public name, the same order the subscription server uses.
	if d, err := b.settingService.GetSubDomain(); err == nil {
		if host := hostFromPanelUrl(d); host != "" {
			return host
		}
	}
	if d, err := b.settingService.GetWebDomain(); err == nil {
		return hostFromPanelUrl(d)
	}
	return ""
}

// hostFromPanelUrl reduces whatever an admin pasted into the setting to a bare
// host[:port]. They paste the address bar of their panel as often as they type a
// hostname, and a link built from "https://host:2053/panel/" is a dead link.
func hostFromPanelUrl(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// Accept "https://host:2053/" as well as a bare "host".
	raw = strings.TrimPrefix(strings.TrimPrefix(raw, "https://"), "http://")
	raw, _, _ = strings.Cut(raw, "/")
	return strings.TrimSpace(raw)
}

// subLink builds the subscription URL a client app is pointed at. It prefers
// the explicitly configured subscription URI and otherwise derives one from the
// subscription server's own settings, so a panel that never filled in subURI —
// which is the default — still hands out a working link.
func (b *Bot) subLink(cfg *model.BotConfig) string {
	if cfg.SubID == "" {
		return ""
	}
	base, _ := b.settingService.GetSubURI()
	if strings.TrimSpace(base) == "" {
		host := b.publicHost()
		if host == "" {
			return ""
		}
		base = b.settingService.BuildSubURIBase(host)
	}
	return joinSubLink(base, cfg.SubID)
}

// joinSubLink appends a subscription id to a base URI, tolerating a base with or
// without its trailing slash. An empty base means the panel has nothing to build
// a subscription link from, and the caller must fall back to the direct links.
func joinSubLink(base, subID string) string {
	base = strings.TrimSpace(base)
	if base == "" || subID == "" {
		return ""
	}
	return strings.TrimSuffix(base, "/") + "/" + subID
}

// configLinks returns the client's direct share links (vless://…). These are
// what a buyer actually pastes into their app, and unlike the subscription URL
// they need no subscription server to be configured at all — so they are the
// shop's primary deliverable.
func (b *Bot) configLinks(cfg *model.BotConfig) []string {
	if b.inboundService == nil {
		return nil
	}
	host := b.publicHost()
	if host == "" {
		// Without a host the builder still returns links, but with an empty
		// address in them — worse than none, because nothing would flag them.
		logger.Warning("shop: no public address configured, cannot build config links for", cfg.Email)
		return nil
	}
	links, err := b.inboundService.GetAllClientLinks(host, cfg.Email)
	if err != nil {
		logger.Warning("shop: could not build config links for", cfg.Email, err)
		return nil
	}
	return links
}

// sendConfig delivers one config: its usage card, then the links themselves.
// A config with no deliverable link is a sale that gave the buyer nothing, so
// that case says so out loud rather than sending a card and going quiet.
func (b *Bot) sendConfig(chatId int64, cfg *model.BotConfig, usedBytes, cost int64) {
	sub := b.subLink(cfg)
	b.send(chatId, configCard(cfg.Email, cfg.VolumeGB, usedBytes, cost, cfg.Active, b.currency(), sub))

	links := b.configLinks(cfg)
	if len(links) == 0 && sub == "" {
		b.send(chatId, msgNoLinkYet)
		for _, adminId := range b.admins() {
			b.send(adminId, fmt.Sprintf(
				"⚠️ برای کانفیگ <code>%s</code> هیچ لینکی ساخته نشد. «آدرس عمومی برای لینک‌ها» را در تنظیمات ربات فروش پر کنید.",
				esc(cfg.Email)))
		}
		return
	}
	for _, link := range links {
		b.send(chatId, "<code>"+esc(link)+"</code>")
	}
}

func (b *Bot) showConfigs(chatId int64) {
	configs, err := b.shopService.ListConfigs(chatId)
	if err != nil || len(configs) == 0 {
		b.send(chatId, msgNoConfigs, b.shopMenu(chatId))
		return
	}
	for i := range configs {
		cfg := &configs[i]
		usage := b.shopService.Usage(cfg)
		b.sendConfig(chatId, cfg, usage.UsedBytes, cfg.ChargedTraffic+cfg.ChargedDays)
	}
}

func (b *Bot) showLedger(chatId int64) {
	entries, err := b.shopService.Transactions(chatId, 15)
	if err != nil || len(entries) == 0 {
		b.send(chatId, msgNoLedger, b.shopMenu(chatId))
		return
	}
	var body strings.Builder
	body.WriteString("<b>آخرین تراکنش‌ها</b>\n\n")
	for _, e := range entries {
		body.WriteString(txLine(e.Amount, e.Balance, e.Kind, e.Details, b.currency()))
		body.WriteString("\n\n")
	}
	b.send(chatId, body.String(), b.shopMenu(chatId))
}

// NotifySuspended tells users the billing job just cut off why that happened.
func (b *Bot) NotifySuspended(ids []int64) {
	if !b.IsRunning() {
		return
	}
	for _, id := range ids {
		b.send(id, msgSuspended, b.shopMenu(id))
	}
}

// -------------------------------------------------------- admin screens --

func (b *Bot) showTopUpQueue(chatId int64) {
	rows, err := b.shopService.ListTopUps(model.TopUpReview, 20)
	if err != nil || len(rows) == 0 {
		b.send(chatId, "درخواست شارژی در انتظار بررسی نیست ✅", b.adminMenu())
		return
	}
	for _, row := range rows {
		caption := fmt.Sprintf("💳 <b>شارژ #%s</b>\n👤 %s\n🆔 <code>%d</code>\n💰 <b>%s %s</b>",
			faNum(int64(row.Id)), esc(row.TelegramName), row.TelegramId,
			faNum(row.Amount), esc(b.currency()))
		kb := tu.InlineKeyboard(tu.InlineKeyboardRow(
			tu.InlineKeyboardButton("✅ تأیید").WithCallbackData(fmt.Sprintf("topok:%d", row.Id)),
			tu.InlineKeyboardButton("❌ رد").WithCallbackData(fmt.Sprintf("topno:%d", row.Id)),
		))
		if row.ReceiptFileId != "" {
			b.sendPhoto(chatId, row.ReceiptFileId, caption, kb)
			continue
		}
		b.send(chatId, caption, kb)
	}
}

func (b *Bot) approveTopUp(adminId int64, id int) {
	row, balance, err := b.shopService.ApproveTopUp(id)
	if err != nil {
		b.send(adminId, msgAlreadyDecided)
		return
	}
	// Paying puts the user's configs back on without waiting for the next
	// billing tick.
	b.shopService.BillAll(b.inboundService)
	b.send(row.TelegramId, fmt.Sprintf(
		"✅ کیف پول شما <b>%s %s</b> شارژ شد.\n\n💰 موجودی جدید: <b>%s %s</b>",
		faNum(row.Amount), esc(b.currency()), faNum(balance), esc(b.currency())),
		b.shopMenu(row.TelegramId))
	b.send(adminId, fmt.Sprintf("شارژ #%s تأیید شد.", faNum(int64(id))))
}

func (b *Bot) rejectTopUp(adminId int64, id int, note string) {
	row, err := b.shopService.RejectTopUp(id, note)
	if err != nil {
		b.send(adminId, msgAlreadyDecided)
		return
	}
	text := fmt.Sprintf("❌ درخواست شارژ شماره <b>%s</b> تأیید نشد.", faNum(int64(row.Id)))
	if strings.TrimSpace(note) != "" {
		text += "\n📝 دلیل: " + esc(note)
	}
	b.send(row.TelegramId, text, b.shopMenu(row.TelegramId))
	b.send(adminId, fmt.Sprintf("شارژ #%s رد شد.", faNum(int64(id))), b.adminMenu())
}

func (b *Bot) showShopUsers(chatId int64) {
	users, err := b.shopService.ListUsers(20)
	if err != nil || len(users) == 0 {
		b.send(chatId, "هنوز کاربری در فروشگاه نیست.", b.adminMenu())
		return
	}
	var rows [][]telego.InlineKeyboardButton
	var body strings.Builder
	body.WriteString("<b>کاربران فروشگاه</b>\n\n")
	for i := range users {
		u := &users[i]
		mark := "🟢"
		if u.Blocked {
			mark = "⛔️"
		}
		name := u.FirstName
		if u.Username != "" {
			name += " @" + u.Username
		}
		fmt.Fprintf(&body, "%s <code>%d</code> %s\n💰 %s %s | 📤 %s\n\n",
			mark, u.TelegramId, esc(strings.TrimSpace(name)),
			faNum(u.Balance), esc(b.currency()), faNum(u.TotalSpent))
		action := "⛔️ مسدود"
		if u.Blocked {
			action = "✅ آزاد"
		}
		rows = append(rows, tu.InlineKeyboardRow(
			tu.InlineKeyboardButton("💵 اصلاح موجودی").WithCallbackData(fmt.Sprintf("adj:%d", u.TelegramId)),
			tu.InlineKeyboardButton(action).WithCallbackData(fmt.Sprintf("blk:%d", u.TelegramId)),
		))
	}
	b.send(chatId, body.String(), tu.InlineKeyboard(rows...))
}

func (b *Bot) showAllConfigs(chatId int64) {
	configs, err := b.shopService.ListAllConfigs(20)
	if err != nil || len(configs) == 0 {
		b.send(chatId, "هنوز کانفیگی ساخته نشده است.", b.adminMenu())
		return
	}
	var body strings.Builder
	body.WriteString("<b>کانفیگ‌های فروشگاه</b>\n\n")
	for i := range configs {
		cfg := &configs[i]
		usage := b.shopService.Usage(cfg)
		mark := "🟢"
		if !cfg.Active {
			mark = "⛔️"
		}
		fmt.Fprintf(&body, "%s <code>%s</code> (🆔 %d)\n📶 %s از %s | 💳 %s %s\n\n",
			mark, esc(cfg.Email), cfg.TelegramId,
			humanBytes(usage.UsedBytes), quotaGB(cfg.VolumeGB),
			faNum(cfg.ChargedTraffic+cfg.ChargedDays), esc(b.currency()))
	}
	b.send(chatId, body.String(), b.adminMenu())
}

func (b *Bot) showShopStats(chatId int64) {
	stats := b.shopService.Stats()
	perGB, _ := b.settingService.GetShopPricePerGB()
	var body strings.Builder
	body.WriteString("<b>آمار فروشگاه</b>\n\n")
	fmt.Fprintf(&body, "👥 کاربران: <b>%s</b>\n", faNum(stats.Users))
	fmt.Fprintf(&body, "📱 کانفیگ‌ها: <b>%s</b> (فعال: %s)\n", faNum(stats.Configs), faNum(stats.ActiveConfigs))
	fmt.Fprintf(&body, "📥 مجموع شارژ: <b>%s %s</b>\n", faNum(stats.TotalPaid), esc(b.currency()))
	fmt.Fprintf(&body, "📤 مجموع مصرف: <b>%s %s</b>\n", faNum(stats.TotalSpent), esc(b.currency()))
	fmt.Fprintf(&body, "💰 موجودی در گردش: <b>%s %s</b>\n", faNum(stats.WalletBalance), esc(b.currency()))
	fmt.Fprintf(&body, "🔎 شارژ در انتظار: <b>%s</b>\n", faNum(stats.PendingTopUps))
	fmt.Fprintf(&body, "⛔️ کاربران بی‌موجودی: <b>%s</b>\n\n", faNum(stats.SuspendedUsers))
	fmt.Fprintf(&body, "🏷 تعرفه فعلی: <b>%s %s</b> بر گیگابایت", faNum(perGB), esc(b.currency()))
	b.send(chatId, body.String(), b.adminMenu())
}

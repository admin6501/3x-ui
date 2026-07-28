package salesbot

import (
	"context"
	"errors"
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
	stepAddVolume    = "add_volume"
	stepAdjustAmount = "adjust_amount"
	stepDiscountCode = "discount_code"
	stepNewDiscount  = "new_discount"
)

func nowMilli() int64 { return time.Now().UnixMilli() }

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
	b.showTopUpInstructions(chatId, row)
}

// showTopUpInstructions is the screen a buyer sits on until they send a receipt:
// what to pay, where, and the option to attach a discount code first.
func (b *Bot) showTopUpInstructions(chatId int64, row *model.WalletTopUp) {
	payText, _ := b.settingService.GetSalesBotPayText()
	b.states.set(chatId, &state{step: stepTopUpReceipt, orderId: row.Id})

	buttons := []telego.InlineKeyboardButton{}
	if b.discountsOffered() && row.DiscountCode == "" {
		buttons = append(buttons, tu.InlineKeyboardButton(btnHaveCode).WithCallbackData(fmt.Sprintf("code:%d", row.Id)))
	}
	buttons = append(buttons, tu.InlineKeyboardButton(btnCancel).WithCallbackData("topupcancel"))

	bonus := int64(0)
	if row.DiscountCode != "" {
		_, bonus, _ = b.shopService.ValidateDiscount(row.DiscountCode, chatId, row.Amount)
	}
	b.send(chatId,
		topUpInstructions(row.Id, row.Amount, b.currency(), payText, row.DiscountCode, bonus),
		tu.InlineKeyboard(tu.InlineKeyboardRow(buttons...)))
}

// discountsOffered hides the code button when the shop has no usable code, so a
// buyer is not invited to hunt for something that does not exist.
func (b *Bot) discountsOffered() bool {
	codes, err := b.shopService.ListDiscounts(50)
	if err != nil {
		return false
	}
	now := nowMilli()
	for i := range codes {
		c := &codes[i]
		if !c.Enabled {
			continue
		}
		if c.ExpiresAt > 0 && now > c.ExpiresAt {
			continue
		}
		if c.MaxUses > 0 && c.Used >= c.MaxUses {
			continue
		}
		return true
	}
	return false
}

// askDiscountCode starts the "I have a code" step for a pending top-up.
func (b *Bot) askDiscountCode(chatId int64, topUpId int) {
	row, err := b.shopService.GetTopUp(topUpId)
	if err != nil || row.TelegramId != chatId {
		b.send(chatId, msgOrderGone, b.shopMenu(chatId))
		return
	}
	b.states.set(chatId, &state{step: stepDiscountCode, orderId: topUpId})
	b.send(chatId, msgAskDiscountCode, cancelKeyboard())
}

// applyDiscountCode validates what the buyer typed and attaches it to the
// top-up. A bad code puts them back on the same step rather than dropping them
// out of the flow.
func (b *Bot) applyDiscountCode(chatId int64, topUpId int, typed string) {
	row, err := b.shopService.GetTopUp(topUpId)
	if err != nil || row.TelegramId != chatId {
		b.states.clear(chatId)
		b.send(chatId, msgOrderGone, b.shopMenu(chatId))
		return
	}
	code, bonus, err := b.shopService.ValidateDiscount(typed, chatId, row.Amount)
	if err != nil {
		b.send(chatId, discountError(err))
		return
	}
	updated, err := b.shopService.AttachDiscountCode(topUpId, code.Code)
	if err != nil {
		b.states.clear(chatId)
		b.send(chatId, msgOrderGone, b.shopMenu(chatId))
		return
	}
	b.send(chatId, fmt.Sprintf(msgDiscountApplied,
		esc(code.Code), faNum(int64(code.Percent)), faNum(bonus), esc(b.currency()),
		faNum(row.Amount+bonus), esc(b.currency())))
	b.showTopUpInstructions(chatId, updated)
}

// discountError turns a validation failure into something a buyer can act on.
func discountError(err error) string {
	switch {
	case errors.Is(err, service.ErrDiscountExpired):
		return msgDiscountExpired
	case errors.Is(err, service.ErrDiscountUsedUp):
		return msgDiscountUsedUp
	case errors.Is(err, service.ErrDiscountAlready):
		return msgDiscountAlready
	default:
		return msgDiscountUnknown
	}
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
	// The admin decides on the code as much as on the payment, so it belongs in
	// the message they approve from.
	if row.DiscountCode != "" {
		if code, bonus, err := b.shopService.ValidateDiscount(row.DiscountCode, row.TelegramId, row.Amount); err == nil {
			caption += fmt.Sprintf("\n🏷 کد تخفیف: <code>%s</code> (%s٪) → هدیه <b>%s %s</b>\n🧮 مجموع واریز به کیف پول: <b>%s %s</b>",
				esc(code.Code), faNum(int64(code.Percent)), faNum(bonus), esc(b.currency()),
				faNum(row.Amount+bonus), esc(b.currency()))
		} else {
			caption += fmt.Sprintf("\n🏷 کد تخفیف <code>%s</code> دیگر معتبر نیست و اعمال نمی‌شود.", esc(row.DiscountCode))
		}
	}
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
// subscription server's own settings — including its path, without which the URL
// points at the subscription server's root and serves nothing. A panel with the
// subscription server switched off has no such URL to give at all.
func (b *Bot) subLink(cfg *model.BotConfig) string {
	if cfg.SubID == "" {
		return ""
	}
	if on, err := b.settingService.GetSubEnable(); err == nil && !on {
		return ""
	}
	base, _ := b.settingService.GetSubURI()
	if strings.TrimSpace(base) == "" {
		host := b.publicHost()
		if host == "" {
			return ""
		}
		base = b.settingService.BuildSubURI(host)
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
	b.sendLinks(chatId, cfg, sub)
}

// sendLinks delivers the direct config links, or explains their absence. A
// config with no deliverable link is a sale that gave the buyer nothing, so that
// case says so out loud rather than going quiet.
func (b *Bot) sendLinks(chatId int64, cfg *model.BotConfig, sub string) {
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

// showConfigs lists the buyer's configs by name, one button each. Dumping every
// card and link at once buried the useful ones once a buyer had more than two;
// picking a name opens that config's own screen instead.
func (b *Bot) showConfigs(chatId int64) {
	configs, err := b.shopService.ListConfigs(chatId)
	if err != nil || len(configs) == 0 {
		b.send(chatId, msgNoConfigs, b.shopMenu(chatId))
		return
	}
	rows := make([][]telego.InlineKeyboardButton, 0, len(configs))
	for i := range configs {
		cfg := &configs[i]
		rows = append(rows, tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(configButtonLabel(cfg)).WithCallbackData(fmt.Sprintf("cfg:%d", cfg.Id)),
		))
	}
	b.send(chatId, msgPickConfig, tu.InlineKeyboard(rows...))
}

// configButtonLabel names a config in the list: its state, its name and its size,
// which is everything needed to tell two of them apart at a glance.
func configButtonLabel(cfg *model.BotConfig) string {
	mark := "🟢"
	switch {
	case cfg.Paused:
		mark = "⏸"
	case !cfg.Active:
		mark = "⛔️"
	}
	return fmt.Sprintf("%s %s — %s", mark, cfg.Email, quotaGB(cfg.VolumeGB))
}

// ownedConfig fetches a config and refuses it to anyone but its owner, so a
// guessed id in a callback cannot reach someone else's account.
func (b *Bot) ownedConfig(chatId int64, id int) *model.BotConfig {
	cfg, err := b.shopService.GetConfig(id)
	if err != nil || cfg.TelegramId != chatId {
		return nil
	}
	return cfg
}

// showConfigMenu is one config's own screen: what it has used, what it has cost,
// and every action its owner can take on it.
func (b *Bot) showConfigMenu(chatId int64, id int) {
	cfg := b.ownedConfig(chatId, id)
	if cfg == nil {
		b.send(chatId, msgConfigGone, b.shopMenu(chatId))
		return
	}
	usage := b.shopService.Usage(cfg)
	body := configDetailCard(cfg, usage.UsedBytes, cfg.ChargedTraffic+cfg.ChargedDays, b.currency())

	toggle := btnCfgPause
	if cfg.Paused {
		toggle = btnCfgResume
	}
	kb := tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(btnCfgLinks).WithCallbackData(fmt.Sprintf("cfglink:%d", cfg.Id)),
			tu.InlineKeyboardButton(btnCfgAddVol).WithCallbackData(fmt.Sprintf("cfgvol:%d", cfg.Id)),
		),
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(toggle).WithCallbackData(fmt.Sprintf("cfgtog:%d", cfg.Id)),
			tu.InlineKeyboardButton(btnCfgDelete).WithCallbackData(fmt.Sprintf("cfgdel:%d", cfg.Id)),
		),
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(btnCfgBack).WithCallbackData("cfglist"),
		),
	)
	b.send(chatId, body, kb)
}

// toggleConfig pauses or resumes a config on its owner's request.
func (b *Bot) toggleConfig(chatId int64, id int) {
	cfg := b.ownedConfig(chatId, id)
	if cfg == nil {
		b.send(chatId, msgConfigGone, b.shopMenu(chatId))
		return
	}
	updated, err := b.shopService.SetConfigPaused(b.inboundService, id, !cfg.Paused)
	if err != nil {
		b.send(chatId, msgSomethingWrong, b.shopMenu(chatId))
		return
	}
	switch {
	case updated.Paused:
		b.send(chatId, msgConfigPaused)
	case updated.Active:
		b.send(chatId, msgConfigResumed)
	default:
		// Un-paused but still off: the wallet cannot pay for it yet.
		b.send(chatId, msgConfigNeedsFunds)
	}
	b.showConfigMenu(chatId, id)
}

// askAddVolume starts the add-volume step for one config.
func (b *Bot) askAddVolume(chatId int64, id int) {
	if b.ownedConfig(chatId, id) == nil {
		b.send(chatId, msgConfigGone, b.shopMenu(chatId))
		return
	}
	b.states.set(chatId, &state{step: stepAddVolume, configId: id})
	b.send(chatId, msgAskAddVolume, cancelKeyboard())
}

// addVolume applies the number the owner typed at the add-volume step.
func (b *Bot) addVolume(chatId int64, id int, extraGB int64) {
	b.states.clear(chatId)
	if b.ownedConfig(chatId, id) == nil {
		b.send(chatId, msgConfigGone, b.shopMenu(chatId))
		return
	}
	cfg, err := b.shopService.AddVolume(b.inboundService, id, extraGB)
	if err != nil {
		switch err {
		case service.ErrVolumeTooLarge:
			maxVol, _ := b.settingService.GetShopMaxVolumeGB()
			b.send(chatId, fmt.Sprintf("حداکثر حجم مجاز هر کانفیگ <b>%s گیگابایت</b> است.", faNum(maxVol)), b.shopMenu(chatId))
		case service.ErrVolumeInvalid:
			b.send(chatId, msgVolumeBad, b.shopMenu(chatId))
		default:
			b.send(chatId, msgSomethingWrong, b.shopMenu(chatId))
		}
		return
	}
	b.send(chatId, fmt.Sprintf("✅ حجم کانفیگ به <b>%s</b> رسید.", quotaGB(cfg.VolumeGB)), b.shopMenu(chatId))
	b.showConfigMenu(chatId, id)
}

// confirmDeleteConfig asks before destroying a config, because the button sits
// next to the ones a buyer presses routinely.
func (b *Bot) confirmDeleteConfig(chatId int64, id int) {
	cfg := b.ownedConfig(chatId, id)
	if cfg == nil {
		b.send(chatId, msgConfigGone, b.shopMenu(chatId))
		return
	}
	kb := tu.InlineKeyboard(tu.InlineKeyboardRow(
		tu.InlineKeyboardButton(btnCfgDeleteYes).WithCallbackData(fmt.Sprintf("cfgdelok:%d", cfg.Id)),
		tu.InlineKeyboardButton(btnCfgBack).WithCallbackData(fmt.Sprintf("cfg:%d", cfg.Id)),
	))
	b.send(chatId, fmt.Sprintf(msgConfirmDelete, esc(cfg.Email)), kb)
}

func (b *Bot) deleteConfig(chatId int64, id int) {
	if b.ownedConfig(chatId, id) == nil {
		b.send(chatId, msgConfigGone, b.shopMenu(chatId))
		return
	}
	if err := b.shopService.DeleteConfig(b.inboundService, id); err != nil {
		b.send(chatId, msgSomethingWrong, b.shopMenu(chatId))
		return
	}
	b.send(chatId, msgConfigDeleted, b.shopMenu(chatId))
}

// sendConfigLinks re-sends one config's links on request.
func (b *Bot) sendConfigLinks(chatId int64, id int) {
	cfg := b.ownedConfig(chatId, id)
	if cfg == nil {
		b.send(chatId, msgConfigGone, b.shopMenu(chatId))
		return
	}
	b.sendLinks(chatId, cfg, b.subLink(cfg))
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
		if row.DiscountCode != "" {
			if code, bonus, err := b.shopService.ValidateDiscount(row.DiscountCode, row.TelegramId, row.Amount); err == nil {
				caption += fmt.Sprintf("\n🏷 <code>%s</code> (%s٪) → هدیه <b>%s %s</b>",
					esc(code.Code), faNum(int64(code.Percent)), faNum(bonus), esc(b.currency()))
			}
		}
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
	text := fmt.Sprintf("✅ کیف پول شما <b>%s %s</b> شارژ شد.",
		faNum(row.Amount), esc(b.currency()))
	if row.Bonus > 0 {
		text += fmt.Sprintf("\n🏷 با کد <code>%s</code> مبلغ <b>%s %s</b> هدیه هم گرفتید.",
			esc(row.DiscountCode), faNum(row.Bonus), esc(b.currency()))
	}
	text += fmt.Sprintf("\n\n💰 موجودی جدید: <b>%s %s</b>", faNum(balance), esc(b.currency()))
	b.send(row.TelegramId, text, b.shopMenu(row.TelegramId))

	adminNote := fmt.Sprintf("شارژ #%s تأیید شد.", faNum(int64(id)))
	if row.DiscountCode != "" && row.Bonus == 0 {
		adminNote += fmt.Sprintf("\n⚠️ کد <code>%s</code> دیگر معتبر نبود و هدیه‌ای اعمال نشد.", esc(row.DiscountCode))
	}
	b.send(adminId, adminNote)
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

// showShopUsers lists shop users by name, one button each. Dumping every user's
// figures and two action buttons into a single message made the list unreadable
// and the buttons impossible to match to a row; picking a name opens that user's
// own screen instead.
func (b *Bot) showShopUsers(chatId int64) {
	users, err := b.shopService.ListUsers(30)
	if err != nil || len(users) == 0 {
		b.send(chatId, "هنوز کاربری در فروشگاه نیست.", b.adminMenu())
		return
	}
	rows := make([][]telego.InlineKeyboardButton, 0, len(users))
	for i := range users {
		u := &users[i]
		rows = append(rows, tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(userButtonLabel(u, b.currency())).
				WithCallbackData(fmt.Sprintf("usr:%d", u.TelegramId)),
		))
	}
	b.send(chatId, msgPickUser, tu.InlineKeyboard(rows...))
}

// userButtonLabel names a shop user in the list: state, who they are, and the
// balance — the number an admin is nearly always looking for.
func userButtonLabel(u *model.BotUser, currency string) string {
	mark := "🟢"
	if u.Blocked {
		mark = "⛔️"
	}
	name := strings.TrimSpace(u.FirstName)
	if u.Username != "" {
		name = strings.TrimSpace(name + " @" + u.Username)
	}
	if name == "" {
		name = fmt.Sprintf("%d", u.TelegramId)
	}
	return fmt.Sprintf("%s %s — %s %s", mark, name, faNum(u.Balance), currency)
}

// showUserMenu is one shop user's own screen: their wallet, their configs and
// every action an admin can take on them.
func (b *Bot) showUserMenu(adminId int64, telegramId int64) {
	u, err := b.shopService.GetUser(telegramId)
	if err != nil {
		b.send(adminId, msgUserGone, b.adminMenu())
		return
	}
	configs, _ := b.shopService.ListConfigs(telegramId)
	pending := b.shopService.CountPendingTopUpsOf(telegramId)

	block := "⛔️ مسدود کردن"
	if u.Blocked {
		block = "✅ رفع مسدودی"
	}
	kb := tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton("💵 اصلاح موجودی").WithCallbackData(fmt.Sprintf("adj:%d", u.TelegramId)),
			tu.InlineKeyboardButton(block).WithCallbackData(fmt.Sprintf("blk:%d", u.TelegramId)),
		),
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton("📱 کانفیگ‌ها").WithCallbackData(fmt.Sprintf("usrcfg:%d", u.TelegramId)),
			tu.InlineKeyboardButton("💳 تراکنش‌ها").WithCallbackData(fmt.Sprintf("usrtx:%d", u.TelegramId)),
		),
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(btnCfgBack).WithCallbackData("usrlist"),
		),
	)
	b.send(adminId, userDetailCard(u, len(configs), pending, b.currency()), kb)
}

// showUserConfigs is the admin's view of one user's configs.
func (b *Bot) showUserConfigs(adminId int64, telegramId int64) {
	configs, err := b.shopService.ListConfigs(telegramId)
	if err != nil || len(configs) == 0 {
		b.send(adminId, "این کاربر هنوز کانفیگی ندارد.")
		return
	}
	var body strings.Builder
	fmt.Fprintf(&body, "<b>کانفیگ‌های کاربر <code>%d</code></b>\n\n", telegramId)
	for i := range configs {
		cfg := &configs[i]
		usage := b.shopService.Usage(cfg)
		mark := "🟢"
		switch {
		case cfg.Paused:
			mark = "⏸"
		case !cfg.Active:
			mark = "⛔️"
		}
		fmt.Fprintf(&body, "%s <code>%s</code>\n📶 %s از %s | 💳 %s %s\n\n",
			mark, esc(cfg.Email), humanBytes(usage.UsedBytes), quotaGB(cfg.VolumeGB),
			faNum(cfg.ChargedTraffic+cfg.ChargedDays), esc(b.currency()))
	}
	b.send(adminId, body.String())
}

// showUserLedger is the admin's view of one user's transactions.
func (b *Bot) showUserLedger(adminId int64, telegramId int64) {
	entries, err := b.shopService.Transactions(telegramId, 15)
	if err != nil || len(entries) == 0 {
		b.send(adminId, "این کاربر هنوز تراکنشی ندارد.")
		return
	}
	var body strings.Builder
	fmt.Fprintf(&body, "<b>تراکنش‌های کاربر <code>%d</code></b>\n\n", telegramId)
	for _, e := range entries {
		body.WriteString(txLine(e.Amount, e.Balance, e.Kind, e.Details, b.currency()))
		body.WriteString("\n\n")
	}
	b.send(adminId, body.String())
}

// ------------------------------------------------------ discount codes --

func (b *Bot) showDiscounts(chatId int64) {
	codes, err := b.shopService.ListDiscounts(30)
	if err != nil {
		b.send(chatId, msgSomethingWrong, b.adminMenu())
		return
	}
	rows := make([][]telego.InlineKeyboardButton, 0, len(codes)+1)
	for i := range codes {
		c := &codes[i]
		rows = append(rows, tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(discountButtonLabel(c)).WithCallbackData(fmt.Sprintf("dsc:%d", c.Id)),
		))
	}
	rows = append(rows, tu.InlineKeyboardRow(
		tu.InlineKeyboardButton(btnNewCode).WithCallbackData("dscnew"),
	))
	body := msgNoDiscounts
	if len(codes) > 0 {
		body = msgPickDiscount
	}
	b.send(chatId, body, tu.InlineKeyboard(rows...))
}

func discountButtonLabel(c *model.DiscountCode) string {
	mark := "🟢"
	switch {
	case !c.Enabled:
		mark = "⛔️"
	case c.ExpiresAt > 0 && nowMilli() > c.ExpiresAt:
		mark = "⌛️"
	case c.MaxUses > 0 && c.Used >= c.MaxUses:
		mark = "🔚"
	}
	return fmt.Sprintf("%s %s — %s٪", mark, c.Code, faNum(int64(c.Percent)))
}

func (b *Bot) showDiscountMenu(chatId int64, id int) {
	c, err := b.shopService.GetDiscount(id)
	if err != nil {
		b.send(chatId, msgDiscountGone, b.adminMenu())
		return
	}
	toggle := "⛔️ غیرفعال کردن"
	if !c.Enabled {
		toggle = "✅ فعال کردن"
	}
	kb := tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(toggle).WithCallbackData(fmt.Sprintf("dsctog:%d", c.Id)),
			tu.InlineKeyboardButton("🗑 حذف").WithCallbackData(fmt.Sprintf("dscdel:%d", c.Id)),
		),
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(btnCfgBack).WithCallbackData("dsclist"),
		),
	)
	b.send(chatId, discountCard(c, b.currency()), kb)
}

func (b *Bot) toggleDiscount(chatId int64, id int) {
	c, err := b.shopService.GetDiscount(id)
	if err != nil {
		b.send(chatId, msgDiscountGone, b.adminMenu())
		return
	}
	if _, err := b.shopService.SetDiscountEnabled(id, !c.Enabled); err != nil {
		b.send(chatId, msgSomethingWrong, b.adminMenu())
		return
	}
	b.showDiscountMenu(chatId, id)
}

func (b *Bot) deleteDiscount(chatId int64, id int) {
	if err := b.shopService.DeleteDiscount(id); err != nil {
		b.send(chatId, msgSomethingWrong, b.adminMenu())
		return
	}
	b.send(chatId, msgDiscountDeleted)
	b.showDiscounts(chatId)
}

// askNewDiscount starts the create-a-code wizard. It is one free-text answer:
// asking four questions in a row for something an owner types once is worse
// than one line with a documented shape.
func (b *Bot) askNewDiscount(chatId int64) {
	b.states.set(chatId, &state{step: stepNewDiscount})
	b.send(chatId, msgAskNewDiscount, cancelKeyboard())
}

// createDiscount parses the wizard's one line: CODE PERCENT [MAX_USES] [DAYS].
func (b *Bot) createDiscount(chatId int64, line string) {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) < 2 {
		b.send(chatId, msgDiscountFormatBad)
		return
	}
	percent, ok := parseNumber(fields[1])
	if !ok || percent <= 0 || percent > 100 {
		b.send(chatId, msgDiscountPercentBad)
		return
	}
	maxUses := int64(0)
	if len(fields) >= 3 {
		if maxUses, ok = parseNumber(fields[2]); !ok {
			b.send(chatId, msgDiscountFormatBad)
			return
		}
	}
	expiresAt := int64(0)
	if len(fields) >= 4 {
		days, ok := parseNumber(fields[3])
		if !ok {
			b.send(chatId, msgDiscountFormatBad)
			return
		}
		if days > 0 {
			expiresAt = time.Now().AddDate(0, 0, int(days)).UnixMilli()
		}
	}

	b.states.clear(chatId)
	code, err := b.shopService.CreateDiscount(fields[0], int(percent), 0, int(maxUses), expiresAt)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrDiscountExists):
			b.send(chatId, msgDiscountExists, b.adminMenu())
		case errors.Is(err, service.ErrDiscountInvalid):
			b.send(chatId, msgDiscountPercentBad, b.adminMenu())
		default:
			b.send(chatId, msgSomethingWrong, b.adminMenu())
		}
		return
	}
	b.send(chatId, msgDiscountCreated, b.adminMenu())
	b.showDiscountMenu(chatId, code.Id)
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

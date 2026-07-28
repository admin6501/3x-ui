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
	// The two halves of naming a new config: choosing how it is named, and — if
	// the buyer wants to choose — typing the name.
	stepConfigNameChoice = "config_name_choice"
	stepConfigName       = "config_name"
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
		tu.InlineKeyboardRow(tu.InlineKeyboardButton(b.m(btnJoinChannel)).WithURL(link)),
		tu.InlineKeyboardRow(tu.InlineKeyboardButton(b.m(btnJoined)).WithCallbackData("joined")),
	)
	b.send(chatId, msgMustJoin+"\n\n"+esc(name), kb)
}

// -------------------------------------------------------------- screens --

func (b *Bot) shopMenu(chatId int64) telego.ReplyMarkup {
	rows := [][]telego.KeyboardButton{
		{tu.KeyboardButton(b.m(btnWallet)), tu.KeyboardButton(b.m(btnBuyCfg))},
		{tu.KeyboardButton(b.m(btnMyCfgs)), tu.KeyboardButton(b.m(btnLedger))},
		{tu.KeyboardButton(b.m(btnPrices)), tu.KeyboardButton(b.m(btnSupport))},
		{tu.KeyboardButton(b.m(btnHelp))},
	}
	if b.isAdmin(chatId) {
		rows = append(rows, []telego.KeyboardButton{tu.KeyboardButton(b.m(btnAdmin))})
	}
	return tu.Keyboard(rows...).WithResizeKeyboard()
}

func (b *Bot) currency() string {
	c, _ := b.settingService.GetSalesBotCurrency()
	return c
}

func (b *Bot) showWallet(chatId int64) {
	tt := b.t()
	user, err := b.shopService.User(chatId, "", "")
	if err != nil {
		b.send(chatId, b.m(msgSomethingWrong), b.shopMenu(chatId))
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
		tu.InlineKeyboardButton(b.m(btnTopUp)).WithCallbackData("topup"),
	))
	b.send(chatId, tt.walletCard(user.Balance, user.TotalPaid, user.TotalSpent, b.currency(), active), kb)
}

func (b *Bot) showPrices(chatId int64) {
	tt := b.t()
	perGB, _ := b.settingService.GetShopPricePerGB()
	perDay, _ := b.settingService.GetShopPricePerDay()
	minTop, _ := b.settingService.GetShopMinTopUp()
	maxTop, _ := b.settingService.GetShopMaxTopUp()
	minBal, _ := b.settingService.GetShopMinBalance()
	maxVol, _ := b.settingService.GetShopMaxVolumeGB()
	b.send(chatId, tt.priceCard(perGB, perDay, b.currency(), minTop, maxTop, minBal, maxVol), b.shopMenu(chatId))
}

func (b *Bot) askTopUpAmount(chatId int64) {
	minTop, _ := b.settingService.GetShopMinTopUp()
	maxTop, _ := b.settingService.GetShopMaxTopUp()
	tt := b.t()
	prompt := tt.s(msgAskTopUp)
	if minTop > 0 || maxTop > 0 {
		prompt += "\n\n"
		if minTop > 0 {
			prompt += tt.f("msg.minTopUpIs", tt.num(minTop), esc(b.currency())) + "\n"
		}
		if maxTop > 0 {
			prompt += tt.f("msg.maxTopUpIs", tt.num(maxTop), esc(b.currency()))
		}
	}
	b.states.set(chatId, &state{step: stepTopUpAmount})
	b.send(chatId, prompt, b.cancelKeyboard())
}

func (b *Bot) startTopUp(chatId int64, name string, amount int64) {
	row, err := b.shopService.RequestTopUp(chatId, name, amount)
	if err != nil {
		switch err {
		case service.ErrTopUpTooSmall, service.ErrTopUpTooLarge:
			b.send(chatId, b.m(msgAmountOutOfRange), b.shopMenu(chatId))
		default:
			b.send(chatId, b.m(msgSomethingWrong), b.shopMenu(chatId))
		}
		b.states.clear(chatId)
		return
	}
	b.showTopUpInstructions(chatId, row)
}

// showTopUpInstructions is the screen a buyer sits on until they send a receipt:
// what to pay, where, and the option to attach a discount code first.
func (b *Bot) showTopUpInstructions(chatId int64, row *model.WalletTopUp) {
	tt := b.t()
	payText, _ := b.settingService.GetSalesBotPayText()
	b.states.set(chatId, &state{step: stepTopUpReceipt, orderId: row.Id})

	buttons := []telego.InlineKeyboardButton{}
	if b.discountsOffered() && row.DiscountCode == "" {
		buttons = append(buttons, tu.InlineKeyboardButton(b.m(btnHaveCode)).WithCallbackData(fmt.Sprintf("code:%d", row.Id)))
	}
	buttons = append(buttons, tu.InlineKeyboardButton(b.m(btnCancel)).WithCallbackData("topupcancel"))

	bonus := int64(0)
	if row.DiscountCode != "" {
		_, bonus, _ = b.shopService.ValidateDiscount(row.DiscountCode, chatId, row.Amount)
	}
	b.send(chatId,
		tt.topUpInstructions(row.Id, row.Amount, b.currency(), payText, row.DiscountCode, bonus),
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
		b.send(chatId, b.m(msgOrderGone), b.shopMenu(chatId))
		return
	}
	b.states.set(chatId, &state{step: stepDiscountCode, orderId: topUpId})
	b.send(chatId, b.m(msgAskDiscountCode), b.cancelKeyboard())
}

// applyDiscountCode validates what the buyer typed and attaches it to the
// top-up. A bad code puts them back on the same step rather than dropping them
// out of the flow.
func (b *Bot) applyDiscountCode(chatId int64, topUpId int, typed string) {
	tt := b.t()
	row, err := b.shopService.GetTopUp(topUpId)
	if err != nil || row.TelegramId != chatId {
		b.states.clear(chatId)
		b.send(chatId, b.m(msgOrderGone), b.shopMenu(chatId))
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
		b.send(chatId, b.m(msgOrderGone), b.shopMenu(chatId))
		return
	}
	b.send(chatId, tt.f(msgDiscountApplied,
		esc(code.Code), tt.percent(int64(code.Percent)), tt.num(bonus), esc(b.currency()),
		tt.num(row.Amount+bonus), esc(b.currency())))
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
		b.send(chatId, b.m(msgOrderGone), b.shopMenu(chatId))
		return
	}
	b.states.clear(chatId)
	b.send(chatId, b.m(msgTopUpSent), b.shopMenu(chatId))

	who := strings.TrimSpace(msg.From.FirstName + " " + msg.From.LastName)
	if msg.From.Username != "" {
		who += " (@" + msg.From.Username + ")"
	}
	tt := b.t()
	caption := tt.f("msg.topUpRequest",
		tt.num(int64(row.Id)), esc(who), row.TelegramId, tt.num(row.Amount), esc(b.currency()))
	// The admin decides on the code as much as on the payment, so it belongs in
	// the message they approve from.
	if row.DiscountCode != "" {
		if code, bonus, err := b.shopService.ValidateDiscount(row.DiscountCode, row.TelegramId, row.Amount); err == nil {
			caption += "\n" + tt.f("msg.topUpDiscountLine",
				esc(code.Code), tt.percent(int64(code.Percent)), tt.num(bonus), esc(b.currency()),
				tt.num(row.Amount+bonus), esc(b.currency()))
		} else {
			caption += "\n" + tt.f("msg.discountNoLongerValid", esc(row.DiscountCode))
		}
	}
	kb := tu.InlineKeyboard(tu.InlineKeyboardRow(
		tu.InlineKeyboardButton(b.m(btnApprove)).WithCallbackData(fmt.Sprintf("topok:%d", row.Id)),
		tu.InlineKeyboardButton(b.m(btnReject)).WithCallbackData(fmt.Sprintf("topno:%d", row.Id)),
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
		b.send(chatId, b.m(msgSomethingWrong), b.shopMenu(chatId))
		return
	}
	if user.Blocked {
		b.send(chatId, b.m(msgBlocked), b.shopMenu(chatId))
		return
	}
	minBal, _ := b.settingService.GetShopMinBalance()
	if user.Balance <= 0 || user.Balance < minBal {
		kb := tu.InlineKeyboard(tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(b.m(btnTopUp)).WithCallbackData("topup"),
		))
		b.send(chatId, b.m(msgNeedBalance), kb)
		return
	}
	perGB, _ := b.settingService.GetShopPricePerGB()
	maxVol, _ := b.settingService.GetShopMaxVolumeGB()
	tt := b.t()
	prompt := tt.s(msgAskVolume)
	if perGB > 0 {
		prompt += "\n\n" + tt.f("msg.volumeHint", tt.num(perGB), esc(b.currency()), tt.quota(user.Balance/perGB))
	}
	if maxVol > 0 {
		prompt += "\n" + tt.f("msg.maxVolumeHint", tt.quota(maxVol))
	}
	b.states.set(chatId, &state{step: stepBuyVolume})
	b.send(chatId, prompt, b.cancelKeyboard())
}

// askConfigName is the step between naming a size and getting the config. A
// buyer who does not care presses one button; one who does gets to choose,
// rather than being handed a name the shop invented for them.
func (b *Bot) askConfigName(chatId int64, volumeGB int64) {
	b.states.set(chatId, &state{step: stepConfigNameChoice, volumeGB: volumeGB})
	kb := tu.InlineKeyboard(tu.InlineKeyboardRow(
		tu.InlineKeyboardButton(b.m(btnNameAuto)).WithCallbackData("nameauto"),
		tu.InlineKeyboardButton(b.m(btnNameCustom)).WithCallbackData("namecustom"),
	))
	b.send(chatId, b.m(msgAskNameChoice), kb)
}

func (b *Bot) createConfig(chatId int64, volumeGB int64, name string) {
	cfg, err := b.shopService.CreateConfig(b.inboundService, chatId, volumeGB, name)
	if err != nil {
		switch err {
		case service.ErrInsufficientFund:
			b.send(chatId, b.m(msgNeedBalance), b.shopMenu(chatId))
		case service.ErrVolumeTooLarge:
			maxVol, _ := b.settingService.GetShopMaxVolumeGB()
			b.send(chatId, b.t().f("msg.maxVolumeIs", b.t().quota(maxVol)), b.shopMenu(chatId))
		case service.ErrVolumeInvalid:
			b.send(chatId, b.m(msgVolumeBad), b.shopMenu(chatId))
		case service.ErrNameInvalid:
			b.send(chatId, b.m(msgNameInvalid), b.shopMenu(chatId))
		case service.ErrNameTaken:
			b.send(chatId, b.m(msgNameTaken), b.shopMenu(chatId))
		case service.ErrNoShopInbound:
			b.send(chatId, b.m(msgShopNoInbound), b.shopMenu(chatId))
		case service.ErrUserBlocked:
			b.send(chatId, b.m(msgBlocked), b.shopMenu(chatId))
		default:
			b.send(chatId, b.m(msgSomethingWrong), b.shopMenu(chatId))
		}
		return
	}
	b.send(chatId, b.m(msgConfigCreated), b.shopMenu(chatId))
	b.sendConfig(chatId, cfg, 0, 0)
}

// subLink builds the subscription URL a client app is pointed at, from the
// panel's own subscription settings and nothing else — the bot has no address of
// its own to offer. It prefers an explicitly configured subscription URI and
// otherwise derives one the way the subscription server does, including its
// path, without which the URL points at the server's root and serves nothing.
// A panel with the subscription server switched off, or with no domain set at
// all, has no such URL to give.
func (b *Bot) subLink(cfg *model.BotConfig) string {
	if cfg.SubID == "" {
		return ""
	}
	if on, err := b.settingService.GetSubEnable(); err == nil && !on {
		return ""
	}
	base, _ := b.settingService.GetSubURI()
	if strings.TrimSpace(base) == "" {
		host := b.settingService.PublicHost()
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

// configLinks returns the client's direct share links (vless://…) exactly as the
// panel builds them for its own share/QR button. The empty host is deliberate:
// there is no request to take one from, so the panel resolves the address the
// way it always does — the inbound's node address, its listen, or its custom
// share address, then the Sub/Web domain. The bot has no business substituting
// an address of its own.
//
// Unlike the subscription URL these need no subscription server configured at
// all, which makes them the shop's primary deliverable.
func (b *Bot) configLinks(cfg *model.BotConfig) []string {
	if b.inboundService == nil {
		return nil
	}
	links, err := b.inboundService.GetAllClientLinks("", cfg.Email)
	if err != nil {
		logger.Warning("shop: could not build config links for", cfg.Email, err)
		return nil
	}
	// A panel that can name no address for the inbound produces links with an
	// empty host — a dead link is worse than none, because nothing flags it.
	out := links[:0]
	for _, link := range links {
		if linkHasHost(link) {
			out = append(out, link)
		}
	}
	if len(out) < len(links) {
		logger.Warning("shop: dropped address-less links for", cfg.Email,
			"— set the inbound's address, or the panel's Sub/Web domain")
	}
	return out
}

// linkHasHost reports whether a share link actually names a server. "vless://id@"
// with nothing after the "@", or "ss://…@:443", is what the builder emits when
// no address could be resolved.
func linkHasHost(link string) bool {
	_, rest, ok := strings.Cut(link, "://")
	if !ok {
		return false
	}
	if at := strings.LastIndex(rest, "@"); at >= 0 {
		rest = rest[at+1:]
	}
	// Trim anything after the authority.
	for _, sep := range []string{"/", "?", "#"} {
		rest, _, _ = strings.Cut(rest, sep)
	}
	host, _, _ := strings.Cut(rest, ":")
	return strings.TrimSpace(host) != ""
}

// sendConfig delivers one config: its usage card, then the links themselves.
// A config with no deliverable link is a sale that gave the buyer nothing, so
// that case says so out loud rather than sending a card and going quiet.
func (b *Bot) sendConfig(chatId int64, cfg *model.BotConfig, usedBytes, cost int64) {
	tt := b.t()
	sub := b.subLink(cfg)
	b.send(chatId, tt.configCard(cfg.Email, cfg.VolumeGB, usedBytes, cost, cfg.Active, b.currency(), sub))
	b.sendLinks(chatId, cfg, sub)
}

// sendLinks delivers the direct config links, or explains their absence. A
// config with no deliverable link is a sale that gave the buyer nothing, so that
// case says so out loud rather than going quiet.
func (b *Bot) sendLinks(chatId int64, cfg *model.BotConfig, sub string) {
	links := b.configLinks(cfg)
	if len(links) == 0 && sub == "" {
		b.send(chatId, b.m(msgNoLinkYet))
		for _, adminId := range b.admins() {
			b.send(adminId, b.t().f("msg.noLinkAdmin", esc(cfg.Email)))
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
	tt := b.t()
	configs, err := b.shopService.ListConfigs(chatId)
	if err != nil || len(configs) == 0 {
		b.send(chatId, b.m(msgNoConfigs), b.shopMenu(chatId))
		return
	}
	rows := make([][]telego.InlineKeyboardButton, 0, len(configs))
	for i := range configs {
		cfg := &configs[i]
		rows = append(rows, tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(configButtonLabel(tt, cfg)).WithCallbackData(fmt.Sprintf("cfg:%d", cfg.Id)),
		))
	}
	b.send(chatId, b.m(msgPickConfig), tu.InlineKeyboard(rows...))
}

// configButtonLabel names a config in the list: its state, its name and its size,
// which is everything needed to tell two of them apart at a glance.
func configButtonLabel(tt *tr, cfg *model.BotConfig) string {
	mark := "🟢"
	switch {
	case cfg.Paused:
		mark = "⏸"
	case !cfg.Active:
		mark = "⛔️"
	}
	return fmt.Sprintf("%s %s — %s", mark, cfg.Email, tt.quota(cfg.VolumeGB))
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
	tt := b.t()
	cfg := b.ownedConfig(chatId, id)
	if cfg == nil {
		b.send(chatId, b.m(msgConfigGone), b.shopMenu(chatId))
		return
	}
	usage := b.shopService.Usage(cfg)
	body := tt.configDetailCard(cfg, usage.UsedBytes, cfg.ChargedTraffic+cfg.ChargedDays, b.currency())

	toggle := btnCfgPause
	if cfg.Paused {
		toggle = btnCfgResume
	}
	kb := tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(b.m(btnCfgLinks)).WithCallbackData(fmt.Sprintf("cfglink:%d", cfg.Id)),
			tu.InlineKeyboardButton(b.m(btnCfgAddVol)).WithCallbackData(fmt.Sprintf("cfgvol:%d", cfg.Id)),
		),
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(toggle).WithCallbackData(fmt.Sprintf("cfgtog:%d", cfg.Id)),
			tu.InlineKeyboardButton(b.m(btnCfgDelete)).WithCallbackData(fmt.Sprintf("cfgdel:%d", cfg.Id)),
		),
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(b.m(btnCfgBack)).WithCallbackData("cfglist"),
		),
	)
	b.send(chatId, body, kb)
}

// toggleConfig pauses or resumes a config on its owner's request.
func (b *Bot) toggleConfig(chatId int64, id int) {
	cfg := b.ownedConfig(chatId, id)
	if cfg == nil {
		b.send(chatId, b.m(msgConfigGone), b.shopMenu(chatId))
		return
	}
	updated, err := b.shopService.SetConfigPaused(b.inboundService, id, !cfg.Paused)
	if err != nil {
		b.send(chatId, b.m(msgSomethingWrong), b.shopMenu(chatId))
		return
	}
	switch {
	case updated.Paused:
		b.send(chatId, b.m(msgConfigPaused))
	case updated.Active:
		b.send(chatId, b.m(msgConfigResumed))
	default:
		// Un-paused but still off: the wallet cannot pay for it yet.
		b.send(chatId, b.m(msgConfigNeedsFunds))
	}
	b.showConfigMenu(chatId, id)
}

// askAddVolume starts the add-volume step for one config.
func (b *Bot) askAddVolume(chatId int64, id int) {
	if b.ownedConfig(chatId, id) == nil {
		b.send(chatId, b.m(msgConfigGone), b.shopMenu(chatId))
		return
	}
	b.states.set(chatId, &state{step: stepAddVolume, configId: id})
	b.send(chatId, b.m(msgAskAddVolume), b.cancelKeyboard())
}

// addVolume applies the number the owner typed at the add-volume step.
func (b *Bot) addVolume(chatId int64, id int, extraGB int64) {
	tt := b.t()
	b.states.clear(chatId)
	if b.ownedConfig(chatId, id) == nil {
		b.send(chatId, b.m(msgConfigGone), b.shopMenu(chatId))
		return
	}
	cfg, err := b.shopService.AddVolume(b.inboundService, id, extraGB)
	if err != nil {
		switch err {
		case service.ErrVolumeTooLarge:
			maxVol, _ := b.settingService.GetShopMaxVolumeGB()
			b.send(chatId, tt.f("msg.maxVolumePerConfig", tt.quota(maxVol)), b.shopMenu(chatId))
		case service.ErrVolumeInvalid:
			b.send(chatId, b.m(msgVolumeBad), b.shopMenu(chatId))
		case service.ErrNameInvalid:
			b.send(chatId, b.m(msgNameInvalid), b.shopMenu(chatId))
		case service.ErrNameTaken:
			b.send(chatId, b.m(msgNameTaken), b.shopMenu(chatId))
		default:
			b.send(chatId, b.m(msgSomethingWrong), b.shopMenu(chatId))
		}
		return
	}
	b.send(chatId, tt.f("msg.volumeNow", tt.quota(cfg.VolumeGB)), b.shopMenu(chatId))
	b.showConfigMenu(chatId, id)
}

// confirmDeleteConfig asks before destroying a config, because the button sits
// next to the ones a buyer presses routinely.
func (b *Bot) confirmDeleteConfig(chatId int64, id int) {
	tt := b.t()
	cfg := b.ownedConfig(chatId, id)
	if cfg == nil {
		b.send(chatId, b.m(msgConfigGone), b.shopMenu(chatId))
		return
	}
	kb := tu.InlineKeyboard(tu.InlineKeyboardRow(
		tu.InlineKeyboardButton(b.m(btnCfgDeleteYes)).WithCallbackData(fmt.Sprintf("cfgdelok:%d", cfg.Id)),
		tu.InlineKeyboardButton(b.m(btnCfgBack)).WithCallbackData(fmt.Sprintf("cfg:%d", cfg.Id)),
	))
	b.send(chatId, tt.f(msgConfirmDelete, esc(cfg.Email)), kb)
}

func (b *Bot) deleteConfig(chatId int64, id int) {
	if b.ownedConfig(chatId, id) == nil {
		b.send(chatId, b.m(msgConfigGone), b.shopMenu(chatId))
		return
	}
	if err := b.shopService.DeleteConfig(b.inboundService, id); err != nil {
		b.send(chatId, b.m(msgSomethingWrong), b.shopMenu(chatId))
		return
	}
	b.send(chatId, b.m(msgConfigDeleted), b.shopMenu(chatId))
}

// sendConfigLinks re-sends one config's links on request.
func (b *Bot) sendConfigLinks(chatId int64, id int) {
	cfg := b.ownedConfig(chatId, id)
	if cfg == nil {
		b.send(chatId, b.m(msgConfigGone), b.shopMenu(chatId))
		return
	}
	b.sendLinks(chatId, cfg, b.subLink(cfg))
}

func (b *Bot) showLedger(chatId int64) {
	tt := b.t()
	entries, err := b.shopService.Transactions(chatId, 15)
	if err != nil || len(entries) == 0 {
		b.send(chatId, b.m(msgNoLedger), b.shopMenu(chatId))
		return
	}
	var body strings.Builder
	body.WriteString(tt.s("msg.ledgerTitle") + "\n\n")
	for _, e := range entries {
		body.WriteString(tt.txLine(e.Amount, e.Balance, e.Kind, e.Details, b.currency()))
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
		b.send(id, b.m(msgSuspended), b.shopMenu(id))
	}
}

// NotifyDeleted tells owners that the clean-up sweep removed a config of theirs.
// A config disappearing without a word would read as the shop having lost it.
func (b *Bot) NotifyDeleted(deleted []service.DeletedConfig) {
	if !b.IsRunning() {
		return
	}
	tt := b.t()
	for _, d := range deleted {
		b.send(d.TelegramId, tt.f("msg.configSwept", esc(d.Email)), b.shopMenu(d.TelegramId))
	}
}

// -------------------------------------------------------- admin screens --

func (b *Bot) showTopUpQueue(chatId int64) {
	tt := b.t()
	rows, err := b.shopService.ListTopUps(model.TopUpReview, 20)
	if err != nil || len(rows) == 0 {
		b.send(chatId, b.m(msgNoPendingTopUps), b.adminMenu())
		return
	}
	for _, row := range rows {
		caption := tt.f("msg.topUpQueueItem",
			tt.num(int64(row.Id)), esc(row.TelegramName), row.TelegramId,
			tt.num(row.Amount), esc(b.currency()))
		if row.DiscountCode != "" {
			if code, bonus, err := b.shopService.ValidateDiscount(row.DiscountCode, row.TelegramId, row.Amount); err == nil {
				caption += "\n" + tt.f("msg.discountQueueLine",
					esc(code.Code), tt.percent(int64(code.Percent)), tt.num(bonus), esc(b.currency()))
			}
		}
		kb := tu.InlineKeyboard(tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(b.m(btnApprove)).WithCallbackData(fmt.Sprintf("topok:%d", row.Id)),
			tu.InlineKeyboardButton(b.m(btnReject)).WithCallbackData(fmt.Sprintf("topno:%d", row.Id)),
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
		b.send(adminId, b.m(msgAlreadyDecided))
		return
	}
	// Paying puts the user's configs back on without waiting for the next
	// billing tick.
	b.shopService.BillAll(b.inboundService)
	tt := b.t()
	text := tt.f("msg.walletCredited", tt.num(row.Amount), esc(b.currency()))
	if row.Bonus > 0 {
		text += "\n" + tt.f("msg.walletBonus", esc(row.DiscountCode), tt.num(row.Bonus), esc(b.currency()))
	}
	text += "\n\n" + tt.f("msg.newBalance", tt.num(balance), esc(b.currency()))
	b.send(row.TelegramId, text, b.shopMenu(row.TelegramId))

	adminNote := tt.f("msg.topUpApproved", tt.num(int64(id)))
	if row.DiscountCode != "" && row.Bonus == 0 {
		adminNote += "\n" + tt.f("msg.discountNotApplied", esc(row.DiscountCode))
	}
	b.send(adminId, adminNote)
}

func (b *Bot) rejectTopUp(adminId int64, id int, note string) {
	row, err := b.shopService.RejectTopUp(id, note)
	if err != nil {
		b.send(adminId, b.m(msgAlreadyDecided))
		return
	}
	tt := b.t()
	text := tt.f("msg.topUpRejected", tt.num(int64(row.Id)))
	if strings.TrimSpace(note) != "" {
		text += "\n" + tt.f("msg.rejectReason", esc(note))
	}
	b.send(row.TelegramId, text, b.shopMenu(row.TelegramId))
	b.send(adminId, tt.f("msg.topUpRejectedAdmin", tt.num(int64(id))), b.adminMenu())
}

// showShopUsers lists shop users by name, one button each. Dumping every user's
// figures and two action buttons into a single message made the list unreadable
// and the buttons impossible to match to a row; picking a name opens that user's
// own screen instead.
func (b *Bot) showShopUsers(chatId int64) {
	tt := b.t()
	users, err := b.shopService.ListUsers(30)
	if err != nil || len(users) == 0 {
		b.send(chatId, b.m(msgNoShopUsers), b.adminMenu())
		return
	}
	rows := make([][]telego.InlineKeyboardButton, 0, len(users))
	for i := range users {
		u := &users[i]
		rows = append(rows, tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(userButtonLabel(tt, u, b.currency())).
				WithCallbackData(fmt.Sprintf("usr:%d", u.TelegramId)),
		))
	}
	b.send(chatId, b.m(msgPickUser), tu.InlineKeyboard(rows...))
}

// userButtonLabel names a shop user in the list: state, who they are, and the
// balance — the number an admin is nearly always looking for.
func userButtonLabel(tt *tr, u *model.BotUser, currency string) string {
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
	return fmt.Sprintf("%s %s — %s %s", mark, name, tt.num(u.Balance), currency)
}

// showUserMenu is one shop user's own screen: their wallet, their configs and
// every action an admin can take on them.
func (b *Bot) showUserMenu(adminId int64, telegramId int64) {
	tt := b.t()
	u, err := b.shopService.GetUser(telegramId)
	if err != nil {
		b.send(adminId, b.m(msgUserGone), b.adminMenu())
		return
	}
	configs, _ := b.shopService.ListConfigs(telegramId)
	pending := b.shopService.CountPendingTopUpsOf(telegramId)

	block := b.m(btnBlock)
	if u.Blocked {
		block = b.m(btnUnblock)
	}
	kb := tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(b.m(btnAdjust)).WithCallbackData(fmt.Sprintf("adj:%d", u.TelegramId)),
			tu.InlineKeyboardButton(block).WithCallbackData(fmt.Sprintf("blk:%d", u.TelegramId)),
		),
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(b.m(btnUserConfigs)).WithCallbackData(fmt.Sprintf("usrcfg:%d", u.TelegramId)),
			tu.InlineKeyboardButton(b.m(btnUserLedger)).WithCallbackData(fmt.Sprintf("usrtx:%d", u.TelegramId)),
		),
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(b.m(btnCfgBack)).WithCallbackData("usrlist"),
		),
	)
	b.send(adminId, tt.userDetailCard(u, len(configs), pending, b.currency()), kb)
}

// showUserConfigs is the admin's view of one user's configs.
func (b *Bot) showUserConfigs(adminId int64, telegramId int64) {
	tt := b.t()
	configs, err := b.shopService.ListConfigs(telegramId)
	if err != nil || len(configs) == 0 {
		b.send(adminId, b.m(msgUserNoConfigs))
		return
	}
	var body strings.Builder
	body.WriteString(tt.f("msg.userConfigsTitle", telegramId) + "\n\n")
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
		body.WriteString(mark + " " + tt.f("msg.configLine", esc(cfg.Email),
			tt.bytes(usage.UsedBytes), tt.quota(cfg.VolumeGB),
			tt.num(cfg.ChargedTraffic+cfg.ChargedDays), esc(b.currency())) + "\n\n")
	}
	b.send(adminId, body.String())
}

// showUserLedger is the admin's view of one user's transactions.
func (b *Bot) showUserLedger(adminId int64, telegramId int64) {
	tt := b.t()
	entries, err := b.shopService.Transactions(telegramId, 15)
	if err != nil || len(entries) == 0 {
		b.send(adminId, b.m(msgUserNoTx))
		return
	}
	var body strings.Builder
	body.WriteString(tt.f("msg.userTxTitle", telegramId) + "\n\n")
	for _, e := range entries {
		body.WriteString(tt.txLine(e.Amount, e.Balance, e.Kind, e.Details, b.currency()))
		body.WriteString("\n\n")
	}
	b.send(adminId, body.String())
}

// ------------------------------------------------------ discount codes --

func (b *Bot) showDiscounts(chatId int64) {
	tt := b.t()
	codes, err := b.shopService.ListDiscounts(30)
	if err != nil {
		b.send(chatId, b.m(msgSomethingWrong), b.adminMenu())
		return
	}
	rows := make([][]telego.InlineKeyboardButton, 0, len(codes)+1)
	for i := range codes {
		c := &codes[i]
		rows = append(rows, tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(discountButtonLabel(tt, c)).WithCallbackData(fmt.Sprintf("dsc:%d", c.Id)),
		))
	}
	rows = append(rows, tu.InlineKeyboardRow(
		tu.InlineKeyboardButton(b.m(btnNewCode)).WithCallbackData("dscnew"),
	))
	body := msgNoDiscounts
	if len(codes) > 0 {
		body = msgPickDiscount
	}
	b.send(chatId, body, tu.InlineKeyboard(rows...))
}

func discountButtonLabel(tt *tr, c *model.DiscountCode) string {
	mark := "🟢"
	switch {
	case !c.Enabled:
		mark = "⛔️"
	case c.ExpiresAt > 0 && nowMilli() > c.ExpiresAt:
		mark = "⌛️"
	case c.MaxUses > 0 && c.Used >= c.MaxUses:
		mark = "🔚"
	}
	return fmt.Sprintf("%s %s — %s", mark, c.Code, tt.percent(int64(c.Percent)))
}

func (b *Bot) showDiscountMenu(chatId int64, id int) {
	tt := b.t()
	c, err := b.shopService.GetDiscount(id)
	if err != nil {
		b.send(chatId, b.m(msgDiscountGone), b.adminMenu())
		return
	}
	toggle := b.m(btnDisable)
	if !c.Enabled {
		toggle = b.m(btnEnable)
	}
	kb := tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(toggle).WithCallbackData(fmt.Sprintf("dsctog:%d", c.Id)),
			tu.InlineKeyboardButton(b.m(btnCfgDelete)).WithCallbackData(fmt.Sprintf("dscdel:%d", c.Id)),
		),
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(b.m(btnCfgBack)).WithCallbackData("dsclist"),
		),
	)
	b.send(chatId, tt.discountCard(c, b.currency()), kb)
}

func (b *Bot) toggleDiscount(chatId int64, id int) {
	c, err := b.shopService.GetDiscount(id)
	if err != nil {
		b.send(chatId, b.m(msgDiscountGone), b.adminMenu())
		return
	}
	if _, err := b.shopService.SetDiscountEnabled(id, !c.Enabled); err != nil {
		b.send(chatId, b.m(msgSomethingWrong), b.adminMenu())
		return
	}
	b.showDiscountMenu(chatId, id)
}

func (b *Bot) deleteDiscount(chatId int64, id int) {
	if err := b.shopService.DeleteDiscount(id); err != nil {
		b.send(chatId, b.m(msgSomethingWrong), b.adminMenu())
		return
	}
	b.send(chatId, b.m(msgDiscountDeleted))
	b.showDiscounts(chatId)
}

// askNewDiscount starts the create-a-code wizard. It is one free-text answer:
// asking four questions in a row for something an owner types once is worse
// than one line with a documented shape.
func (b *Bot) askNewDiscount(chatId int64) {
	b.states.set(chatId, &state{step: stepNewDiscount})
	b.send(chatId, b.m(msgAskNewDiscount), b.cancelKeyboard())
}

// createDiscount parses the wizard's one line: CODE PERCENT [MAX_USES] [DAYS].
func (b *Bot) createDiscount(chatId int64, line string) {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) < 2 {
		b.send(chatId, b.m(msgDiscountFormatBad))
		return
	}
	percent, ok := parseNumber(fields[1])
	if !ok || percent <= 0 || percent > 100 {
		b.send(chatId, b.m(msgDiscountPercentBad))
		return
	}
	maxUses := int64(0)
	if len(fields) >= 3 {
		if maxUses, ok = parseNumber(fields[2]); !ok {
			b.send(chatId, b.m(msgDiscountFormatBad))
			return
		}
	}
	expiresAt := int64(0)
	if len(fields) >= 4 {
		days, ok := parseNumber(fields[3])
		if !ok {
			b.send(chatId, b.m(msgDiscountFormatBad))
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
			b.send(chatId, b.m(msgDiscountExists), b.adminMenu())
		case errors.Is(err, service.ErrDiscountInvalid):
			b.send(chatId, b.m(msgDiscountPercentBad), b.adminMenu())
		default:
			b.send(chatId, b.m(msgSomethingWrong), b.adminMenu())
		}
		return
	}
	b.send(chatId, b.m(msgDiscountCreated), b.adminMenu())
	b.showDiscountMenu(chatId, code.Id)
}

func (b *Bot) showAllConfigs(chatId int64) {
	tt := b.t()
	configs, err := b.shopService.ListAllConfigs(20)
	if err != nil || len(configs) == 0 {
		b.send(chatId, b.m(msgNoConfigsYet), b.adminMenu())
		return
	}
	var body strings.Builder
	body.WriteString(tt.s("msg.allConfigsTitle") + "\n\n")
	for i := range configs {
		cfg := &configs[i]
		usage := b.shopService.Usage(cfg)
		mark := "🟢"
		if !cfg.Active {
			mark = "⛔️"
		}
		body.WriteString(mark + " " + tt.f("msg.allConfigLine", esc(cfg.Email), cfg.TelegramId,
			tt.bytes(usage.UsedBytes), tt.quota(cfg.VolumeGB),
			tt.num(cfg.ChargedTraffic+cfg.ChargedDays), esc(b.currency())) + "\n\n")
	}
	b.send(chatId, body.String(), b.adminMenu())
}

func (b *Bot) showShopStats(chatId int64) {
	tt := b.t()
	stats := b.shopService.Stats()
	perGB, _ := b.settingService.GetShopPricePerGB()
	var body strings.Builder
	body.WriteString(tt.s("card.stats.title") + "\n\n")
	body.WriteString(tt.f("card.stats.users", tt.num(stats.Users)) + "\n")
	body.WriteString(tt.f("card.stats.configs", tt.num(stats.Configs), tt.num(stats.ActiveConfigs)) + "\n")
	body.WriteString(tt.f("card.stats.paid", tt.num(stats.TotalPaid), esc(b.currency())) + "\n")
	body.WriteString(tt.f("card.stats.spent", tt.num(stats.TotalSpent), esc(b.currency())) + "\n")
	body.WriteString(tt.f("card.stats.float", tt.num(stats.WalletBalance), esc(b.currency())) + "\n")
	body.WriteString(tt.f("card.stats.pending", tt.num(stats.PendingTopUps)) + "\n")
	body.WriteString(tt.f("card.stats.suspended", tt.num(stats.SuspendedUsers)) + "\n\n")
	body.WriteString(tt.f("card.stats.pricePerGB", tt.num(perGB), esc(b.currency())))
	b.send(chatId, body.String(), b.adminMenu())
}

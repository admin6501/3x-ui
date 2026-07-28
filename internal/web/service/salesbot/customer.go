package salesbot

import (
	"strconv"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
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
			b.send(chatId, b.m(msgSomethingWrong))
		}
	}()
	fn()
}

// idCard answers /id — buyers need their numeric id to be added as an admin,
// and it is the first thing a shop owner looks for when setting the bot up.
func (b *Bot) idCard(msg telego.Message) string {
	return b.t().f("msg.idCard", msg.Chat.ID)
}

// ------------------------------------------------------------ main menu --

func (b *Bot) mainMenu(chatId int64) telego.ReplyMarkup {
	return b.shopMenu(chatId)
}

func (b *Bot) onStart(msg telego.Message) {
	chatId := msg.Chat.ID
	b.states.clear(chatId)
	if _, err := b.shopService.User(chatId, msg.From.Username, msg.From.FirstName); err != nil {
		logger.Warning("shop: could not register user:", err)
	}
	if !b.requireChannel(chatId) {
		b.promptJoin(chatId)
		return
	}
	welcome, _ := b.settingService.GetSalesBotWelcome()
	if strings.TrimSpace(welcome) == "" {
		welcome = b.m(msgShopWelcome)
	} else {
		welcome = esc(welcome)
	}
	b.send(chatId, welcome, b.shopMenu(chatId))
}

// onMessage routes a plain message: first the conversation the chat is in,
// then the reply-keyboard buttons.
func (b *Bot) onMessage(msg telego.Message) {
	chatId := msg.Chat.ID

	// A receipt is a photo, and only means anything mid-top-up.
	if len(msg.Photo) > 0 {
		if st, ok := b.states.get(chatId); ok && st.step == stepTopUpReceipt {
			b.onTopUpReceipt(msg, st)
			return
		}
		b.send(chatId, b.m(msgReceiptFirst), b.shopMenu(chatId))
		return
	}

	if st, ok := b.states.get(chatId); ok {
		if b.handleConversation(msg, st) {
			return
		}
	}

	// The join gate stands in front of everything except the admin side: an
	// admin should still be able to run the shop without joining their own
	// channel.
	if !b.isAdmin(chatId) && !b.requireChannel(chatId) {
		b.promptJoin(chatId)
		return
	}

	switch buttonKeyFor(msg.Text) {
	case btnWallet:
		b.showWallet(chatId)
	case btnTopUp:
		b.askTopUpAmount(chatId)
	case btnBuyCfg:
		b.askVolume(chatId)
	case btnMyCfgs:
		b.showConfigs(chatId)
	case btnLedger:
		b.showLedger(chatId)
	case btnPrices:
		b.showPrices(chatId)
	case btnSupport:
		b.showSupport(chatId)
	case btnHelp:
		b.send(chatId, b.m(msgShopHelp), b.shopMenu(chatId))
	case btnMainMenu, btnBack:
		b.states.clear(chatId)
		b.send(chatId, b.m(msgMainMenu), b.shopMenu(chatId))
	case btnCancel:
		b.states.clear(chatId)
		b.send(chatId, b.m(msgCancelled), b.shopMenu(chatId))
	case btnAdmin:
		b.onAdminMenu(msg)
	case btnAdminTop, btnAdminUsr, btnAdminCfg, btnAdminStats, btnAdminCodes, btnAdminBroadcast, btnAdminExit:
		b.onAdminButton(msg)
	default:
		b.send(chatId, b.m(msgShopHelp), b.shopMenu(chatId))
	}
}

func (b *Bot) showSupport(chatId int64) {
	support, _ := b.settingService.GetSalesBotSupport()
	if strings.TrimSpace(support) == "" {
		b.send(chatId, b.m(msgNoSupport), b.mainMenu(chatId))
		return
	}
	b.send(chatId, b.t().f("msg.supportIs", esc(support)), b.mainMenu(chatId))
}

// ----------------------------------------------------------- callbacks --

func (b *Bot) onCallback(q telego.CallbackQuery) {
	chatId := q.From.ID
	parts := strings.Split(q.Data, ":")
	action := parts[0]

	switch action {
	case "joined":
		if b.requireChannel(chatId) {
			b.answer(q.ID, b.m(msgJoinOk))
			b.send(chatId, b.m(msgShopWelcome), b.shopMenu(chatId))
			return
		}
		b.answer(q.ID, b.m(msgNotJoinedYet))

	case "topup":
		b.answer(q.ID, "")
		b.askTopUpAmount(chatId)

	case "topupcancel":
		b.states.clear(chatId)
		b.answer(q.ID, b.m(msgCancelled))
		b.send(chatId, b.m(msgCancelled), b.shopMenu(chatId))

	case "cfglist":
		b.answer(q.ID, "")
		b.showConfigs(chatId)

	case "code":
		id, ok := callbackArg(parts)
		if !ok {
			b.answer(q.ID, b.m(msgSomethingWrong))
			return
		}
		b.answer(q.ID, "")
		b.askDiscountCode(chatId, id)

	// Every config action carries the config's id; ownership is re-checked on
	// the way in, so a guessed id reaches nothing.
	case "cfg", "cfglink", "cfgtog", "cfgvol", "cfgdel", "cfgdelok":
		id, ok := callbackArg(parts)
		if !ok {
			b.answer(q.ID, b.m(msgSomethingWrong))
			return
		}
		b.answer(q.ID, "")
		switch action {
		case "cfg":
			b.showConfigMenu(chatId, id)
		case "cfglink":
			b.sendConfigLinks(chatId, id)
		case "cfgtog":
			b.toggleConfig(chatId, id)
		case "cfgvol":
			b.askAddVolume(chatId, id)
		case "cfgdel":
			b.confirmDeleteConfig(chatId, id)
		case "cfgdelok":
			b.deleteConfig(chatId, id)
		}

	default:
		b.onAdminCallback(q)
	}
}

// callbackArg reads the numeric argument out of a "verb:id" callback.
func callbackArg(parts []string) (int, bool) {
	if len(parts) < 2 {
		return 0, false
	}
	id, err := strconv.Atoi(parts[1])
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

const bytesPerGB = 1024 * 1024 * 1024

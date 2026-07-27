package salesbot

import (
	"fmt"
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
		welcome = msgShopWelcome
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
		b.send(chatId, "برای ارسال رسید، ابتدا از «💰 کیف پول من» درخواست شارژ ثبت کنید.", b.shopMenu(chatId))
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

	switch strings.TrimSpace(msg.Text) {
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
		b.send(chatId, msgShopHelp, b.shopMenu(chatId))
	case btnMainMenu, btnBack:
		b.states.clear(chatId)
		b.send(chatId, "منوی اصلی 🏠", b.shopMenu(chatId))
	case btnCancel:
		b.states.clear(chatId)
		b.send(chatId, msgCancelled, b.shopMenu(chatId))
	case btnAdmin:
		b.onAdminMenu(msg)
	case btnAdminTop, btnAdminUsr, btnAdminCfg, btnAdminStats, btnAdminBroadcast, btnAdminExit:
		b.onAdminButton(msg)
	default:
		b.send(chatId, msgShopHelp, b.shopMenu(chatId))
	}
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
	case "joined":
		if b.requireChannel(chatId) {
			b.answer(q.ID, msgJoinOk)
			b.send(chatId, msgShopWelcome, b.shopMenu(chatId))
			return
		}
		b.answer(q.ID, msgNotJoinedYet)

	case "topup":
		b.answer(q.ID, "")
		b.askTopUpAmount(chatId)

	case "topupcancel":
		b.states.clear(chatId)
		b.answer(q.ID, msgCancelled)
		b.send(chatId, msgCancelled, b.shopMenu(chatId))

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

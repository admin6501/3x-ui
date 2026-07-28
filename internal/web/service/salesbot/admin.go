package salesbot

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

// Conversation steps. Each is a single question the bot is waiting an answer
// to; the reply advances to the next one.
const (
	stepRejectNote = "reject_note"
	stepBroadcast  = "broadcast"
)

func (b *Bot) adminMenu() telego.ReplyMarkup {
	return tu.Keyboard(
		[]telego.KeyboardButton{tu.KeyboardButton(btnAdminTop), tu.KeyboardButton(btnAdminUsr)},
		[]telego.KeyboardButton{tu.KeyboardButton(btnAdminCfg), tu.KeyboardButton(btnAdminStats)},
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
	pending := b.shopService.CountPendingTopUps()
	text := msgAdminWelcome
	if pending > 0 {
		text += fmt.Sprintf("\n\n🔔 <b>%s درخواست شارژ</b> در انتظار بررسی است.", faNum(pending))
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
	case btnAdminTop:
		b.showTopUpQueue(chatId)
	case btnAdminUsr:
		b.showShopUsers(chatId)
	case btnAdminCfg:
		b.showAllConfigs(chatId)
	case btnAdminStats:
		b.showShopStats(chatId)
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

func (b *Bot) broadcast(adminId int64, text string) {
	ids := b.shopService.ShopUserIds()
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
	arg := int64(0)
	if len(parts) > 1 {
		arg, _ = strconv.ParseInt(parts[1], 10, 64)
	}

	switch action {
	case "topok":
		b.answer(q.ID, "")
		b.approveTopUp(chatId, int(arg))
	case "topno":
		b.answer(q.ID, "")
		b.states.set(chatId, &state{step: stepRejectNote, orderId: int(arg)})
		b.send(chatId, msgAskRejectNote, tu.Keyboard(
			[]telego.KeyboardButton{tu.KeyboardButton(btnSkip), tu.KeyboardButton(btnCancel)},
		).WithResizeKeyboard())
	case "adj":
		b.answer(q.ID, "")
		b.states.set(chatId, &state{step: stepAdjustAmount, targetUser: arg})
		b.send(chatId, fmt.Sprintf(
			"مبلغ اصلاح موجودی کاربر <code>%d</code> را بنویسید.\nبرای کم‌کردن، عدد را با علامت منها بنویسید (مثلاً -50000):", arg),
			cancelKeyboard())
	case "blk":
		user, err := b.shopService.GetUser(arg)
		if err != nil {
			b.answer(q.ID, msgSomethingWrong)
			return
		}
		if err := b.shopService.SetBlocked(arg, !user.Blocked); err != nil {
			b.answer(q.ID, msgSomethingWrong)
			return
		}
		// Blocking has to take their configs down, not just stop them buying.
		b.shopService.BillAll(b.inboundService)
		b.answer(q.ID, "انجام شد")
		b.showShopUsers(chatId)
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
	case stepTopUpReceipt:
		// Only a photo advances this step; a stray message gets a nudge.
		b.send(chatId, msgReceiptOnlyPic)
		return true

	case stepTopUpAmount:
		amount, ok := parseNumber(text)
		if !ok {
			b.send(chatId, msgVolumeBad)
			return true
		}
		b.states.clear(chatId)
		b.startTopUp(chatId, strings.TrimSpace(msg.From.FirstName+" "+msg.From.LastName), amount)
		return true

	case stepBuyVolume:
		volume, ok := parseNumber(text)
		if !ok || volume <= 0 {
			b.send(chatId, msgVolumeBad)
			return true
		}
		b.states.clear(chatId)
		b.createConfig(chatId, volume)
		return true

	case stepAddVolume:
		volume, ok := parseNumber(text)
		if !ok || volume <= 0 {
			b.send(chatId, msgVolumeBad)
			return true
		}
		b.addVolume(chatId, st.configId, volume)
		return true

	case stepAdjustAmount:
		amount, ok := parseSignedNumber(text)
		if !ok {
			b.send(chatId, msgVolumeBad)
			return true
		}
		b.states.clear(chatId)
		balance, err := b.shopService.Adjust(st.targetUser, amount, "اصلاح توسط مدیر")
		if err != nil {
			b.send(chatId, msgSomethingWrong, b.adminMenu())
			return true
		}
		// A correction in either direction can switch configs on or off.
		b.shopService.BillAll(b.inboundService)
		b.send(chatId, fmt.Sprintf("موجودی کاربر <code>%d</code> اکنون <b>%s %s</b> است.",
			st.targetUser, faNum(balance), esc(b.currency())), b.adminMenu())
		b.send(st.targetUser, fmt.Sprintf("💰 موجودی کیف پول شما توسط مدیر به <b>%s %s</b> تغییر کرد.",
			faNum(balance), esc(b.currency())), b.shopMenu(st.targetUser))
		return true

	case stepRejectNote:
		note := text
		if text == btnSkip {
			note = ""
		}
		b.states.clear(chatId)
		b.rejectTopUp(chatId, st.orderId, note)
		return true

	case stepBroadcast:
		b.states.clear(chatId)
		b.broadcast(chatId, text)
		return true
	}
	return false
}

// parseSignedNumber is parseNumber that also accepts a leading minus, for an
// admin correcting a balance downwards.
func parseSignedNumber(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	negative := strings.HasPrefix(s, "-") || strings.HasPrefix(s, "\u2212")
	s = strings.TrimPrefix(strings.TrimPrefix(s, "-"), "\u2212")
	value, ok := parseNumber(s)
	if !ok {
		return 0, false
	}
	if negative {
		return -value, true
	}
	return value, true
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

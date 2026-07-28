package salesbot

import (
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// configCard is one config as its owner sees it just after buying it.
func (t *tr) configCard(email string, volumeGB, usedBytes, cost int64, active bool, currency, link string) string {
	var b strings.Builder
	mark := "🟢"
	if !active {
		mark = "⛔️"
	}
	b.WriteString(mark + " <b>" + esc(email) + "</b>\n")
	b.WriteString(t.f("card.config.usage", t.bytes(usedBytes), t.quota(volumeGB)) + "\n")
	if volumeGB > 0 {
		b.WriteString(t.progressBar(float64(usedBytes), float64(volumeGB)*bytesPerGB) + "\n")
	}
	b.WriteString(t.f("card.config.cost", t.num(cost), esc(currency)) + "\n")
	if strings.TrimSpace(link) != "" {
		b.WriteString("\n" + t.s("card.config.subLink") + "\n<code>" + esc(link) + "</code>")
	}
	return b.String()
}

// configDetailCard is one config's own screen — the fuller view behind a name in
// the config list, with the state spelled out rather than left to an icon.
func (t *tr) configDetailCard(cfg *model.BotConfig, usedBytes, cost int64, currency string) string {
	var b strings.Builder
	b.WriteString("📱 <b>" + esc(cfg.Email) + "</b>\n\n")

	switch {
	case cfg.Paused:
		b.WriteString(t.s("card.cfg.statusPaused") + "\n")
	case cfg.Active:
		b.WriteString(t.s("card.cfg.statusActive") + "\n")
	default:
		b.WriteString(t.s("card.cfg.statusOff") + "\n")
	}

	b.WriteString(t.f("card.cfg.volume", t.quota(cfg.VolumeGB)) + "\n")
	b.WriteString(t.f("card.cfg.used", t.bytes(usedBytes)) + "\n")
	if cfg.VolumeGB > 0 {
		b.WriteString(t.progressBar(float64(usedBytes), float64(cfg.VolumeGB)*bytesPerGB) + "\n")
		if remaining := cfg.VolumeGB*bytesPerGB - usedBytes; remaining > 0 {
			b.WriteString(t.f("card.cfg.remaining", t.bytes(remaining)) + "\n")
		} else {
			b.WriteString(t.s("card.cfg.exhausted") + "\n")
		}
	}
	b.WriteString("\n" + t.f("card.cfg.cost", t.num(cost), esc(currency)) + "\n")
	if cfg.CreatedAt > 0 {
		b.WriteString(t.f("card.cfg.created", t.date(cfg.CreatedAt)) + "\n")
	}
	return b.String()
}

// date renders a millisecond timestamp in the language's own numerals.
func (t *tr) date(ms int64) string {
	return t.digits(time.UnixMilli(ms).Format("2006-01-02"))
}

// userDetailCard is one shop user's own screen for an admin.
func (t *tr) userDetailCard(u *model.BotUser, configs int, pendingTopUps int64, currency string) string {
	var b strings.Builder
	name := strings.TrimSpace(u.FirstName)
	if u.Username != "" {
		name = strings.TrimSpace(name + " @" + u.Username)
	}
	if name == "" {
		name = t.s("card.user.noName")
	}
	b.WriteString("👤 <b>" + esc(name) + "</b>\n")
	b.WriteString(t.f("card.user.tgId", u.TelegramId) + "\n\n")
	if u.Blocked {
		b.WriteString(t.s("card.user.statusBlocked") + "\n")
	} else {
		b.WriteString(t.s("card.user.statusOk") + "\n")
	}
	b.WriteString(t.f("card.user.balance", t.num(u.Balance), esc(currency)) + "\n")
	b.WriteString(t.f("card.user.paid", t.num(u.TotalPaid), esc(currency)) + "\n")
	b.WriteString(t.f("card.user.spent", t.num(u.TotalSpent), esc(currency)) + "\n")
	b.WriteString(t.f("card.user.configs", t.num(int64(configs))) + "\n")
	if pendingTopUps > 0 {
		b.WriteString(t.f("card.user.pending", t.num(pendingTopUps)) + "\n")
	}
	if u.Balance <= 0 {
		b.WriteString("\n" + t.s("card.user.empty"))
	}
	return b.String()
}

// discountCard is one code's own screen for an admin.
func (t *tr) discountCard(c *model.DiscountCode, currency string) string {
	var b strings.Builder
	b.WriteString("🏷 <b>" + esc(c.Code) + "</b>\n\n")
	b.WriteString(t.f("card.discount.bonus", t.percent(int64(c.Percent))) + "\n")
	if c.MaxBonus > 0 {
		b.WriteString(t.f("card.discount.maxBonus", t.num(c.MaxBonus), esc(currency)) + "\n")
	}
	if c.MaxUses > 0 {
		b.WriteString(t.f("card.discount.usesLimited", t.num(int64(c.Used)), t.num(int64(c.MaxUses))) + "\n")
	} else {
		b.WriteString(t.f("card.discount.usesUnlimited", t.num(int64(c.Used))) + "\n")
	}
	if c.ExpiresAt > 0 {
		b.WriteString(t.f("card.discount.expiresAt", t.date(c.ExpiresAt)) + "\n")
	} else {
		b.WriteString(t.s("card.discount.noExpiry") + "\n")
	}

	switch {
	case !c.Enabled:
		b.WriteString("\n" + t.s("card.discount.disabled"))
	case c.ExpiresAt > 0 && nowMilli() > c.ExpiresAt:
		b.WriteString("\n" + t.s("card.discount.expired"))
	case c.MaxUses > 0 && c.Used >= c.MaxUses:
		b.WriteString("\n" + t.s("card.discount.exhausted"))
	default:
		b.WriteString("\n" + t.s("card.discount.usable"))
	}
	return b.String()
}

// topUpInstructions is the screen between naming an amount and sending a receipt.
func (t *tr) topUpInstructions(id int, amount int64, currency, payText, code string, bonus int64) string {
	var b strings.Builder
	b.WriteString(t.f("card.topup.created", t.num(int64(id))) + "\n\n")
	b.WriteString(t.f("card.topup.amount", t.num(amount), esc(currency)) + "\n")
	if strings.TrimSpace(code) != "" && bonus > 0 {
		b.WriteString(t.f("card.topup.bonus", esc(code), t.num(bonus), esc(currency)) + "\n")
		b.WriteString(t.f("card.topup.total", t.num(amount+bonus), esc(currency)) + "\n")
	}
	b.WriteString("\n")
	if strings.TrimSpace(payText) != "" {
		b.WriteString(esc(payText) + "\n\n")
	} else {
		b.WriteString(t.s("card.topup.noPayInfo") + "\n\n")
	}
	b.WriteString(t.s("card.topup.sendReceipt"))
	return b.String()
}

// txLine renders one ledger entry.
func (t *tr) txLine(amount, balance int64, kind, details, currency string) string {
	sign := "➕"
	if amount < 0 {
		sign = "➖"
		amount = -amount
	}
	label := t.s("tx." + kind)
	if label == keyPrefix+"tx."+kind {
		label = kind
	}
	line := sign + " <b>" + t.num(amount) + " " + esc(currency) + "</b> — " + label
	if strings.TrimSpace(details) != "" {
		line += "\n   <i>" + esc(details) + "</i>"
	}
	line += "\n   " + t.f("tx.balanceAfter", t.num(balance), esc(currency))
	return line
}

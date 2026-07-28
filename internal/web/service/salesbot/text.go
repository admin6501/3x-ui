// Package salesbot: the bot's copy. Every string lives in the panel's
// translation files under the "shopbot." prefix and is rendered through tr, so
// a shop can be run in any language the panel ships — picked from the panel's
// settings, independently of the panel's own language and of the notification
// bot's.
package salesbot

import "strings"

// walletCard is the buyer's balance screen.
func (t *tr) walletCard(balance, paid, spent int64, currency string, configs int) string {
	var b strings.Builder
	b.WriteString(t.s("card.wallet.title") + "\n\n")
	b.WriteString(t.f("card.wallet.balance", t.num(balance), esc(currency)) + "\n")
	b.WriteString(t.f("card.wallet.paid", t.num(paid), esc(currency)) + "\n")
	b.WriteString(t.f("card.wallet.spent", t.num(spent), esc(currency)) + "\n")
	b.WriteString(t.f("card.wallet.configs", t.num(int64(configs))) + "\n")
	if balance <= 0 {
		b.WriteString("\n" + t.s("card.wallet.empty"))
	}
	return b.String()
}

// priceCard tells a buyer exactly what they will be charged for.
func (t *tr) priceCard(perGB, perDay int64, currency string, minTopUp, maxTopUp, minBalance, maxVolume int64) string {
	var b strings.Builder
	b.WriteString(t.s("card.prices.title") + "\n\n")
	if perGB > 0 {
		b.WriteString(t.f("card.prices.perGB", t.num(perGB), esc(currency)) + "\n")
	} else {
		b.WriteString(t.s("card.prices.freeTraffic") + "\n")
	}
	if perDay > 0 {
		b.WriteString(t.f("card.prices.perDay", t.num(perDay), esc(currency)) + "\n")
	}
	b.WriteString("\n")
	if minTopUp > 0 {
		b.WriteString(t.f("card.prices.minTopUp", t.num(minTopUp), esc(currency)) + "\n")
	}
	if maxTopUp > 0 {
		b.WriteString(t.f("card.prices.maxTopUp", t.num(maxTopUp), esc(currency)) + "\n")
	}
	if minBalance > 0 {
		b.WriteString(t.f("card.prices.minBalance", t.num(minBalance), esc(currency)) + "\n")
	}
	if maxVolume > 0 {
		b.WriteString(t.f("card.prices.maxVolume", t.quota(maxVolume)) + "\n")
	}
	b.WriteString("\n" + t.s("card.prices.note"))
	return b.String()
}

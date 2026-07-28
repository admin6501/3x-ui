package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

const gb = int64(1024 * 1024 * 1024)

func setShop(t *testing.T, pricePerGB, pricePerDay int64, inboundId int, extra map[string]string) {
	t.Helper()
	values := map[string]string{
		"shopPricePerGB":  itoa64(pricePerGB),
		"shopPricePerDay": itoa64(pricePerDay),
		"shopInboundId":   itoa(inboundId),
	}
	for k, v := range extra {
		values[k] = v
	}
	for key, value := range values {
		if err := database.GetDB().Where(model.Setting{Key: key}).
			Assign(model.Setting{Value: value}).FirstOrCreate(&model.Setting{}).Error; err != nil {
			t.Fatalf("set %s: %v", key, err)
		}
	}
}

func itoa64(n int64) string { return itoa(int(n)) }

// meter writes what xray would have recorded for a config's client.
func meter(t *testing.T, email string, bytes int64) {
	t.Helper()
	db := database.GetDB()
	var row xray.ClientTraffic
	err := db.Model(xray.ClientTraffic{}).Where("email = ?", email).First(&row).Error
	if err == nil {
		if err := db.Model(xray.ClientTraffic{}).Where("email = ?", email).
			Updates(map[string]any{"up": int64(0), "down": bytes}).Error; err != nil {
			t.Fatalf("update meter: %v", err)
		}
		return
	}
	if err := db.Create(&xray.ClientTraffic{Email: email, Enable: true, Down: bytes}).Error; err != nil {
		t.Fatalf("create meter: %v", err)
	}
}

func balanceOf(t *testing.T, shop *ShopService, id int64) int64 {
	t.Helper()
	u, err := shop.GetUser(id)
	if err != nil {
		t.Fatalf("get user %d: %v", id, err)
	}
	return u.Balance
}

// newConfig registers a config row directly, which is what CreateConfig does
// once the panel client exists. Tests that do not care about the client itself
// use this to keep the billing assertions in focus.
func newConfig(t *testing.T, telegramId int64, email string, volumeGB int64) *model.BotConfig {
	t.Helper()
	cfg := &model.BotConfig{TelegramId: telegramId, Email: email, VolumeGB: volumeGB, Active: true, InboundId: 1}
	if err := database.GetDB().Create(cfg).Error; err != nil {
		t.Fatalf("create config: %v", err)
	}
	return cfg
}

// TestUsageIsChargedPerGigabyteConsumed is the headline promise: buy 10 GB, use
// 1 GB, pay for 1 GB — not for 10.
func TestUsageIsChargedPerGigabyteConsumed(t *testing.T) {
	setupBulkDB(t)
	shop := &ShopService{}
	inboundSvc := &InboundService{}
	setShop(t, 2000, 0, 1, nil)

	if _, err := shop.User(900001, "ali", "Ali"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := shop.Adjust(900001, 50000, "seed"); err != nil {
		t.Fatalf("seed balance: %v", err)
	}
	cfg := newConfig(t, 900001, "tg900001_a", 10)

	// Nothing used yet — nothing charged.
	shop.BillAll(inboundSvc)
	if got := balanceOf(t, shop, 900001); got != 50000 {
		t.Fatalf("balance after an idle run = %d, want 50000", got)
	}

	// One gigabyte moved: one gigabyte charged.
	meter(t, cfg.Email, gb)
	shop.BillAll(inboundSvc)
	if got := balanceOf(t, shop, 900001); got != 48000 {
		t.Errorf("balance after 1 GB = %d, want 50000-2000", got)
	}

	// Running the job again without new traffic must not charge again.
	shop.BillAll(inboundSvc)
	if got := balanceOf(t, shop, 900001); got != 48000 {
		t.Errorf("billing is not idempotent: balance = %d, want 48000", got)
	}

	// Two more gigabytes.
	meter(t, cfg.Email, 3*gb)
	shop.BillAll(inboundSvc)
	if got := balanceOf(t, shop, 900001); got != 44000 {
		t.Errorf("balance after 3 GB total = %d, want 50000-6000", got)
	}
}

// TestPartialGigabytesAreCharged: a user who moves half a gigabyte pays half
// the price, and the fractions do not get lost across runs.
func TestPartialGigabytesAreCharged(t *testing.T) {
	setupBulkDB(t)
	shop := &ShopService{}
	inboundSvc := &InboundService{}
	setShop(t, 2000, 0, 1, nil)

	_, _ = shop.User(900002, "", "")
	_, _ = shop.Adjust(900002, 100000, "seed")
	cfg := newConfig(t, 900002, "tg900002_a", 10)

	meter(t, cfg.Email, gb/2)
	shop.BillAll(inboundSvc)
	if got := balanceOf(t, shop, 900002); got != 99000 {
		t.Errorf("half a gigabyte cost %d, want 1000", 100000-got)
	}

	// A second half-gigabyte completes the first, so the total is exactly the
	// full per-GB price — no rounding lost on either run.
	meter(t, cfg.Email, gb)
	shop.BillAll(inboundSvc)
	if got := balanceOf(t, shop, 900002); got != 98000 {
		t.Errorf("one gigabyte in two halves cost %d, want 2000", 100000-got)
	}
}

// TestRunningOutSwitchesConfigsOff is the whole point of a prepaid wallet: when
// the money is gone, the traffic stops.
func TestRunningOutSwitchesConfigsOff(t *testing.T) {
	setupBulkDB(t)
	shop := &ShopService{}
	inboundSvc := &InboundService{}
	ib := mkInbound(t, 34001, model.VLESS, `{"clients":[]}`)
	setShop(t, 2000, 0, ib.Id, map[string]string{"shopMinBalance": "0"})

	_, _ = shop.User(900003, "", "")
	if _, err := shop.Adjust(900003, 3000, "seed"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	cfg, err := shop.CreateConfig(inboundSvc, 900003, 10, "")
	if err != nil {
		t.Fatalf("create config: %v", err)
	}

	// Two gigabytes cost 4000 against a 3000 balance.
	meter(t, cfg.Email, 2*gb)
	result := shop.BillAll(inboundSvc)
	if got := balanceOf(t, shop, 900003); got != -1000 {
		t.Errorf("balance = %d, want -1000 (the last tick cannot un-send bytes)", got)
	}
	if len(result.SuspendedIds) != 1 || result.SuspendedIds[0] != 900003 {
		t.Errorf("suspended = %v, want [900003]", result.SuspendedIds)
	}

	reloaded, _ := shop.GetConfig(cfg.Id)
	if reloaded.Active {
		t.Error("the config should be switched off once the wallet is empty")
	}
	rec, err := shop.clientService.GetRecordByEmail(nil, cfg.Email)
	if err != nil {
		t.Fatalf("load client: %v", err)
	}
	if rec.Enable {
		t.Error("the panel client should be disabled, not just the bot's record")
	}

	// Topping up brings it back without the user having to do anything else.
	if _, err := shop.Adjust(900003, 20000, "top-up"); err != nil {
		t.Fatalf("top up: %v", err)
	}
	shop.BillAll(inboundSvc)
	reloaded, _ = shop.GetConfig(cfg.Id)
	if !reloaded.Active {
		t.Error("paying should bring the config back")
	}
	rec, _ = shop.clientService.GetRecordByEmail(nil, cfg.Email)
	if !rec.Enable {
		t.Error("the panel client should be re-enabled too")
	}
}

// TestDailyFeeIsChargedOncePerDay covers the optional time-based charge.
func TestDailyFeeIsChargedOncePerDay(t *testing.T) {
	setupBulkDB(t)
	shop := &ShopService{}
	inboundSvc := &InboundService{}
	setShop(t, 0, 500, 1, nil)

	_, _ = shop.User(900004, "", "")
	_, _ = shop.Adjust(900004, 10000, "seed")
	cfg := newConfig(t, 900004, "tg900004_a", 10)

	// Backdate the config by three days.
	threeDaysAgo := nowMilli() - 3*24*60*60*1000
	if err := database.GetDB().Model(&model.BotConfig{}).Where("id = ?", cfg.Id).
		Update("created_at", threeDaysAgo).Error; err != nil {
		t.Fatalf("backdate: %v", err)
	}

	shop.BillAll(inboundSvc)
	if got := balanceOf(t, shop, 900004); got != 8500 {
		t.Errorf("three days cost %d, want 1500", 10000-got)
	}
	// A second run on the same day charges nothing more.
	shop.BillAll(inboundSvc)
	if got := balanceOf(t, shop, 900004); got != 8500 {
		t.Errorf("the daily fee was charged twice for the same day: %d", got)
	}
}

// TestFreePricingChargesNothing: with the price left at zero the shop gives
// traffic away rather than silently charging some default.
func TestFreePricingChargesNothing(t *testing.T) {
	setupBulkDB(t)
	shop := &ShopService{}
	inboundSvc := &InboundService{}
	setShop(t, 0, 0, 1, nil)

	_, _ = shop.User(900005, "", "")
	_, _ = shop.Adjust(900005, 10000, "seed")
	cfg := newConfig(t, 900005, "tg900005_a", 10)
	meter(t, cfg.Email, 5*gb)

	shop.BillAll(inboundSvc)
	if got := balanceOf(t, shop, 900005); got != 10000 {
		t.Errorf("balance = %d, want it untouched when no price is set", got)
	}
}

// TestTopUpBoundsAreEnforced covers the min/max the admin configures.
func TestTopUpBoundsAreEnforced(t *testing.T) {
	setupBulkDB(t)
	shop := &ShopService{}
	setShop(t, 2000, 0, 1, map[string]string{"shopMinTopUp": "50000", "shopMaxTopUp": "500000"})

	if _, err := shop.RequestTopUp(900006, "A", 10000); err != ErrTopUpTooSmall {
		t.Errorf("below minimum = %v, want ErrTopUpTooSmall", err)
	}
	if _, err := shop.RequestTopUp(900006, "A", 600000); err != ErrTopUpTooLarge {
		t.Errorf("above maximum = %v, want ErrTopUpTooLarge", err)
	}
	if _, err := shop.RequestTopUp(900006, "A", 0); err != ErrTopUpTooSmall {
		t.Errorf("zero = %v, want ErrTopUpTooSmall", err)
	}
	if _, err := shop.RequestTopUp(900006, "A", 100000); err != nil {
		t.Errorf("an in-range amount was refused: %v", err)
	}
}

// TestApprovingATopUpCreditsOnce keeps a double-tap on the approve button from
// paying somebody twice.
func TestApprovingATopUpCreditsOnce(t *testing.T) {
	setupBulkDB(t)
	shop := &ShopService{}
	setShop(t, 2000, 0, 1, nil)

	_, _ = shop.User(900007, "", "")
	top, err := shop.RequestTopUp(900007, "A", 100000)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if _, err := shop.AttachTopUpReceipt(top.Id, "file-1"); err != nil {
		t.Fatalf("attach: %v", err)
	}
	if _, balance, err := shop.ApproveTopUp(top.Id); err != nil || balance != 100000 {
		t.Fatalf("approve = %d, %v", balance, err)
	}
	if _, _, err := shop.ApproveTopUp(top.Id); err != ErrTopUpNotPending {
		t.Errorf("second approve = %v, want ErrTopUpNotPending", err)
	}
	if got := balanceOf(t, shop, 900007); got != 100000 {
		t.Errorf("balance = %d, want the top-up counted once", got)
	}

	// A rejected top-up credits nothing.
	other, _ := shop.RequestTopUp(900007, "A", 100000)
	_, _ = shop.AttachTopUpReceipt(other.Id, "file-2")
	if _, err := shop.RejectTopUp(other.Id, "رسید نامعتبر"); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if got := balanceOf(t, shop, 900007); got != 100000 {
		t.Errorf("a rejected top-up moved the balance to %d", got)
	}
}

// TestBuyingNeedsFunds stops a user with an empty wallet from creating a config
// they could never pay for.
func TestBuyingNeedsFunds(t *testing.T) {
	setupBulkDB(t)
	shop := &ShopService{}
	inboundSvc := &InboundService{}
	ib := mkInbound(t, 34011, model.VLESS, `{"clients":[]}`)
	setShop(t, 2000, 0, ib.Id, map[string]string{"shopMinBalance": "20000", "shopMaxVolumeGB": "100"})

	_, _ = shop.User(900008, "", "")
	if _, err := shop.CreateConfig(inboundSvc, 900008, 10, ""); err != ErrInsufficientFund {
		t.Errorf("empty wallet = %v, want ErrInsufficientFund", err)
	}
	_, _ = shop.Adjust(900008, 10000, "seed")
	if _, err := shop.CreateConfig(inboundSvc, 900008, 10, ""); err != ErrInsufficientFund {
		t.Errorf("below the minimum balance = %v, want ErrInsufficientFund", err)
	}
	_, _ = shop.Adjust(900008, 20000, "seed")
	if _, err := shop.CreateConfig(inboundSvc, 900008, 200, ""); err != ErrVolumeTooLarge {
		t.Errorf("over the volume ceiling = %v, want ErrVolumeTooLarge", err)
	}
	if _, err := shop.CreateConfig(inboundSvc, 900008, 0, ""); err != ErrVolumeInvalid {
		t.Errorf("zero volume = %v, want ErrVolumeInvalid", err)
	}
	cfg, err := shop.CreateConfig(inboundSvc, 900008, 10, "")
	if err != nil {
		t.Fatalf("a funded user could not buy: %v", err)
	}
	// The panel client carries the requested cap, so xray stops it at 10 GB
	// even if the wallet would have covered more.
	rec, err := shop.clientService.GetRecordByEmail(nil, cfg.Email)
	if err != nil {
		t.Fatalf("client not created: %v", err)
	}
	if rec.TotalGB != 10*gb {
		t.Errorf("client cap = %d bytes, want 10 GB", rec.TotalGB)
	}
}

// TestConfigNeedsAnInbound: without a configured inbound the shop must refuse
// rather than create a client nobody can connect to.
func TestConfigNeedsAnInbound(t *testing.T) {
	setupBulkDB(t)
	shop := &ShopService{}
	inboundSvc := &InboundService{}
	setShop(t, 2000, 0, 0, nil)

	_, _ = shop.User(900009, "", "")
	_, _ = shop.Adjust(900009, 100000, "seed")
	if _, err := shop.CreateConfig(inboundSvc, 900009, 10, ""); err != ErrNoShopInbound {
		t.Errorf("unset inbound = %v, want ErrNoShopInbound", err)
	}
	setShop(t, 2000, 0, 4242, nil)
	if _, err := shop.CreateConfig(inboundSvc, 900009, 10, ""); err != ErrNoShopInbound {
		t.Errorf("inbound that does not exist = %v, want ErrNoShopInbound", err)
	}
}

// TestLedgerExplainsTheBalance: every movement is recorded, and the running
// balance on each entry agrees with the wallet.
func TestLedgerExplainsTheBalance(t *testing.T) {
	setupBulkDB(t)
	shop := &ShopService{}
	inboundSvc := &InboundService{}
	setShop(t, 2000, 0, 1, nil)

	_, _ = shop.User(900010, "", "")
	_, _ = shop.Adjust(900010, 100000, "seed")
	cfg := newConfig(t, 900010, "tg900010_a", 10)
	meter(t, cfg.Email, 2*gb)
	shop.BillAll(inboundSvc)

	entries, err := shop.Transactions(900010, 0)
	if err != nil {
		t.Fatalf("ledger: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("ledger has %d entries, want a credit and a charge", len(entries))
	}
	// Newest first.
	if entries[0].Kind != model.TxUsage || entries[0].Amount != -4000 {
		t.Errorf("usage entry = %+v", entries[0])
	}
	if entries[1].Kind != model.TxAdjust || entries[1].Amount != 100000 {
		t.Errorf("credit entry = %+v", entries[1])
	}
	if entries[0].Balance != balanceOf(t, shop, 900010) {
		t.Errorf("ledger balance %d disagrees with the wallet %d",
			entries[0].Balance, balanceOf(t, shop, 900010))
	}
}

// TestStatsAddUp keeps the admin's figures honest.
func TestStatsAddUp(t *testing.T) {
	setupBulkDB(t)
	shop := &ShopService{}
	inboundSvc := &InboundService{}
	setShop(t, 2000, 0, 1, nil)

	_, _ = shop.User(900011, "", "")
	top, _ := shop.RequestTopUp(900011, "A", 100000)
	_, _ = shop.AttachTopUpReceipt(top.Id, "f")
	_, _, _ = shop.ApproveTopUp(top.Id)
	cfg := newConfig(t, 900011, "tg900011_a", 10)
	meter(t, cfg.Email, gb)
	shop.BillAll(inboundSvc)

	// A second user who only ever asked, and was never approved.
	_, _ = shop.User(900012, "", "")
	pending, _ := shop.RequestTopUp(900012, "B", 100000)
	_, _ = shop.AttachTopUpReceipt(pending.Id, "f2")

	stats := shop.Stats()
	if stats.Users != 2 {
		t.Errorf("users = %d, want 2", stats.Users)
	}
	if stats.TotalPaid != 100000 {
		t.Errorf("totalPaid = %d, want only the approved top-up", stats.TotalPaid)
	}
	if stats.TotalSpent != 2000 {
		t.Errorf("totalSpent = %d, want the one gigabyte charged", stats.TotalSpent)
	}
	if stats.WalletBalance != 98000 {
		t.Errorf("walletBalance = %d, want 98000", stats.WalletBalance)
	}
	if stats.PendingTopUps != 1 {
		t.Errorf("pendingTopUps = %d, want 1", stats.PendingTopUps)
	}
}

// TestPausingSurvivesABillingRun: a config its owner switched off from the bot
// must stay off. Billing owns Active, so without a separate Paused flag the next
// run would look at a funded wallet and switch the config straight back on.
func TestPausingSurvivesABillingRun(t *testing.T) {
	setupBulkDB(t)
	shop := &ShopService{}
	inboundSvc := &InboundService{}
	ib := mkInbound(t, 34011, model.VLESS, `{"clients":[]}`)
	setShop(t, 2000, 0, ib.Id, map[string]string{"shopMinBalance": "0"})

	_, _ = shop.User(900011, "", "")
	if _, err := shop.Adjust(900011, 100000, "seed"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	cfg, err := shop.CreateConfig(inboundSvc, 900011, 10, "")
	if err != nil {
		t.Fatalf("create config: %v", err)
	}

	paused, err := shop.SetConfigPaused(inboundSvc, cfg.Id, true)
	if err != nil {
		t.Fatalf("pause: %v", err)
	}
	if !paused.Paused || paused.Active {
		t.Fatalf("after pausing: paused=%v active=%v", paused.Paused, paused.Active)
	}
	rec, err := shop.clientService.GetRecordByEmail(nil, cfg.Email)
	if err != nil {
		t.Fatalf("load client: %v", err)
	}
	if rec.Enable {
		t.Error("pausing should disable the panel client too")
	}

	// A billing run with money in the wallet must not undo the pause.
	meter(t, cfg.Email, gb)
	shop.BillAll(inboundSvc)
	reloaded, _ := shop.GetConfig(cfg.Id)
	if reloaded.Active {
		t.Error("billing re-enabled a config the owner had paused")
	}

	// Resuming brings it back.
	resumed, err := shop.SetConfigPaused(inboundSvc, cfg.Id, false)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if resumed.Paused || !resumed.Active {
		t.Fatalf("after resuming: paused=%v active=%v", resumed.Paused, resumed.Active)
	}
	rec, _ = shop.clientService.GetRecordByEmail(nil, cfg.Email)
	if !rec.Enable {
		t.Error("resuming should re-enable the panel client")
	}
}

// TestResumingWithAnEmptyWalletStaysOff: un-pausing is not a way to get service
// without paying for it.
func TestResumingWithAnEmptyWalletStaysOff(t *testing.T) {
	setupBulkDB(t)
	shop := &ShopService{}
	inboundSvc := &InboundService{}
	ib := mkInbound(t, 34012, model.VLESS, `{"clients":[]}`)
	setShop(t, 2000, 0, ib.Id, map[string]string{"shopMinBalance": "0"})

	_, _ = shop.User(900012, "", "")
	if _, err := shop.Adjust(900012, 5000, "seed"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	cfg, err := shop.CreateConfig(inboundSvc, 900012, 10, "")
	if err != nil {
		t.Fatalf("create config: %v", err)
	}
	if _, err := shop.SetConfigPaused(inboundSvc, cfg.Id, true); err != nil {
		t.Fatalf("pause: %v", err)
	}
	if _, err := shop.Adjust(900012, -5000, "spend it all"); err != nil {
		t.Fatalf("drain: %v", err)
	}

	resumed, err := shop.SetConfigPaused(inboundSvc, cfg.Id, false)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if resumed.Active {
		t.Error("an empty wallet must not be able to resume a config")
	}
}

// TestAddVolumeRaisesTheCapWithoutCharging: the wallet pays for traffic as it is
// used, so raising the ceiling is free at the moment it happens.
func TestAddVolumeRaisesTheCapWithoutCharging(t *testing.T) {
	setupBulkDB(t)
	shop := &ShopService{}
	inboundSvc := &InboundService{}
	ib := mkInbound(t, 34013, model.VLESS, `{"clients":[]}`)
	setShop(t, 2000, 0, ib.Id, map[string]string{"shopMinBalance": "0", "shopMaxVolumeGB": "50"})

	_, _ = shop.User(900013, "", "")
	if _, err := shop.Adjust(900013, 100000, "seed"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	cfg, err := shop.CreateConfig(inboundSvc, 900013, 10, "")
	if err != nil {
		t.Fatalf("create config: %v", err)
	}
	before := balanceOf(t, shop, 900013)

	grown, err := shop.AddVolume(inboundSvc, cfg.Id, 15)
	if err != nil {
		t.Fatalf("add volume: %v", err)
	}
	if grown.VolumeGB != 25 {
		t.Errorf("volume = %d GB, want 25", grown.VolumeGB)
	}
	if got := balanceOf(t, shop, 900013); got != before {
		t.Errorf("balance moved from %d to %d; adding volume must not charge", before, got)
	}
	rec, err := shop.clientService.GetRecordByEmail(nil, cfg.Email)
	if err != nil {
		t.Fatalf("load client: %v", err)
	}
	if rec.TotalGB != 25*gb {
		t.Errorf("panel client cap = %d bytes, want %d", rec.TotalGB, 25*gb)
	}

	// The configured maximum still applies.
	if _, err := shop.AddVolume(inboundSvc, cfg.Id, 30); err != ErrVolumeTooLarge {
		t.Errorf("adding past the maximum returned %v, want ErrVolumeTooLarge", err)
	}
	if _, err := shop.AddVolume(inboundSvc, cfg.Id, 0); err != ErrVolumeInvalid {
		t.Errorf("adding zero returned %v, want ErrVolumeInvalid", err)
	}
}

// TestAutoNameCarriesNoTelegramId: the config name travels in share links, in
// the subscription page and in the panel's client list. Putting the buyer's
// Telegram id in it would identify them to anyone who sees any of those.
func TestAutoNameCarriesNoTelegramId(t *testing.T) {
	setupBulkDB(t)
	shop := &ShopService{}
	inboundSvc := &InboundService{}
	ib := mkInbound(t, 34021, model.VLESS, `{"clients":[]}`)
	setShop(t, 2000, 0, ib.Id, map[string]string{"shopMinBalance": "0"})

	const telegramId = int64(918273645)
	_, _ = shop.User(telegramId, "", "")
	if _, err := shop.Adjust(telegramId, 100000, "seed"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	cfg, err := shop.CreateConfig(inboundSvc, telegramId, 10, "")
	if err != nil {
		t.Fatalf("create config: %v", err)
	}
	if strings.Contains(cfg.Email, "918273645") {
		t.Errorf("generated name %q leaks the buyer's Telegram id", cfg.Email)
	}
	if !strings.HasPrefix(cfg.Email, "cfg-") {
		t.Errorf("generated name %q does not look like a shop config name", cfg.Email)
	}
	// Two configs in a row must not collide.
	second, err := shop.CreateConfig(inboundSvc, telegramId, 10, "")
	if err != nil {
		t.Fatalf("create second config: %v", err)
	}
	if second.Email == cfg.Email {
		t.Error("two generated names collided")
	}
}

// TestBuyerChosenName covers the other half: a name the buyer typed is used
// as-is, and the ones that would break the panel are refused.
func TestBuyerChosenName(t *testing.T) {
	setupBulkDB(t)
	shop := &ShopService{}
	inboundSvc := &InboundService{}
	ib := mkInbound(t, 34022, model.VLESS, `{"clients":[]}`)
	setShop(t, 2000, 0, ib.Id, map[string]string{"shopMinBalance": "0"})

	_, _ = shop.User(900051, "", "")
	if _, err := shop.Adjust(900051, 100000, "seed"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	cfg, err := shop.CreateConfig(inboundSvc, 900051, 5, "my-phone")
	if err != nil {
		t.Fatalf("create with a chosen name: %v", err)
	}
	if cfg.Email != "my-phone" {
		t.Errorf("config name = %q, want my-phone", cfg.Email)
	}
	// The panel keys clients by this, so a duplicate has to be refused.
	if _, err := shop.CreateConfig(inboundSvc, 900051, 5, "my-phone"); !errors.Is(err, ErrNameTaken) {
		t.Errorf("duplicate name returned %v, want ErrNameTaken", err)
	}
	for _, bad := range []string{"ab", strings.Repeat("x", 33), "has space", "quote\"", "emoji😀", "semi;colon"} {
		if _, err := shop.CreateConfig(inboundSvc, 900051, 5, bad); !errors.Is(err, ErrNameInvalid) {
			t.Errorf("name %q returned %v, want ErrNameInvalid", bad, err)
		}
	}
	// Surrounding whitespace is the user's keyboard, not their intent.
	if cfg, err := shop.CreateConfig(inboundSvc, 900051, 5, "  laptop  "); err != nil {
		t.Errorf("padded name refused: %v", err)
	} else if cfg.Email != "laptop" {
		t.Errorf("padded name stored as %q", cfg.Email)
	}
}

// TestDeadConfigsAreSweptAfterTheGracePeriod is the clean-up: a config whose
// traffic ran out, or whose owner's wallet is empty, disappears from both the
// bot and the panel once it has been that way longer than the admin allows.
func TestDeadConfigsAreSweptAfterTheGracePeriod(t *testing.T) {
	setupBulkDB(t)
	shop := &ShopService{}
	inboundSvc := &InboundService{}
	ib := mkInbound(t, 34023, model.VLESS, `{"clients":[]}`)
	setShop(t, 2000, 0, ib.Id, map[string]string{"shopMinBalance": "0", "shopDeleteDeadDays": "7"})

	_, _ = shop.User(900061, "", "")
	if _, err := shop.Adjust(900061, 100000, "seed"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	alive, err := shop.CreateConfig(inboundSvc, 900061, 10, "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	spent, err := shop.CreateConfig(inboundSvc, 900061, 10, "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	meter(t, spent.Email, 10*gb) // used its whole allowance

	// First pass only marks; nothing is old enough to sweep.
	if deleted := shop.MaintainConfigs(inboundSvc); len(deleted) != 0 {
		t.Fatalf("first pass deleted %v, want nothing", deleted)
	}
	marked, _ := shop.GetConfig(spent.Id)
	if marked.DeadSince == 0 {
		t.Error("the out-of-traffic config was not marked dead")
	}
	healthy, _ := shop.GetConfig(alive.Id)
	if healthy.DeadSince != 0 {
		t.Error("a healthy config was marked dead")
	}

	// Backdate it past the grace period.
	if err := database.GetDB().Model(&model.BotConfig{}).Where("id = ?", spent.Id).
		Update("dead_since", nowMilli()-8*24*60*60*1000).Error; err != nil {
		t.Fatalf("backdate: %v", err)
	}
	deleted := shop.MaintainConfigs(inboundSvc)
	if len(deleted) != 1 || deleted[0].Email != spent.Email {
		t.Fatalf("swept %v, want just %s", deleted, spent.Email)
	}
	if _, err := shop.GetConfig(spent.Id); err == nil {
		t.Error("the config row survived the sweep")
	}
	if _, err := shop.clientService.GetRecordByEmail(nil, spent.Email); err == nil {
		t.Error("the panel client survived the sweep — it should go from both sides")
	}
	if _, err := shop.GetConfig(alive.Id); err != nil {
		t.Error("the healthy config was swept too")
	}
}

// TestSweepLeavesPausedAndRecoveredConfigsAlone: pausing is a choice, and a
// config that comes back to life must lose its death mark rather than carry an
// old timestamp into the next outage.
func TestSweepLeavesPausedAndRecoveredConfigsAlone(t *testing.T) {
	setupBulkDB(t)
	shop := &ShopService{}
	inboundSvc := &InboundService{}
	ib := mkInbound(t, 34024, model.VLESS, `{"clients":[]}`)
	setShop(t, 2000, 0, ib.Id, map[string]string{"shopMinBalance": "0", "shopDeleteDeadDays": "1"})

	_, _ = shop.User(900071, "", "")
	if _, err := shop.Adjust(900071, 100000, "seed"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	paused, err := shop.CreateConfig(inboundSvc, 900071, 10, "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := shop.SetConfigPaused(inboundSvc, paused.Id, true); err != nil {
		t.Fatalf("pause: %v", err)
	}
	shop.MaintainConfigs(inboundSvc)
	reloaded, _ := shop.GetConfig(paused.Id)
	if reloaded.DeadSince != 0 {
		t.Error("a config the owner paused was marked dead")
	}

	// A wallet that empties and then refills clears the mark.
	other, err := shop.CreateConfig(inboundSvc, 900071, 10, "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := shop.Adjust(900071, -100000, "drain"); err != nil {
		t.Fatalf("drain: %v", err)
	}
	shop.MaintainConfigs(inboundSvc)
	if got, _ := shop.GetConfig(other.Id); got.DeadSince == 0 {
		t.Fatal("an empty wallet did not mark the config dead")
	}
	if _, err := shop.Adjust(900071, 50000, "refill"); err != nil {
		t.Fatalf("refill: %v", err)
	}
	shop.MaintainConfigs(inboundSvc)
	if got, _ := shop.GetConfig(other.Id); got.DeadSince != 0 {
		t.Error("topping up did not clear the death mark")
	}
}

// TestSweepIsOffByDefault: a panel that never sets the grace period must not
// start deleting people's configs after an upgrade.
func TestSweepIsOffByDefault(t *testing.T) {
	setupBulkDB(t)
	shop := &ShopService{}
	inboundSvc := &InboundService{}
	ib := mkInbound(t, 34025, model.VLESS, `{"clients":[]}`)
	setShop(t, 2000, 0, ib.Id, map[string]string{"shopMinBalance": "0"})

	_, _ = shop.User(900081, "", "")
	if _, err := shop.Adjust(900081, 100000, "seed"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	cfg, err := shop.CreateConfig(inboundSvc, 900081, 10, "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	meter(t, cfg.Email, 20*gb)
	shop.MaintainConfigs(inboundSvc)
	if err := database.GetDB().Model(&model.BotConfig{}).Where("id = ?", cfg.Id).
		Update("dead_since", int64(1)).Error; err != nil {
		t.Fatalf("backdate: %v", err)
	}
	if deleted := shop.MaintainConfigs(inboundSvc); len(deleted) != 0 {
		t.Errorf("swept %v with the grace period unset", deleted)
	}
	if _, err := shop.GetConfig(cfg.Id); err != nil {
		t.Error("a config was deleted with the sweep switched off")
	}
}

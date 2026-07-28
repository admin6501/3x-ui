package job

import (
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// ShopBillingJob meters every config the Telegram shop sold and debits its
// owner's wallet for what it actually moved. It is what makes the shop
// pay-as-you-go rather than prepaid: the money leaves the wallet as the bytes
// go by, and a config whose owner has run out is switched off.
//
// The run is inert until an admin sets a price: with both the per-GB and the
// per-day price at their default of zero, nothing is ever charged.
// The cron ticks this job every minute and the job decides whether its own
// interval has elapsed. Rebuilding the cron entry instead would mean a settings
// change only took effect after a panel restart, which is not how the rest of
// the shop settings behave.
type ShopBillingJob struct {
	shopService    service.ShopService
	settingService service.SettingService
	inboundService service.InboundService
	// notify tells the bot which users were just cut off, so they hear about it
	// from the shop rather than by noticing their connection died. Optional.
	notify func(telegramIds []int64)
	// notifyDeleted tells the bot whose configs the clean-up sweep removed. A
	// config vanishing without a word would read as the shop losing it. Optional.
	notifyDeleted func(deleted []service.DeletedConfig)

	mu      sync.Mutex
	lastRun time.Time
}

// NewShopBillingJob creates the billing job. Both callbacks may be nil.
func NewShopBillingJob(notify func([]int64), notifyDeleted func([]service.DeletedConfig)) *ShopBillingJob {
	return &ShopBillingJob{notify: notify, notifyDeleted: notifyDeleted}
}

// due reports whether the configured interval has elapsed, and records the run.
// The first tick after start is always due, so a panel that has just come up
// bills before waiting out a long interval.
func (j *ShopBillingJob) due(now time.Time) bool {
	minutes, _ := j.settingService.GetShopBillingInterval()
	j.mu.Lock()
	defer j.mu.Unlock()
	if !j.lastRun.IsZero() && now.Sub(j.lastRun) < time.Duration(minutes)*time.Minute {
		return false
	}
	j.lastRun = now
	return true
}

func (j *ShopBillingJob) Run() {
	if !j.due(time.Now()) {
		return
	}
	result := j.shopService.BillAll(&j.inboundService)
	if result.Charged > 0 {
		logger.Debugf("shop billing: charged %d across %d wallet(s)", result.Charged, result.ChargedUsers)
	}
	if len(result.SuspendedIds) > 0 && j.notify != nil {
		j.notify(result.SuspendedIds)
	}

	// The sweep runs on the same tick but is independent of pricing: a shop that
	// charges nothing still accumulates configs whose owner walked away.
	if deleted := j.shopService.MaintainConfigs(&j.inboundService); len(deleted) > 0 {
		logger.Debugf("shop: swept %d config(s) that had been dead past the grace period", len(deleted))
		if j.notifyDeleted != nil {
			j.notifyDeleted(deleted)
		}
	}
}

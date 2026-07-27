package job

import (
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
type ShopBillingJob struct {
	shopService    service.ShopService
	inboundService service.InboundService
	// notify tells the bot which users were just cut off, so they hear about it
	// from the shop rather than by noticing their connection died. Optional.
	notify func(telegramIds []int64)
}

// NewShopBillingJob creates the billing job. notify may be nil.
func NewShopBillingJob(notify func([]int64)) *ShopBillingJob {
	return &ShopBillingJob{notify: notify}
}

func (j *ShopBillingJob) Run() {
	result := j.shopService.BillAll(&j.inboundService)
	if result.Charged > 0 {
		logger.Debugf("shop billing: charged %d across %d wallet(s)", result.Charged, result.ChargedUsers)
	}
	if len(result.SuspendedIds) > 0 && j.notify != nil {
		j.notify(result.SuspendedIds)
	}
}

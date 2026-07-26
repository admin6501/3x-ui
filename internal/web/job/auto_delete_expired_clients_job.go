package job

import (
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// AutoDeleteExpiredClientsJob removes clients that have been expired for longer
// than the configured grace period, so a panel doesn't slowly fill up with dead
// accounts an admin has to sweep by hand.
//
// Two independent guards keep it inert unless an admin really asked for it: the
// feature switch must be on AND the grace period must be greater than zero.
// Either one at its default (off / 0 days) means nothing is ever deleted.
type AutoDeleteExpiredClientsJob struct {
	settingService service.SettingService
	inboundService service.InboundService
	clientService  service.ClientService
	xrayService    service.XrayService
}

// NewAutoDeleteExpiredClientsJob creates the auto-delete job.
func NewAutoDeleteExpiredClientsJob() *AutoDeleteExpiredClientsJob {
	return new(AutoDeleteExpiredClientsJob)
}

// Run deletes the clients whose expiry date passed more than the configured
// number of days ago. Clients that only exhausted their traffic quota are left
// alone — there is no recorded moment to age them from — as are auto-renew
// clients, which are due to come back.
func (j *AutoDeleteExpiredClientsJob) Run() {
	enabled, err := j.settingService.GetAutoDeleteExpiredEnable()
	if err != nil {
		logger.Warning("AutoDeleteExpiredClients: reading the enable flag failed:", err)
		return
	}
	if !enabled {
		return
	}

	days, err := j.settingService.GetAutoDeleteExpiredDays()
	if err != nil {
		logger.Warning("AutoDeleteExpiredClients: reading the grace period failed:", err)
		return
	}
	if days <= 0 {
		// Switched on but left at zero days: treat it as "never delete" rather
		// than "delete everything expired right now".
		return
	}

	deleted, needRestart, err := j.clientService.DeleteExpiredOlderThan(&j.inboundService, days)
	if err != nil {
		logger.Warning("AutoDeleteExpiredClients: delete failed:", err)
		return
	}
	if deleted == 0 {
		return
	}
	logger.Infof("AutoDeleteExpiredClients: removed %d client(s) expired for more than %d day(s)", deleted, days)
	if needRestart {
		j.xrayService.SetToNeedRestart()
	}
}

package job

import "github.com/mhsanaei/3x-ui/v3/internal/web/service"

// ClientActivityJob drives per-client activity tracking: it appends new
// access-log connections to the activity table and resolves the network
// operator behind newly seen IPs. Both steps are no-ops while the
// clientActivityEnable setting is off, so scheduling it unconditionally is
// cheap and the feature turns on and off purely from settings.
type ClientActivityJob struct {
	activityService service.ClientActivityService
}

func NewClientActivityJob() *ClientActivityJob {
	return new(ClientActivityJob)
}

func (j *ClientActivityJob) Run() {
	j.activityService.Collect()
	j.activityService.EnrichIPs()
}

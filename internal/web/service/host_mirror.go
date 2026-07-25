package service

import (
	"slices"

	"gorm.io/gorm"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

// The legacy `externalProxy` array inside an inbound's streamSettings was
// replaced by the Hosts page, but it is still what the panel's own link/QR
// generator reads (and what the subscription layer falls back to when an
// inbound has no host). Leaving it frozen at its pre-migration value made the
// panel show stale links and let deleted hosts "come back" through the
// fallback path.
//
// So instead of deleting it, the array is kept as a mirror of the inbound's
// hosts: every host mutation rewrites it from the current host rows. Hosts stay
// the single source of truth; externalProxy is just their projection.
//
// Only hosts that are enabled and not excluded from the "raw" subscription are
// mirrored (model.HostsToExternalProxyEntries) — that is exactly the set the
// panel's raw share links represent, and it keeps the subscription fallback
// (used when no host applies to a format) from resurrecting an excluded host.

// syncInboundHostMirror rewrites one inbound's streamSettings.externalProxy from
// its host rows.
//
// force=false leaves inbounds that have no host row at all untouched: such an
// inbound may still be using a hand-written externalProxy array (the legacy
// feature is still supported), and it must not be wiped. Deletion paths pass
// force=true so removing the last host also clears its mirror.
func syncInboundHostMirror(db *gorm.DB, inboundId int, force bool) error {
	if db == nil {
		db = database.GetDB()
	}
	var hosts []*model.Host
	if err := db.Where("inbound_id = ?", inboundId).
		Order("sort_order asc, id asc").
		Find(&hosts).Error; err != nil {
		return err
	}
	if len(hosts) == 0 && !force {
		return nil
	}

	inbound := &model.Inbound{}
	if err := db.First(inbound, inboundId).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil
		}
		return err
	}

	entries := model.HostsToExternalProxyEntries(hosts, inbound.Port)
	updated, changed, err := model.StreamSettingsWithExternalProxy(inbound.StreamSettings, entries)
	if err != nil {
		logger.Warning("host mirror: inbound", inboundId, "has invalid streamSettings:", err)
		return nil
	}
	if !changed {
		return nil
	}
	return db.Model(&model.Inbound{}).
		Where("id = ?", inboundId).
		Update("stream_settings", updated).Error
}

// inboundIdsForHosts resolves the inbounds a set of host ids belongs to, used by
// the delete paths (which need the ids before the rows disappear).
func inboundIdsForHosts(db *gorm.DB, ids []int) []int {
	if len(ids) == 0 {
		return nil
	}
	var hosts []*model.Host
	if err := db.Where("id IN ?", ids).Find(&hosts).Error; err != nil {
		logger.Warning("host mirror: resolving inbound ids:", err)
		return nil
	}
	out := make([]int, 0, len(hosts))
	for _, h := range hosts {
		if !slices.Contains(out, h.InboundId) {
			out = append(out, h.InboundId)
		}
	}
	return out
}

// syncHostMirrors syncs several inbounds, logging (not returning) failures: the
// host mutation itself already succeeded, the mirror is derived data.
func syncHostMirrors(db *gorm.DB, inboundIds []int, force bool) {
	for _, id := range inboundIds {
		if err := syncInboundHostMirror(db, id, force); err != nil {
			logger.Warning("host mirror: sync inbound", id, "failed:", err)
		}
	}
}

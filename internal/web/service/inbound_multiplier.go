package service

import (
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// A multiplier of 1 is the identity, and 0 is what an inbound created before the
// column existed carries — both mean "count traffic as measured".
const defaultTrafficMultiplier = 1.0

// NormalizeTrafficMultiplier brings a stored or submitted multiplier into the
// range the panel supports. Zero and negatives read as unset rather than as
// "free traffic": an inbound that silently stopped charging its clients would be
// a quota hole, and there is already an Enable switch for turning one off.
func NormalizeTrafficMultiplier(v float64) float64 {
	if v <= 0 {
		return defaultTrafficMultiplier
	}
	if v > maxTrafficMultiplier {
		return maxTrafficMultiplier
	}
	return v
}

const maxTrafficMultiplier = 100.0

// ApplyTrafficMultiplier scales one measured byte count. It rounds to nearest so
// a fractional multiplier neither systematically over- nor under-charges across
// the many small deltas a poll produces, and never turns real traffic into zero
// — a client moving bytes must always move their counter, or a small enough
// poll would be free.
func ApplyTrafficMultiplier(bytes int64, multiplier float64) int64 {
	if bytes <= 0 || multiplier == defaultTrafficMultiplier {
		return bytes
	}
	scaled := int64(float64(bytes)*multiplier + 0.5)
	if scaled < 1 {
		scaled = 1
	}
	return scaled
}

// trafficMultipliersByInbound returns every inbound whose multiplier is not 1,
// keyed by inbound id. Inbounds at 1 are left out so the common case — nobody
// uses the feature — costs one small query and no further work.
func (s *InboundService) trafficMultipliersByInbound() map[int]float64 {
	var rows []struct {
		Id                int
		TrafficMultiplier float64
	}
	if err := database.GetDB().Model(&model.Inbound{}).
		Select("id", "traffic_multiplier").
		Where("traffic_multiplier > 0 AND traffic_multiplier <> 1").
		Scan(&rows).Error; err != nil {
		return nil
	}
	if len(rows) == 0 {
		return nil
	}
	out := make(map[int]float64, len(rows))
	for _, r := range rows {
		out[r.Id] = NormalizeTrafficMultiplier(r.TrafficMultiplier)
	}
	return out
}

// trafficMultipliersByEmail maps each of the given emails to the multiplier its
// traffic should be charged at.
//
// Xray counts by user email, not by inbound, so a client attached to several
// inbounds arrives as one undivided number that cannot be attributed. Such a
// client is charged at the highest multiplier among the inbounds they are on:
// the alternative — the lowest, or an average — would let a client sidestep a
// deliberately expensive inbound just by also being attached to a cheap one.
// A client on a single inbound, which is the normal case and the only one the
// shop ever creates, is simply charged at that inbound's rate.
//
// Returns nil when no inbound has a multiplier, which is the common case.
func (s *InboundService) trafficMultipliersByEmail(emails []string) map[string]float64 {
	byInbound := s.trafficMultipliersByInbound()
	if len(byInbound) == 0 || len(emails) == 0 {
		return nil
	}
	ids := make([]int, 0, len(byInbound))
	for id := range byInbound {
		ids = append(ids, id)
	}

	out := make(map[string]float64)
	db := database.GetDB()
	for _, batch := range chunkStrings(emails, sqlInChunk) {
		var rows []struct {
			Email     string
			InboundId int
		}
		if err := db.Table("client_inbounds AS ci").
			Select("c.email AS email, ci.inbound_id AS inbound_id").
			Joins("JOIN clients AS c ON c.id = ci.client_id").
			Where("c.email IN ? AND ci.inbound_id IN ?", batch, ids).
			Scan(&rows).Error; err != nil {
			return nil
		}
		for _, r := range rows {
			if m, ok := byInbound[r.InboundId]; ok && m > out[r.Email] {
				out[r.Email] = m
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

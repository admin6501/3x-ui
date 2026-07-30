package service

import (
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
)

// validateFinalMaskRealityCombo refuses a TCP finalmask on a REALITY inbound.
//
// xray-core crashes on the first connection to an inbound configured with both
// (XTLS/Xray-core#6453). The crash takes the whole core down, so it is not one
// inbound degrading — every client on the panel drops, and the core will not
// stay up while the inbound is stored.
//
// Save time is the only cheap place to catch it: the panel validates the
// generated config, and a config with this combination builds perfectly well.
// It fails when traffic arrives.
func validateFinalMaskRealityCombo(inbound *model.Inbound) error {
	if inbound == nil {
		return nil
	}
	if model.StreamHasReality(inbound.StreamSettings) && model.StreamHasTcpFinalMask(inbound.StreamSettings) {
		return common.NewError("a TCP finalmask cannot be combined with REALITY: xray-core crashes on the first connection (XTLS/Xray-core#6453). Remove the TCP mask, or switch the inbound away from REALITY.")
	}
	return nil
}

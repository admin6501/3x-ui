// Package service: admin RBAC service. Lives separately from user.go so the
// existing single-admin login/2FA/LDAP plumbing in user.go stays untouched.
package service

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/crypto"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"

	"gorm.io/gorm"
)

// AdminService provides CRUD + audit-log helpers for admin user accounts.
type AdminService struct{}

// validRoles is the closed set of RBAC roles the panel recognises.
var validRoles = map[string]struct{}{
	model.RoleSuperAdmin: {},
	model.RoleManager:    {},
	model.RoleReseller:   {},
	model.RoleReadonly:   {},
}

// IsValidRole reports whether s is one of the recognised RBAC roles.
func IsValidRole(s string) bool {
	_, ok := validRoles[s]
	return ok
}

// NormalizeAllowedInbounds sanitises an input CSV like " 3, ,7,3 " into "3,7"
// (sorted, deduped, stripped of blanks/non-numerics). Empty result is fine —
// for resellers it means "no inbounds visible" which is the safe default.
func NormalizeAllowedInbounds(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	seen := map[int]struct{}{}
	var ids []int
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil || n <= 0 {
			continue
		}
		if _, dup := seen[n]; dup {
			continue
		}
		seen[n] = struct{}{}
		ids = append(ids, n)
	}
	if len(ids) == 0 {
		return ""
	}
	// Sort ascending for stable storage / equality checks.
	for i := 1; i < len(ids); i++ {
		for j := i; j > 0 && ids[j-1] > ids[j]; j-- {
			ids[j-1], ids[j] = ids[j], ids[j-1]
		}
	}
	parts := make([]string, len(ids))
	for i, n := range ids {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ",")
}

// normalizeQuota clamps a negative quota to 0 (unlimited).
func normalizeQuota(v int64) int64 {
	if v < 0 {
		return 0
	}
	return v
}

// AllowedInboundIDs parses a User.AllowedInbounds CSV into a slice of ids.
func AllowedInboundIDs(u *model.User) []int {
	if u == nil || strings.TrimSpace(u.AllowedInbounds) == "" {
		return nil
	}
	out := make([]int, 0)
	for _, part := range strings.Split(u.AllowedInbounds, ",") {
		if n, err := strconv.Atoi(strings.TrimSpace(part)); err == nil && n > 0 {
			out = append(out, n)
		}
	}
	return out
}

// ListAdmins returns every admin row except passwords.
func (s *AdminService) ListAdmins() ([]model.User, error) {
	db := database.GetDB()
	var users []model.User
	if err := db.Model(&model.User{}).
		Select("id", "username", "role", "allowed_inbounds", "traffic_quota_gb", "client_quota", "clients_created_total", "disabled").
		Order("id ASC").Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

// GetUserByID returns the full user row (including password) by id. Used for
// reseller quota checks where fresh quota/counter values are required.
func (s *AdminService) GetUserByID(id int) (*model.User, error) {
	db := database.GetDB()
	var u model.User
	if err := db.First(&u, id).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// IncrementClientsCreated bumps the reseller's cumulative created-clients
// counter by n. Best-effort: a failure here must not break client creation.
func (s *AdminService) IncrementClientsCreated(userID, n int) {
	if userID <= 0 || n <= 0 {
		return
	}
	db := database.GetDB()
	_ = db.Model(&model.User{}).Where("id = ?", userID).
		UpdateColumn("clients_created_total", gorm.Expr("clients_created_total + ?", n)).Error
}

// ResellerStats is the aggregated usage snapshot for one reseller.
type ResellerStats struct {
	TrafficUsedBytes    int64 `json:"trafficUsedBytes"`
	CurrentClients      int   `json:"currentClients"`
	ClientsCreatedTotal int   `json:"clientsCreatedTotal"`
	TrafficQuotaGB      int64 `json:"trafficQuotaGB"`
	ClientQuota         int   `json:"clientQuota"`
}

// GetResellerStats computes a reseller's total traffic (summed across ALL
// assigned inbounds as a single number) and current distinct client count.
func (s *AdminService) GetResellerStats(u *model.User) ResellerStats {
	stats := ResellerStats{
		ClientsCreatedTotal: u.ClientsCreatedTotal,
		TrafficQuotaGB:      u.TrafficQuotaGB,
		ClientQuota:         u.ClientQuota,
	}
	ids := AllowedInboundIDs(u)
	if len(ids) == 0 {
		return stats
	}
	db := database.GetDB()
	var traffic int64
	db.Model(&model.Inbound{}).Where("id IN ?", ids).
		Select("COALESCE(SUM(up + down), 0)").Scan(&traffic)
	stats.TrafficUsedBytes = traffic
	var current int64
	db.Model(&model.ClientInbound{}).Where("inbound_id IN ?", ids).
		Distinct("client_id").Count(&current)
	stats.CurrentClients = int(current)
	return stats
}

// GetAllResellerStats returns a map of reseller user id -> usage stats, for the
// super-admin admins page.
func (s *AdminService) GetAllResellerStats() (map[int]ResellerStats, error) {
	db := database.GetDB()
	var resellers []model.User
	if err := db.Where("role = ?", model.RoleReseller).Find(&resellers).Error; err != nil {
		return nil, err
	}
	out := make(map[int]ResellerStats, len(resellers))
	for i := range resellers {
		out[resellers[i].Id] = s.GetResellerStats(&resellers[i])
	}
	return out, nil
}

const resellerBytesPerGB = int64(1024 * 1024 * 1024)

// EnforceResellerQuotas disables every assigned inbound of any reseller whose
// total consumption has reached their traffic quota, and re-enables inbounds it
// previously auto-disabled once the reseller recovers (quota raised / reset).
// Over-selling is intentionally allowed — only real consumption matters.
// Returns true if any inbound enable flag flipped (caller should restart xray).
func (s *AdminService) EnforceResellerQuotas(inboundSvc *InboundService) bool {
	db := database.GetDB()
	var resellers []model.User
	if err := db.Where("role = ? AND traffic_quota_gb > 0", model.RoleReseller).Find(&resellers).Error; err != nil {
		return false
	}
	changed := false
	for i := range resellers {
		r := &resellers[i]
		if r.Disabled {
			// A disabled account already has its inbounds down for a different
			// reason; leaving it alone here keeps the quota enforcer from
			// re-enabling them the moment the quota is raised.
			continue
		}
		ids := AllowedInboundIDs(r)
		if len(ids) == 0 {
			continue
		}
		var used int64
		db.Model(&model.Inbound{}).Where("id IN ?", ids).
			Select("COALESCE(SUM(up + down), 0)").Scan(&used)
		exhausted := used >= r.TrafficQuotaGB*resellerBytesPerGB

		if exhausted {
			for _, id := range ids {
				ib, err := inboundSvc.GetInbound(id)
				if err != nil || !ib.Enable {
					continue
				}
				if nr, err := inboundSvc.SetInboundEnable(id, false); err == nil {
					changed = changed || nr
					_ = db.Where("inbound_id = ?", id).
						Delete(&model.ResellerQuotaDisabledInbound{}).Error
					_ = db.Create(&model.ResellerQuotaDisabledInbound{
						InboundId:  id,
						ResellerId: r.Id,
						Reason:     model.ResellerDisableReasonQuota,
					}).Error
					changed = true
				}
			}
			continue
		}

		// Recovered: re-enable only the inbounds this enforcer took down for
		// quota. Rows recorded for a disabled account belong to
		// SetAdminEnabled and must survive a quota recovery.
		var tracked []model.ResellerQuotaDisabledInbound
		if err := db.Where("reseller_id = ? AND reason = ?", r.Id, model.ResellerDisableReasonQuota).
			Find(&tracked).Error; err != nil {
			continue
		}
		for _, row := range tracked {
			if nr, err := inboundSvc.SetInboundEnable(row.InboundId, true); err == nil {
				changed = changed || nr
			}
			_ = db.Where("inbound_id = ?", row.InboundId).
				Delete(&model.ResellerQuotaDisabledInbound{}).Error
			changed = true
		}
	}
	return changed
}

// GetAdmin returns a single admin row (without password).
func (s *AdminService) GetAdmin(id int) (*model.User, error) {
	db := database.GetDB()
	var u model.User
	if err := db.Model(&model.User{}).
		Select("id", "username", "role", "allowed_inbounds").
		Where("id = ?", id).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// CreateAdmin inserts a new admin row. Caller is expected to have already
// verified actor is a super admin via middleware.
func (s *AdminService) CreateAdmin(actor *model.User, username, password, role, allowedInbounds string, trafficQuotaGB int64, clientQuota int) (*model.User, error) {
	if strings.TrimSpace(username) == "" {
		return nil, errors.New("username is required")
	}
	if strings.TrimSpace(password) == "" {
		return nil, errors.New("password is required")
	}
	if !IsValidRole(role) {
		return nil, fmt.Errorf("unknown role %q", role)
	}

	db := database.GetDB()

	// Reject duplicate usernames up front; the users table has no unique index
	// on Username today and we don't want to add one without a migration of
	// existing rows.
	var existing model.User
	err := db.Where("username = ?", username).First(&existing).Error
	switch {
	case err == nil:
		return nil, fmt.Errorf("username %q already exists", username)
	case !database.IsNotFound(err):
		// A failed lookup is not proof the name is free; creating anyway would
		// risk a duplicate account.
		return nil, err
	}

	hashed, err := crypto.HashPasswordAsBcrypt(password)
	if err != nil {
		return nil, err
	}

	u := &model.User{
		Username:        username,
		Password:        hashed,
		Role:            role,
		AllowedInbounds: NormalizeAllowedInbounds(allowedInbounds),
		TrafficQuotaGB:  normalizeQuota(trafficQuotaGB),
		ClientQuota:     int(normalizeQuota(int64(clientQuota))),
	}
	if err := db.Create(u).Error; err != nil {
		return nil, err
	}
	s.logAction(actor, "create_admin", u.Id, u.Username,
		fmt.Sprintf("role=%s, allowedInbounds=[%s]", u.Role, u.AllowedInbounds))
	return u, nil
}

// UpdateAdmin updates role / allowedInbounds / username for the given admin.
// Password changes go through ResetAdminPassword to keep the audit trail
// honest.
func (s *AdminService) UpdateAdmin(actor *model.User, id int, username, role, allowedInbounds string, trafficQuotaGB int64, clientQuota int, inboundSvc *InboundService, xrayService *XrayService) error {
	if !IsValidRole(role) {
		return fmt.Errorf("unknown role %q", role)
	}
	db := database.GetDB()
	var u model.User
	if err := db.First(&u, id).Error; err != nil {
		return err
	}
	// If the only super-admin in the DB is being demoted, refuse — otherwise
	// the panel becomes unmanageable.
	if u.Role == model.RoleSuperAdmin && role != model.RoleSuperAdmin {
		count, err := s.countSuperAdmins()
		if err != nil {
			return err
		}
		if count <= 1 {
			return errors.New("cannot demote the last super_admin")
		}
	}
	updates := map[string]any{
		"role":             role,
		"allowed_inbounds": NormalizeAllowedInbounds(allowedInbounds),
		"traffic_quota_gb": normalizeQuota(trafficQuotaGB),
		"client_quota":     int(normalizeQuota(int64(clientQuota))),
	}
	if strings.TrimSpace(username) != "" && username != u.Username {
		var dup model.User
		err := db.Where("username = ? AND id <> ?", username, id).First(&dup).Error
		switch {
		case err == nil:
			return fmt.Errorf("username %q already exists", username)
		case !database.IsNotFound(err):
			return err
		}
		updates["username"] = username
	}
	if err := db.Model(&model.User{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return err
	}
	// An inbound taken off a reseller — or a reseller turned into another role
	// — would otherwise stay disabled with a tracking row nobody clears.
	released := false
	if role != model.RoleReseller {
		released = s.releaseResellerInbounds(u.Id, nil, inboundSvc)
	} else if dropped := droppedInboundIDs(u.AllowedInbounds, updates["allowed_inbounds"].(string)); len(dropped) > 0 {
		released = s.releaseResellerInbounds(u.Id, dropped, inboundSvc)
	}
	if released && xrayService != nil {
		xrayService.SetToNeedRestart()
	}
	s.logAction(actor, "update_admin", u.Id, u.Username,
		fmt.Sprintf("role=%s, allowedInbounds=[%s]", role, NormalizeAllowedInbounds(allowedInbounds)))
	return nil
}

// droppedInboundIDs returns the ids present in the old CSV but not the new one.
func droppedInboundIDs(oldCSV, newCSV string) []int {
	keep := map[int]struct{}{}
	for _, id := range AllowedInboundIDs(&model.User{AllowedInbounds: newCSV}) {
		keep[id] = struct{}{}
	}
	var dropped []int
	for _, id := range AllowedInboundIDs(&model.User{AllowedInbounds: oldCSV}) {
		if _, ok := keep[id]; !ok {
			dropped = append(dropped, id)
		}
	}
	return dropped
}

// ResetAdminPassword overwrites another admin's password without requiring
// the old one. The controller layer gates this on super-admin; the admin
// being reset is identified by id, not by self-claim.
func (s *AdminService) ResetAdminPassword(actor *model.User, id int, newPassword string) error {
	if strings.TrimSpace(newPassword) == "" {
		return errors.New("password is required")
	}
	hashed, err := crypto.HashPasswordAsBcrypt(newPassword)
	if err != nil {
		return err
	}
	db := database.GetDB()
	var u model.User
	if err := db.First(&u, id).Error; err != nil {
		return err
	}
	if err := db.Model(&model.User{}).Where("id = ?", id).
		Update("password", hashed).Error; err != nil {
		return err
	}
	s.logAction(actor, "reset_password", u.Id, u.Username, "")
	return nil
}

// DeleteAdmin removes an admin row. Refuses to delete the last super-admin
// or the actor's own row (preventing accidental self-lockout).
func (s *AdminService) DeleteAdmin(actor *model.User, id int, inboundSvc *InboundService, xrayService *XrayService) error {
	if actor != nil && actor.Id == id {
		return errors.New("cannot delete your own account; ask another super_admin")
	}
	db := database.GetDB()
	var u model.User
	if err := db.First(&u, id).Error; err != nil {
		return err
	}
	if u.Role == model.RoleSuperAdmin {
		count, err := s.countSuperAdmins()
		if err != nil {
			return err
		}
		if count <= 1 {
			return errors.New("cannot delete the last super_admin")
		}
	}
	if err := db.Delete(&model.User{}, id).Error; err != nil {
		return err
	}
	// Inbounds the panel took down on this reseller's behalf have nobody left
	// to restore them, so hand them back before the account disappears.
	if s.releaseResellerInbounds(u.Id, nil, inboundSvc) && xrayService != nil {
		xrayService.SetToNeedRestart()
	}
	s.logAction(actor, "delete_admin", u.Id, u.Username, fmt.Sprintf("role=%s", u.Role))
	return nil
}

// applyResellerAccountState takes a reseller's assigned inbounds down when
// their account is disabled and brings back exactly those when it is enabled
// again.
//
// Disabling the account alone only stops the reseller logging in — their
// clients keep connecting, which is rarely what an admin means by "disable this
// reseller". Every inbound taken down here is recorded with the "account"
// reason so re-enabling restores precisely what we disabled: an inbound the
// admin had already switched off by hand stays off, and rows the quota
// enforcer owns are left to it.
func (s *AdminService) applyResellerAccountState(u *model.User, enabled bool, inboundSvc *InboundService) bool {
	if u == nil || u.Role != model.RoleReseller || inboundSvc == nil {
		return false
	}
	db := database.GetDB()
	changed := false

	if !enabled {
		for _, id := range AllowedInboundIDs(u) {
			ib, err := inboundSvc.GetInbound(id)
			if err != nil || ib == nil || !ib.Enable {
				// Already off: don't record it, or enabling the account later
				// would switch on an inbound the admin meant to keep down.
				continue
			}
			if _, err := inboundSvc.SetInboundEnable(id, false); err != nil {
				logger.Warning("disable reseller account: inbound", id, "failed:", err)
				continue
			}
			_ = db.Where("inbound_id = ?", id).Delete(&model.ResellerQuotaDisabledInbound{}).Error
			_ = db.Create(&model.ResellerQuotaDisabledInbound{
				InboundId:  id,
				ResellerId: u.Id,
				Reason:     model.ResellerDisableReasonAccount,
			}).Error
			changed = true
		}
		return changed
	}

	var tracked []model.ResellerQuotaDisabledInbound
	if err := db.Where("reseller_id = ? AND reason = ?", u.Id, model.ResellerDisableReasonAccount).
		Find(&tracked).Error; err != nil {
		return false
	}
	for _, row := range tracked {
		if _, err := inboundSvc.SetInboundEnable(row.InboundId, true); err != nil {
			logger.Warning("enable reseller account: inbound", row.InboundId, "failed:", err)
			continue
		}
		changed = true
	}
	_ = db.Where("reseller_id = ? AND reason = ?", u.Id, model.ResellerDisableReasonAccount).
		Delete(&model.ResellerQuotaDisabledInbound{}).Error
	return changed
}

// releaseResellerInbounds re-enables and forgets every inbound the panel took
// down on a reseller's behalf. Used when the reseller loses an inbound or is
// deleted outright — without it those inbounds would stay disabled forever
// with nobody left to restore them.
func (s *AdminService) releaseResellerInbounds(resellerID int, inboundIDs []int, inboundSvc *InboundService) bool {
	if resellerID <= 0 || inboundSvc == nil {
		return false
	}
	db := database.GetDB()
	query := db.Where("reseller_id = ?", resellerID)
	if inboundIDs != nil {
		if len(inboundIDs) == 0 {
			return false
		}
		query = query.Where("inbound_id IN ?", inboundIDs)
	}
	var tracked []model.ResellerQuotaDisabledInbound
	if err := query.Find(&tracked).Error; err != nil {
		return false
	}
	changed := false
	for _, row := range tracked {
		if _, err := inboundSvc.SetInboundEnable(row.InboundId, true); err != nil {
			logger.Warning("release reseller inbound", row.InboundId, "failed:", err)
			continue
		}
		changed = true
	}
	ids := make([]int, 0, len(tracked))
	for _, row := range tracked {
		ids = append(ids, row.InboundId)
	}
	if len(ids) > 0 {
		_ = db.Where("reseller_id = ? AND inbound_id IN ?", resellerID, ids).
			Delete(&model.ResellerQuotaDisabledInbound{}).Error
	}
	return changed
}

// SetAdminEnabled toggles the Disabled flag on any admin account (any role).
// A disabled account can no longer log in, and for a reseller their assigned
// inbounds follow the account state. Guards against locking yourself out and
// against disabling the last enabled super_admin.
func (s *AdminService) SetAdminEnabled(actor *model.User, id int, enabled bool, inboundSvc *InboundService, xrayService *XrayService) error {
	if !enabled && actor != nil && actor.Id == id {
		return errors.New("cannot disable your own account; ask another super_admin")
	}
	db := database.GetDB()
	var u model.User
	if err := db.First(&u, id).Error; err != nil {
		return err
	}
	if !enabled && u.Role == model.RoleSuperAdmin {
		var enabledSupers int64
		if err := db.Model(&model.User{}).
			Where("role = ? AND disabled = ?", model.RoleSuperAdmin, false).
			Count(&enabledSupers).Error; err != nil {
			return err
		}
		if enabledSupers <= 1 {
			return errors.New("cannot disable the last enabled super_admin")
		}
	}
	if err := db.Model(&model.User{}).Where("id = ?", id).
		Update("disabled", !enabled).Error; err != nil {
		return err
	}
	u.Disabled = !enabled
	if s.applyResellerAccountState(&u, enabled, inboundSvc) && xrayService != nil {
		xrayService.SetToNeedRestart()
	}
	action := "enable_admin"
	if !enabled {
		action = "disable_admin"
	}
	s.logAction(actor, action, u.Id, u.Username, fmt.Sprintf("role=%s", u.Role))
	return nil
}

// ResetResellerTraffic zeroes a reseller's measured traffic (the up+down
// counters on every inbound assigned to them, which is exactly what the quota
// is measured against) so consumption starts over from zero. It also re-enables
// every inbound that the quota enforcer had auto-disabled for this reseller and
// asks xray to restart so the changes take effect immediately.
func (s *AdminService) ResetResellerTraffic(actor *model.User, id int, inboundSvc *InboundService, xrayService *XrayService) error {
	db := database.GetDB()
	var u model.User
	if err := db.First(&u, id).Error; err != nil {
		return err
	}
	if u.Role != model.RoleReseller {
		return errors.New("traffic reset is only available for reseller accounts")
	}

	ids := AllowedInboundIDs(&u)
	for _, inboundID := range ids {
		if err := inboundSvc.ResetInboundTraffic(inboundID); err != nil {
			logger.Warning("ResetResellerTraffic: reset inbound", inboundID, "failed:", err)
		}
	}

	// Re-enable inbounds WE auto-disabled for this reseller, then forget the rows.
	var tracked []model.ResellerQuotaDisabledInbound
	if err := db.Where("reseller_id = ?", u.Id).Find(&tracked).Error; err == nil {
		for _, row := range tracked {
			if _, err := inboundSvc.SetInboundEnable(row.InboundId, true); err != nil {
				logger.Warning("ResetResellerTraffic: re-enable inbound", row.InboundId, "failed:", err)
			}
		}
		_ = db.Where("reseller_id = ?", u.Id).Delete(&model.ResellerQuotaDisabledInbound{}).Error
	}

	if xrayService != nil {
		xrayService.SetToNeedRestart()
	}
	s.logAction(actor, "reset_reseller_traffic", u.Id, u.Username,
		fmt.Sprintf("inbounds=%v", ids))
	return nil
}

// ListAuditLogs returns the most recent audit log entries (newest first,
// capped at limit; pass 0/negative for the default of 200).
func (s *AdminService) ListAuditLogs(limit int) ([]model.AdminAuditLog, error) {
	if limit <= 0 {
		limit = 200
	}
	db := database.GetDB()
	var rows []model.AdminAuditLog
	err := db.Model(&model.AdminAuditLog{}).
		Order("id DESC").Limit(limit).Find(&rows).Error
	return rows, err
}

func (s *AdminService) countSuperAdmins() (int64, error) {
	db := database.GetDB()
	var count int64
	err := db.Model(&model.User{}).Where("role = ?", model.RoleSuperAdmin).Count(&count).Error
	return count, err
}

// logAction is a fire-and-forget helper — never blocks or fails the parent
// operation if writing to the log table errors out.
func (s *AdminService) logAction(actor *model.User, action string, targetID int, target, details string) {
	db := database.GetDB()
	row := &model.AdminAuditLog{
		Action:   action,
		TargetId: targetID,
		Target:   target,
		Details:  details,
	}
	if actor != nil {
		row.ActorId = actor.Id
		row.Actor = actor.Username
	}
	_ = db.Create(row).Error
}

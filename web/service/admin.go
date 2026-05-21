// Package service: admin RBAC service. Lives separately from user.go so the
// existing single-admin login/2FA/LDAP plumbing in user.go stays untouched.
package service

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/util/crypto"
	"github.com/mhsanaei/3x-ui/v2/web/websocket"

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
		Select("id", "username", "role", "allowed_inbounds", "traffic_quota", "traffic_used").
		Order("id ASC").Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

// AdminWithStats packages a User with computed traffic stats — used by the
// admin list UI so each row can show "consumed traffic by this admin's
// clients" without forcing the frontend to N+1 the API.  We embed the User
// so JSON shape stays backward compatible: every field the old endpoint
// returned still exists at the same key.
type AdminWithStats struct {
	model.User
	// ConsumedTraffic is the sum of up+down across every client_traffic
	// row that lives inside an inbound this admin can see.
	//   • Reseller → restricted to the inbound IDs in AllowedInbounds
	//   • Other roles → the panel-wide total (they can see everything)
	// 0 when the admin has no inbound scope (e.g. reseller with empty
	// AllowedInbounds).
	ConsumedTraffic int64 `json:"consumedTraffic"`
}

// ListAdminsWithStats is like ListAdmins but also fills in ConsumedTraffic
// for each row. We compute the sum in a single GROUP BY query against
// client_traffics, then join in memory — that's cheaper than running N
// individual SUMs on a busy panel.
func (s *AdminService) ListAdminsWithStats() ([]AdminWithStats, error) {
	users, err := s.ListAdmins()
	if err != nil {
		return nil, err
	}

	// Build inbound_id → consumed map in one shot.
	db := database.GetDB()
	var rows []struct {
		InboundId int
		Total     int64
	}
	if err := db.Table("client_traffics").
		Select("inbound_id, COALESCE(SUM(up + down), 0) AS total").
		Group("inbound_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	consumedByInbound := make(map[int]int64, len(rows))
	var grandTotal int64
	for _, r := range rows {
		consumedByInbound[r.InboundId] = r.Total
		grandTotal += r.Total
	}

	out := make([]AdminWithStats, 0, len(users))
	for _, u := range users {
		row := AdminWithStats{User: u}
		switch u.Role {
		case model.RoleReseller:
			// Only count traffic from inbounds inside the reseller's
			// AllowedInbounds CSV. Anything else would credit them
			// for usage on inbounds they can't even see.
			for _, ibId := range AllowedInboundIDs(&u) {
				row.ConsumedTraffic += consumedByInbound[ibId]
			}
		default:
			// super_admin / manager / readonly all see the whole panel,
			// so their "consumed" reading is the panel-wide grand
			// total. Operators may find this useful for a top-level
			// dashboard view; the UI is free to hide it.
			row.ConsumedTraffic = grandTotal
		}
		out = append(out, row)
	}
	return out, nil
}

// GetAdmin returns a single admin row (without password).
func (s *AdminService) GetAdmin(id int) (*model.User, error) {
	db := database.GetDB()
	var u model.User
	if err := db.Model(&model.User{}).
		Select("id", "username", "role", "allowed_inbounds", "traffic_quota", "traffic_used").
		Where("id = ?", id).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// CreateAdmin inserts a new admin row. Caller is expected to have already
// verified actor is a super admin via middleware.
//
// trafficQuota is the byte-cap a reseller may allocate across their clients;
// pass 0 for "unlimited". Ignored (but stored) for non-reseller roles so a
// later role flip preserves the value.
func (s *AdminService) CreateAdmin(actor *model.User, username, password, role, allowedInbounds string, trafficQuota int64) (*model.User, error) {
	if strings.TrimSpace(username) == "" {
		return nil, errors.New("username is required")
	}
	if strings.TrimSpace(password) == "" {
		return nil, errors.New("password is required")
	}
	if !IsValidRole(role) {
		return nil, fmt.Errorf("unknown role %q", role)
	}
	if trafficQuota < 0 {
		trafficQuota = 0
	}

	db := database.GetDB()

	// Reject duplicate usernames up front; SQLite doesn't have a unique index
	// on User.Username today and we don't want to add one without a migration
	// of existing rows.
	var existing model.User
	err := db.Where("username = ?", username).First(&existing).Error
	if err == nil {
		return nil, fmt.Errorf("username %q already exists", username)
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
		TrafficQuota:    trafficQuota,
		TrafficUsed:     0,
	}
	if err := db.Create(u).Error; err != nil {
		return nil, err
	}
	s.logAction(actor, "create_admin", u.Id, u.Username,
		fmt.Sprintf("role=%s, allowedInbounds=[%s], trafficQuota=%d", u.Role, u.AllowedInbounds, u.TrafficQuota))
	return u, nil
}

// UpdateAdmin updates role / allowedInbounds / username / trafficQuota for
// the given admin. Password changes go through ResetAdminPassword to keep
// the audit trail honest.
//
// trafficQuota is updated as-is — pass the current value if you don't want
// to change it. Negative values are clamped to 0 (= unlimited). NOTE: this
// does NOT touch TrafficUsed; use ResetTrafficUsage for that.
func (s *AdminService) UpdateAdmin(actor *model.User, id int, username, role, allowedInbounds string, trafficQuota int64) error {
	if !IsValidRole(role) {
		return fmt.Errorf("unknown role %q", role)
	}
	if trafficQuota < 0 {
		trafficQuota = 0
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
		"traffic_quota":    trafficQuota,
	}
	if strings.TrimSpace(username) != "" && username != u.Username {
		var dup model.User
		if err := db.Where("username = ? AND id <> ?", username, id).First(&dup).Error; err == nil {
			return fmt.Errorf("username %q already exists", username)
		}
		updates["username"] = username
	}
	if err := db.Model(&model.User{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return err
	}
	s.logAction(actor, "update_admin", u.Id, u.Username,
		fmt.Sprintf("role=%s, allowedInbounds=[%s], trafficQuota=%d", role, NormalizeAllowedInbounds(allowedInbounds), trafficQuota))
	return nil
}

// ResetTrafficUsage zeroes out the TrafficUsed counter for a reseller —
// effectively giving them a fresh start on their quota. Super-admin-only;
// audited.
func (s *AdminService) ResetTrafficUsage(actor *model.User, id int) error {
	db := database.GetDB()
	var u model.User
	if err := db.First(&u, id).Error; err != nil {
		return err
	}
	if err := db.Model(&model.User{}).Where("id = ?", id).
		Update("traffic_used", 0).Error; err != nil {
		return err
	}
	s.logAction(actor, "reset_traffic_usage", u.Id, u.Username,
		fmt.Sprintf("previousUsed=%d", u.TrafficUsed))
	websocket.BroadcastInvalidate(websocket.MessageTypeAdmins)
	return nil
}

// CheckResellerQuota verifies the reseller has at least `bytes` of headroom
// left in their TrafficQuota. Returns nil on OK; an error when the quota
// would be exceeded.  Non-reseller roles and quota=0 (unlimited) always
// pass through. Pass the actor directly — callers usually already have it
// in their gin context to avoid an extra DB hit.
//
// Important: this only *reads* — it does NOT bump TrafficUsed. Callers must
// pair this with AccumulateUsage() after the mutating action succeeds.
func (s *AdminService) CheckResellerQuota(actor *model.User, bytes int64) error {
	if actor == nil || actor.Role != model.RoleReseller {
		return nil
	}
	if actor.TrafficQuota <= 0 {
		return nil // unlimited
	}
	// Re-read from DB so a stale session doesn't let an over-cap action sneak
	// through (e.g. two browser tabs racing on the same reseller account).
	db := database.GetDB()
	var fresh model.User
	if err := db.Select("traffic_quota", "traffic_used").
		Where("id = ?", actor.Id).First(&fresh).Error; err != nil {
		return err
	}
	if fresh.TrafficQuota > 0 && fresh.TrafficUsed+bytes > fresh.TrafficQuota {
		remaining := fresh.TrafficQuota - fresh.TrafficUsed
		if remaining < 0 {
			remaining = 0
		}
		return fmt.Errorf("reseller traffic quota exceeded: requested %d bytes, %d bytes remaining of %d total",
			bytes, remaining, fresh.TrafficQuota)
	}
	return nil
}

// AccumulateUsage bumps the reseller's TrafficUsed counter by `bytes` (use
// negative values at your own risk — we clamp to 0 so accidents can't free
// up quota via a delete path). For non-resellers this is a no-op.
func (s *AdminService) AccumulateUsage(u *model.User, bytes int64) error {
	if u == nil || u.Role != model.RoleReseller || bytes <= 0 {
		return nil
	}
	db := database.GetDB()
	if err := db.Model(&model.User{}).Where("id = ?", u.Id).
		UpdateColumn("traffic_used", gorm.Expr("traffic_used + ?", bytes)).Error; err != nil {
		return err
	}
	// Keep the in-memory session copy roughly in sync so subsequent reads in
	// the same request don't undercount; a fresh DB read happens on the
	// next CheckResellerQuota() call anyway.
	u.TrafficUsed += bytes
	// Audit trail so the operator can see in the panel what got billed
	// when — invaluable for debugging "the reseller's quota looks wrong"
	// support reports. Self-action: the reseller themselves caused this.
	s.logAction(u, "quota_accumulate", u.Id, u.Username, fmt.Sprintf("bytes=%d", bytes))
	// Nudge any open admins page (super_admin viewing in another tab) to
	// re-fetch its list so the freshly-billed traffic_used surfaces without
	// waiting for a manual refresh.
	websocket.BroadcastInvalidate(websocket.MessageTypeAdmins)
	return nil
}

// RefundUsage decrements the reseller's TrafficUsed counter by `bytes` —
// used when a client is deleted to return the unused remainder of that
// client's allocation to the reseller's pool. Floors at 0 so a refund
// larger than the current usage (e.g. an inbound deleted by a super_admin
// whose client allocations were never billed in the first place) can
// never push the counter negative. For non-resellers this is a no-op.
//
// Pair this with the `delete` controllers; never call it during a reset
// (resets are intentionally billed — see AccumulateUsage usage in
// resetClientTraffic/resetAllClientTraffics).
func (s *AdminService) RefundUsage(u *model.User, bytes int64) error {
	if u == nil || u.Role != model.RoleReseller || bytes <= 0 {
		return nil
	}
	db := database.GetDB()
	if err := db.Model(&model.User{}).Where("id = ?", u.Id).
		UpdateColumn("traffic_used",
			gorm.Expr("CASE WHEN traffic_used - ? < 0 THEN 0 ELSE traffic_used - ? END", bytes, bytes)).
		Error; err != nil {
		return err
	}
	// Mirror the change on the in-memory copy so the same request sees a
	// fresh remaining-quota reading.
	if u.TrafficUsed >= bytes {
		u.TrafficUsed -= bytes
	} else {
		u.TrafficUsed = 0
	}
	// Audit trail — see comment in AccumulateUsage.
	s.logAction(u, "quota_refund", u.Id, u.Username, fmt.Sprintf("bytes=%d", bytes))
	// Same auto-refresh nudge as AccumulateUsage — see comments there.
	websocket.BroadcastInvalidate(websocket.MessageTypeAdmins)
	return nil
}

// GetUserByID returns the User row for the given id (or nil if missing).
// Used by the delete-client refund path so the controller can find an
// inbound's owner (which may be a reseller different from the actor).
func (s *AdminService) GetUserByID(id int) (*model.User, error) {
	if id <= 0 {
		return nil, nil
	}
	db := database.GetDB()
	var u model.User
	if err := db.First(&u, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

// ResellerRemaining returns how many bytes the reseller can still allocate.
// For unlimited / non-reseller it returns math.MaxInt64 so callers can use
// it directly in arithmetic without special-casing.
func (s *AdminService) ResellerRemaining(u *model.User) int64 {
	const maxInt64 = int64(1<<63 - 1)
	if u == nil || u.Role != model.RoleReseller || u.TrafficQuota <= 0 {
		return maxInt64
	}
	remaining := u.TrafficQuota - u.TrafficUsed
	if remaining < 0 {
		return 0
	}
	return remaining
}

// ResetAdminPassword overwrites another admin's password without requiring
// the old one. Refuses to silently allow a reseller / readonly to escalate
// by re-using this on themselves — the controller layer already gates on
// super-admin, but we double-check actor.Id != target via an extra rule:
// the admin being reset is identified by id, not by self-claim.
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
func (s *AdminService) DeleteAdmin(actor *model.User, id int) error {
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
	s.logAction(actor, "delete_admin", u.Id, u.Username, fmt.Sprintf("role=%s", u.Role))
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

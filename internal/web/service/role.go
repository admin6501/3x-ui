// Package service: custom admin roles. The four built-in roles are hard-coded
// permission sets; a custom role is a named set an admin assembles themselves.
// Everything that gates behaviour resolves through PermissionsForRole, so a
// custom role is enforced by exactly the same checks as a built-in one.
package service

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/session"
)

// builtinPermissions is the authoritative statement of what each built-in role
// may do. It reproduces the behaviour the panel had before custom roles
// existed — super_admin everything, manager everything but panel-wide config,
// reseller scoped to its own inbounds' clients, readonly nothing but reads.
var builtinPermissions = map[string][]string{
	model.RoleSuperAdmin: {
		model.PermInboundsView, model.PermInboundsManage,
		model.PermClientsView, model.PermClientsManage,
		model.PermPlansManage, model.PermGroupsManage, model.PermHostsManage,
		model.PermNodesManage, model.PermSettingsManage, model.PermXrayManage,
		model.PermAdminsManage,
	},
	model.RoleManager: {
		model.PermInboundsView, model.PermInboundsManage,
		model.PermClientsView, model.PermClientsManage,
		model.PermPlansManage, model.PermGroupsManage, model.PermHostsManage,
	},
	model.RoleReseller: {
		model.PermInboundsView,
		model.PermClientsView, model.PermClientsManage,
		model.PermGroupsManage,
		model.PermInboundsScoped,
	},
	model.RoleReadonly: {
		model.PermInboundsView, model.PermClientsView,
	},
}

var (
	// roleCache holds the custom roles by id. Permission checks run on every
	// request, so they must not hit the database each time.
	roleCache   map[int]model.AdminRole
	roleCacheMu sync.RWMutex
)

// AdminRoleService manages admin-defined roles.
type AdminRoleService struct{}

var (
	ErrRoleNameRequired = errors.New("role name is required")
	ErrRoleNameTaken    = errors.New("a role with this name already exists")
	ErrRoleInUse        = errors.New("this role is still assigned to one or more admins")
	ErrRoleNotFound     = errors.New("role not found")
)

// normalizePermissions keeps only recognised keys, deduped and in the panel's
// canonical order, so the stored CSV is stable and comparable.
func normalizePermissions(perms []string) string {
	want := make(map[string]struct{}, len(perms))
	for _, p := range perms {
		p = strings.TrimSpace(p)
		if p != "" && model.IsKnownPermission(p) {
			want[p] = struct{}{}
		}
	}
	out := make([]string, 0, len(want))
	for _, p := range model.AllPermissions {
		if _, ok := want[p]; ok {
			out = append(out, p)
		}
	}
	return strings.Join(out, ",")
}

// ReloadRoleCache re-reads every custom role from the database. Called at
// startup and after any role write.
func ReloadRoleCache() {
	var rows []model.AdminRole
	if err := database.GetDB().Model(&model.AdminRole{}).Find(&rows).Error; err != nil {
		return
	}
	next := make(map[int]model.AdminRole, len(rows))
	for _, r := range rows {
		next[r.Id] = r
	}
	roleCacheMu.Lock()
	roleCache = next
	roleCacheMu.Unlock()
}

func cachedRole(id int) (model.AdminRole, bool) {
	roleCacheMu.RLock()
	r, ok := roleCache[id]
	roleCacheMu.RUnlock()
	if ok {
		return r, true
	}
	// A role created by another panel process (or before the first load) is
	// not in the cache yet — fall back to the database once, then cache it.
	var row model.AdminRole
	if err := database.GetDB().First(&row, id).Error; err != nil {
		return model.AdminRole{}, false
	}
	roleCacheMu.Lock()
	if roleCache == nil {
		roleCache = map[int]model.AdminRole{}
	}
	roleCache[id] = row
	roleCacheMu.Unlock()
	return row, true
}

// PermissionsForRole resolves any role string — built-in or "custom:<id>" —
// into the set of permissions it grants. An unknown role grants nothing, which
// is the safe direction: the account can log in but cannot do anything.
func PermissionsForRole(role string) map[string]bool {
	role = strings.TrimSpace(role)
	if role == "" {
		// Pre-RBAC sessions predate the role column; they are super_admin, the
		// same assumption the session layer makes.
		role = model.RoleSuperAdmin
	}
	out := make(map[string]bool, len(model.AllPermissions))
	if perms, ok := builtinPermissions[role]; ok {
		for _, p := range perms {
			out[p] = true
		}
		return out
	}
	if id, ok := model.ParseCustomRole(role); ok {
		if r, found := cachedRole(id); found {
			for _, p := range r.PermissionList() {
				out[p] = true
			}
		}
	}
	return out
}

// IsScopedRole reports whether a role restricts the account to its assigned
// inbounds — the built-in reseller, or a custom role holding the same
// permission. Everything the panel does "for resellers" keys off this.
func IsScopedRole(role string) bool {
	return PermissionsForRole(role)[model.PermInboundsScoped]
}

// ScopedRoleNames lists every role string that is scoped, for the queries that
// need to select all reseller-like accounts at once.
func ScopedRoleNames() []string {
	out := []string{}
	for role := range builtinPermissions {
		if IsScopedRole(role) {
			out = append(out, role)
		}
	}
	roleCacheMu.RLock()
	ids := make([]int, 0, len(roleCache))
	for id := range roleCache {
		ids = append(ids, id)
	}
	roleCacheMu.RUnlock()
	sort.Ints(ids)
	for _, id := range ids {
		name := model.CustomRolePrefix + strconv.Itoa(id)
		if IsScopedRole(name) {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// RoleDisplayName returns the name to show for a role string. Built-ins keep
// their key (the frontend translates those); a custom role reports its name.
func RoleDisplayName(role string) string {
	if id, ok := model.ParseCustomRole(role); ok {
		if r, found := cachedRole(id); found {
			return r.Name
		}
	}
	return role
}

// RoleExists reports whether a role string is assignable: one of the built-ins,
// or a custom role that actually exists.
func RoleExists(role string) bool {
	if IsValidRole(role) {
		return true
	}
	if id, ok := model.ParseCustomRole(role); ok {
		_, found := cachedRole(id)
		return found
	}
	return false
}

// List returns every custom role, oldest first.
func (s *AdminRoleService) List() ([]model.AdminRole, error) {
	var rows []model.AdminRole
	if err := database.GetDB().Model(&model.AdminRole{}).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// Create adds a custom role. Names are unique and trimmed; permissions are
// filtered down to the recognised set.
func (s *AdminRoleService) Create(name string, perms []string) (*model.AdminRole, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrRoleNameRequired
	}
	db := database.GetDB()
	var count int64
	if err := db.Model(&model.AdminRole{}).Where("name = ?", name).Count(&count).Error; err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, ErrRoleNameTaken
	}
	row := &model.AdminRole{Name: name, Permissions: normalizePermissions(perms)}
	if err := db.Create(row).Error; err != nil {
		return nil, err
	}
	ReloadRoleCache()
	return row, nil
}

// Update renames a role and/or replaces its permission set. Accounts holding
// the role pick the change up on their next request.
func (s *AdminRoleService) Update(id int, name string, perms []string) (*model.AdminRole, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrRoleNameRequired
	}
	db := database.GetDB()
	var row model.AdminRole
	if err := db.First(&row, id).Error; err != nil {
		return nil, ErrRoleNotFound
	}
	var count int64
	if err := db.Model(&model.AdminRole{}).Where("name = ? AND id <> ?", name, id).Count(&count).Error; err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, ErrRoleNameTaken
	}
	row.Name = name
	row.Permissions = normalizePermissions(perms)
	if err := db.Model(&model.AdminRole{}).Where("id = ?", id).
		Updates(map[string]any{"name": row.Name, "permissions": row.Permissions}).Error; err != nil {
		return nil, err
	}
	ReloadRoleCache()
	return &row, nil
}

// Delete removes a role, refusing while any admin still holds it — otherwise
// those accounts would be left pointing at a role that grants nothing.
func (s *AdminRoleService) Delete(id int) error {
	db := database.GetDB()
	var row model.AdminRole
	if err := db.First(&row, id).Error; err != nil {
		return ErrRoleNotFound
	}
	var holders int64
	if err := db.Model(&model.User{}).Where("role = ?", model.CustomRolePrefix+strconv.Itoa(id)).Count(&holders).Error; err != nil {
		return err
	}
	if holders > 0 {
		return ErrRoleInUse
	}
	if err := db.Delete(&model.AdminRole{}, id).Error; err != nil {
		return err
	}
	ReloadRoleCache()
	return nil
}

// The panel wires the permission lookup into the session layer as soon as this
// package is linked, so every gate resolves roles the same way without the
// session package having to know about the database.
func init() {
	session.SetPermissionResolver(PermissionsForRole)
}

package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// TestBuiltinPermissionsAreUnchanged pins the four built-in roles to exactly
// what they could do before permissions existed. Custom roles are enforced by
// the same gates, so a slip here silently re-grants or revokes access
// panel-wide.
func TestBuiltinPermissionsAreUnchanged(t *testing.T) {
	setupBulkDB(t)
	cases := []struct {
		role  string
		grant []string
		deny  []string
	}{
		{
			role:  model.RoleSuperAdmin,
			grant: []string{model.PermSettingsManage, model.PermXrayManage, model.PermAdminsManage, model.PermInboundsManage, model.PermNodesManage},
			deny:  []string{model.PermInboundsScoped},
		},
		{
			role:  model.RoleManager,
			grant: []string{model.PermInboundsManage, model.PermClientsManage, model.PermPlansManage, model.PermHostsManage},
			deny:  []string{model.PermSettingsManage, model.PermXrayManage, model.PermAdminsManage, model.PermNodesManage, model.PermInboundsScoped},
		},
		{
			role:  model.RoleReseller,
			grant: []string{model.PermInboundsView, model.PermClientsView, model.PermClientsManage, model.PermInboundsScoped},
			deny:  []string{model.PermInboundsManage, model.PermPlansManage, model.PermSettingsManage, model.PermAdminsManage, model.PermNodesManage, model.PermHostsManage},
		},
		{
			role:  model.RoleReadonly,
			grant: []string{model.PermInboundsView, model.PermClientsView},
			deny:  []string{model.PermClientsManage, model.PermInboundsManage, model.PermSettingsManage, model.PermInboundsScoped},
		},
	}
	for _, tc := range cases {
		perms := PermissionsForRole(tc.role)
		for _, p := range tc.grant {
			if !perms[p] {
				t.Errorf("%s should hold %s", tc.role, p)
			}
		}
		for _, p := range tc.deny {
			if perms[p] {
				t.Errorf("%s must not hold %s", tc.role, p)
			}
		}
	}

	// A legacy session predating the role column is super_admin, matching the
	// session layer's own fallback.
	if !PermissionsForRole("")[model.PermAdminsManage] {
		t.Error("an empty role must resolve to super_admin")
	}
	// Anything unrecognised grants nothing — fail closed.
	if len(PermissionsForRole("custom:9999")) != 0 {
		t.Error("a role that does not exist must grant no permissions")
	}
}

// TestCustomRoleResolvesThroughTheSameGates is the whole feature in one test:
// a named role with a hand-picked permission set behaves like a built-in.
func TestCustomRoleResolvesThroughTheSameGates(t *testing.T) {
	setupBulkDB(t)
	svc := &AdminRoleService{}

	role, err := svc.Create("Support", []string{model.PermClientsView, model.PermClientsManage, "not.a.permission"})
	if err != nil {
		t.Fatalf("create role: %v", err)
	}
	if role.Permissions != "clients.view,clients.manage" {
		t.Errorf("permissions = %q, want the known keys only, in canonical order", role.Permissions)
	}

	key := model.CustomRolePrefix + itoa(role.Id)
	perms := PermissionsForRole(key)
	if !perms[model.PermClientsManage] || perms[model.PermSettingsManage] {
		t.Errorf("custom role resolved to %v", perms)
	}
	if !RoleExists(key) {
		t.Error("an existing custom role must be assignable")
	}
	if got := RoleDisplayName(key); got != "Support" {
		t.Errorf("display name = %q, want Support", got)
	}
	if IsScopedRole(key) {
		t.Error("a role without inbounds.scoped must not be scoped")
	}

	// Editing the permission set takes effect without a restart.
	if _, err := svc.Update(role.Id, "Support L2", []string{model.PermInboundsView, model.PermInboundsScoped}); err != nil {
		t.Fatalf("update role: %v", err)
	}
	if perms := PermissionsForRole(key); perms[model.PermClientsManage] {
		t.Error("a revoked permission must stop resolving")
	}
	if !IsScopedRole(key) {
		t.Error("granting inbounds.scoped must make the role scoped")
	}
	if got := RoleDisplayName(key); got != "Support L2" {
		t.Errorf("display name = %q, want the new name", got)
	}
	// The scoped custom role has to show up wherever the panel selects all
	// reseller-like accounts, or its quotas would never be enforced.
	var found bool
	for _, name := range ScopedRoleNames() {
		if name == key {
			found = true
		}
	}
	if !found {
		t.Errorf("ScopedRoleNames() = %v, want it to include %s", ScopedRoleNames(), key)
	}
}

// TestDuplicateRoleNamesAreRejected keeps the role list unambiguous.
func TestDuplicateRoleNamesAreRejected(t *testing.T) {
	setupBulkDB(t)
	svc := &AdminRoleService{}

	first, err := svc.Create("Billing", []string{model.PermClientsView})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Create("  Billing  ", []string{model.PermClientsView}); err != ErrRoleNameTaken {
		t.Errorf("duplicate name error = %v, want ErrRoleNameTaken", err)
	}
	if _, err := svc.Create("   ", nil); err != ErrRoleNameRequired {
		t.Errorf("blank name error = %v, want ErrRoleNameRequired", err)
	}

	second, err := svc.Create("Billing L2", []string{model.PermClientsView})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	if _, err := svc.Update(second.Id, "Billing", nil); err != ErrRoleNameTaken {
		t.Errorf("rename onto an existing name error = %v, want ErrRoleNameTaken", err)
	}
	if _, err := svc.Update(first.Id, "Billing", []string{model.PermClientsView}); err != nil {
		t.Errorf("renaming a role to its own name should be allowed, got %v", err)
	}
}

// TestRoleInUseCannotBeDeleted stops an account from being stranded on a role
// that no longer exists — which would silently strip every permission it had.
func TestRoleInUseCannotBeDeleted(t *testing.T) {
	setupBulkDB(t)
	svc := &AdminRoleService{}
	adminSvc := &AdminService{}

	role, err := svc.Create("Ops", []string{model.PermInboundsView})
	if err != nil {
		t.Fatalf("create role: %v", err)
	}
	key := model.CustomRolePrefix + itoa(role.Id)

	actor := &model.User{Id: 1, Username: "admin", Role: model.RoleSuperAdmin}
	u, err := adminSvc.CreateAdmin(actor, "ops1", "pw", key, "", 0, 0)
	if err != nil {
		t.Fatalf("assign custom role: %v", err)
	}

	if err := svc.Delete(role.Id); err != ErrRoleInUse {
		t.Fatalf("delete while assigned = %v, want ErrRoleInUse", err)
	}

	// Move the account off the role, and deletion goes through.
	if err := database.GetDB().Model(&model.User{}).Where("id = ?", u.Id).
		Update("role", model.RoleReadonly).Error; err != nil {
		t.Fatalf("reassign: %v", err)
	}
	if err := svc.Delete(role.Id); err != nil {
		t.Fatalf("delete after reassign: %v", err)
	}
	if RoleExists(key) {
		t.Error("a deleted role must not remain assignable")
	}
}

// TestUnknownRoleIsNotAssignable stops a typo from creating an account that
// silently resolves to no permissions at all.
func TestUnknownRoleIsNotAssignable(t *testing.T) {
	setupBulkDB(t)
	adminSvc := &AdminService{}
	actor := &model.User{Id: 1, Username: "admin", Role: model.RoleSuperAdmin}

	if _, err := adminSvc.CreateAdmin(actor, "ghost", "pw", "custom:404", "", 0, 0); err == nil {
		t.Error("creating an admin with a non-existent custom role should fail")
	}
	if _, err := adminSvc.CreateAdmin(actor, "ghost2", "pw", "wizard", "", 0, 0); err == nil {
		t.Error("creating an admin with an unknown built-in role should fail")
	}
}

// Permission keys the panel recognises, mirroring model.AllPermissions on the
// server. The SPA uses them to decide what to render; the server enforces the
// same set on every request, so a tampered client gains nothing.
export const PERMISSIONS = [
  'inbounds.view',
  'inbounds.manage',
  'clients.view',
  'clients.manage',
  'plans.manage',
  'groups.manage',
  'hosts.manage',
  'nodes.manage',
  'settings.manage',
  'xray.manage',
  'admins.manage',
  'inbounds.scoped',
] as const;

export type Permission = (typeof PERMISSIONS)[number];

// Permission sets of the four built-in roles, kept in step with
// builtinPermissions in internal/web/service/role.go. Used only as a fallback
// for a page loaded before window.X_UI_PERMS existed (e.g. a stale tab).
const BUILTIN: Record<string, Permission[]> = {
  super_admin: [
    'inbounds.view', 'inbounds.manage', 'clients.view', 'clients.manage',
    'plans.manage', 'groups.manage', 'hosts.manage', 'nodes.manage',
    'settings.manage', 'xray.manage', 'admins.manage',
  ],
  manager: [
    'inbounds.view', 'inbounds.manage', 'clients.view', 'clients.manage',
    'plans.manage', 'groups.manage', 'hosts.manage',
  ],
  reseller: ['inbounds.view', 'clients.view', 'clients.manage', 'groups.manage', 'inbounds.scoped'],
  readonly: ['inbounds.view', 'clients.view'],
};

/** The permissions granted to the logged-in account. */
export function currentPermissions(): Set<string> {
  if (typeof window === 'undefined') return new Set(BUILTIN.super_admin);
  const injected = window.X_UI_PERMS;
  if (Array.isArray(injected)) return new Set(injected);
  const role = window.X_UI_ROLE || 'super_admin';
  return new Set(BUILTIN[role] ?? BUILTIN.super_admin);
}

/** Whether the logged-in account holds a permission. */
export function can(perm: Permission): boolean {
  return currentPermissions().has(perm);
}

/**
 * Whether the account is restricted to its assigned inbounds — the built-in
 * reseller role and any custom role carrying the same permission. Prefer this
 * over comparing window.X_UI_ROLE to 'reseller'.
 */
export function isScoped(): boolean {
  return can('inbounds.scoped');
}

/** Display name of the current role: the admin-chosen name for a custom role. */
export function currentRoleName(): string {
  if (typeof window === 'undefined') return 'super_admin';
  return window.X_UI_ROLE_NAME || window.X_UI_ROLE || 'super_admin';
}

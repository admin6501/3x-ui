import { afterEach, describe, expect, it } from 'vitest';

import { can, currentPermissions, currentRoleName, isScoped, PERMISSIONS } from '@/lib/permissions';

function setWindow(values: Partial<Window>) {
  Object.assign(window, values);
}

afterEach(() => {
  delete (window as { X_UI_PERMS?: string[] }).X_UI_PERMS;
  delete (window as { X_UI_ROLE?: string }).X_UI_ROLE;
  delete (window as { X_UI_ROLE_NAME?: string }).X_UI_ROLE_NAME;
});

describe('permissions', () => {
  it('uses the set the server injected, whatever the role is called', () => {
    setWindow({ X_UI_ROLE: 'custom:3', X_UI_PERMS: ['clients.view', 'clients.manage'] });
    expect(can('clients.manage')).toBe(true);
    expect(can('settings.manage')).toBe(false);
    expect(isScoped()).toBe(false);
  });

  it('treats a custom role carrying inbounds.scoped as a reseller', () => {
    setWindow({ X_UI_ROLE: 'custom:4', X_UI_PERMS: ['clients.view', 'inbounds.scoped'] });
    expect(isScoped()).toBe(true);
  });

  // A page served before X_UI_PERMS existed (a tab left open across an upgrade)
  // must still gate correctly rather than falling open.
  it('falls back to the built-in table when no permissions were injected', () => {
    setWindow({ X_UI_ROLE: 'readonly' });
    expect(can('clients.view')).toBe(true);
    expect(can('clients.manage')).toBe(false);

    setWindow({ X_UI_ROLE: 'reseller' });
    expect(isScoped()).toBe(true);
    expect(can('inbounds.manage')).toBe(false);
  });

  it('reports the admin-chosen name for a custom role', () => {
    setWindow({ X_UI_ROLE: 'custom:3', X_UI_ROLE_NAME: 'Support' });
    expect(currentRoleName()).toBe('Support');
    setWindow({ X_UI_ROLE: 'manager', X_UI_ROLE_NAME: 'manager' });
    expect(currentRoleName()).toBe('manager');
  });

  it('grants nothing for an empty injected set', () => {
    setWindow({ X_UI_ROLE: 'custom:9', X_UI_PERMS: [] });
    expect(currentPermissions().size).toBe(0);
    for (const p of PERMISSIONS) expect(can(p)).toBe(false);
  });
});

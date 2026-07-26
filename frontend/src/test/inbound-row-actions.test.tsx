import { describe, it, expect } from 'vitest';

import { buildRowActionsMenu } from '@/pages/inbounds/list/RowActions';
import type { DBInboundRecord } from '@/pages/inbounds/list/types';

const t = (k: string) => k;

function record(over: Partial<DBInboundRecord> = {}): DBInboundRecord {
  return {
    id: 1,
    enable: true,
    remark: 'inbound-1',
    subSortIndex: 0,
    port: 443,
    protocol: 'vless',
    up: 0,
    down: 0,
    total: 0,
    expiryTime: 0,
    _expiryTime: null,
    settings: { clients: [] },
    streamSettings: {},
    ...over,
  };
}

function keysOf(items: ReturnType<typeof buildRowActionsMenu>): string[] {
  return (items ?? [])
    .map((item) => (item && 'key' in item ? String(item.key) : ''))
    .filter(Boolean);
}

function labelOf(items: ReturnType<typeof buildRowActionsMenu>, key: string): string {
  const item = (items ?? []).find((i) => i && 'key' in i && i.key === key) as { label?: unknown } | undefined;
  return String(item?.label ?? '');
}

describe('inbound row actions — delete expired clients', () => {
  it('offers the action on a multi-user inbound that has clients', () => {
    const items = buildRowActionsMenu({ record: record(), subEnable: false, t, hasClients: true });
    const keys = keysOf(items);
    expect(keys).toContain('delDepletedClients');
    // Sits with the other destructive client actions, above "delete all".
    expect(keys.indexOf('delDepletedClients')).toBeLessThan(keys.indexOf('delAllClients'));
  });

  it('hides the action when the inbound has no clients', () => {
    const items = buildRowActionsMenu({ record: record(), subEnable: false, t, hasClients: false });
    expect(keysOf(items)).not.toContain('delDepletedClients');
  });

  it('hides the action on single-user protocols', () => {
    const items = buildRowActionsMenu({
      record: record({ protocol: 'wireguard', isWireguard: true }),
      subEnable: false,
      t,
      hasClients: true,
    });
    expect(keysOf(items)).not.toContain('delDepletedClients');
  });

  it('hides the action for read-only (reseller) roles', () => {
    const items = buildRowActionsMenu({ record: record(), subEnable: false, t, hasClients: true, readOnly: true });
    expect(keysOf(items)).not.toContain('delDepletedClients');
  });

  it('appends the ended-client count to the label when the rollup reports one', () => {
    const withCount = buildRowActionsMenu({ record: record(), subEnable: false, t, hasClients: true, depletedCount: 3 });
    expect(labelOf(withCount, 'delDepletedClients')).toBe('pages.inbounds.delDepletedClients (3)');

    const withoutCount = buildRowActionsMenu({ record: record(), subEnable: false, t, hasClients: true });
    expect(labelOf(withoutCount, 'delDepletedClients')).toBe('pages.inbounds.delDepletedClients');
  });
});

// @vitest-environment jsdom
// RandomUtil.randomUUID reads window.location, so this file needs a DOM env
// even though it only exercises plain functions.
import { describe, expect, it } from 'vitest';

import {
  migrateClientsToProtocol,
  protocolSupportsClients,
} from '@/lib/xray/protocol-migration';

// The migrated client is asserted field-by-field, so read it back as a plain
// record instead of the per-protocol union.
const first = (clients: unknown): Record<string, unknown> =>
  (clients as unknown as Record<string, unknown>[])[0];

const vlessClient = {
  id: '11111111-2222-4333-8444-555555555555',
  email: 'user1',
  flow: 'xtls-rprx-vision',
  limitIp: 3,
  totalGB: 1024,
  expiryTime: 1750000000000,
  enable: false,
  tgId: 42,
  subId: 'sub123',
  comment: 'note',
  reset: 7,
  created_at: 1700000000000,
  updated_at: 1710000000000,
};

describe('protocolSupportsClients', () => {
  it('marks the client-bearing protocols', () => {
    for (const p of ['vless', 'vmess', 'trojan', 'shadowsocks', 'hysteria']) {
      expect(protocolSupportsClients(p)).toBe(true);
    }
    for (const p of ['http', 'mixed', 'tun', 'tunnel', 'wireguard', 'mtproto']) {
      expect(protocolSupportsClients(p)).toBe(false);
    }
  });
});

describe('migrateClientsToProtocol', () => {
  it('keeps identity and quota fields when switching vless -> trojan', () => {
    const migrated = first(migrateClientsToProtocol('trojan', [vlessClient], { fromProtocol: 'vless' }));
    expect(migrated.email).toBe('user1');
    expect(migrated.subId).toBe('sub123');
    expect(migrated.limitIp).toBe(3);
    expect(migrated.totalGB).toBe(1024);
    expect(migrated.expiryTime).toBe(1750000000000);
    expect(migrated.enable).toBe(false);
    expect(migrated.tgId).toBe(42);
    expect(migrated.comment).toBe('note');
    expect(migrated.reset).toBe(7);
    expect(migrated.created_at).toBe(1700000000000);
    expect(migrated.updated_at).toBe(1710000000000);
    expect(typeof migrated.password).toBe('string');
    expect((migrated.password as string).length).toBeGreaterThan(0);
    expect(migrated.id).toBeUndefined();
  });

  it('reuses a valid uuid when switching trojan -> vless and clears flow', () => {
    const trojan = { password: 'pw123456', email: 'user2', id: vlessClient.id };
    const migrated = first(migrateClientsToProtocol('vless', [trojan], { fromProtocol: 'trojan' }));
    expect(migrated.id).toBe(vlessClient.id);
    expect(migrated.flow).toBe('');
    expect(migrated.email).toBe('user2');
  });

  it('generates a uuid when the source has none', () => {
    const migrated = first(migrateClientsToProtocol('vmess', [{ password: 'pw', email: 'u' }]));
    expect(String(migrated.id)).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i);
  });

  it('carries the trojan password into hysteria auth', () => {
    const migrated = first(migrateClientsToProtocol('hysteria', [{ password: 'pw123456', email: 'u' }], {
      fromProtocol: 'trojan',
    }));
    expect(migrated.auth).toBe('pw123456');
  });

  it('regenerates the shadowsocks key instead of reusing a foreign password', () => {
    const migrated = first(migrateClientsToProtocol('shadowsocks', [{ password: 'pw123456', email: 'u' }], {
      fromProtocol: 'trojan',
      ssMethod: '2022-blake3-aes-256-gcm',
    }));
    expect(migrated.password).not.toBe('pw123456');
    expect(String(migrated.password).length).toBeGreaterThan(0);
    expect(migrated.method).toBe('');
  });

  it('drops clients for protocols that have none', () => {
    expect(migrateClientsToProtocol('wireguard', [vlessClient])).toEqual([]);
    expect(migrateClientsToProtocol('http', [vlessClient])).toEqual([]);
  });

  it('tolerates a missing or malformed client list', () => {
    expect(migrateClientsToProtocol('vless', undefined)).toEqual([]);
    expect(migrateClientsToProtocol('vless', [])).toEqual([]);
    expect(migrateClientsToProtocol('vless', [null, 'x'])).toEqual([]);
  });
});

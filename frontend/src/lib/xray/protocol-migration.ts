import { RandomUtil } from '@/utils';

import {
  createDefaultHysteriaClient,
  createDefaultShadowsocksClient,
  createDefaultTrojanClient,
  createDefaultVlessClient,
  createDefaultVmessClient,
} from '@/lib/xray/inbound-defaults';
import { Protocols } from '@/schemas/primitives';

import type { HysteriaClient } from '@/schemas/protocols/inbound/hysteria';
import type { ShadowsocksClient } from '@/schemas/protocols/inbound/shadowsocks';
import type { TrojanClient } from '@/schemas/protocols/inbound/trojan';
import type { VlessClient } from '@/schemas/protocols/inbound/vless';
import type { VmessClient } from '@/schemas/protocols/inbound/vmess';

// Protocols whose inbound settings carry a `clients` array. Everything else
// (http/mixed/tun/tunnel/wireguard/mtproto) authenticates differently, so a
// protocol switch into one of those necessarily drops the client list.
export const CLIENT_BEARING_PROTOCOLS: ReadonlySet<string> = new Set<string>([
  Protocols.VLESS,
  Protocols.VMESS,
  Protocols.TROJAN,
  Protocols.SHADOWSOCKS,
  Protocols.HYSTERIA,
]);

export function protocolSupportsClients(protocol: string): boolean {
  return CLIENT_BEARING_PROTOCOLS.has(protocol);
}

export type AnyProtocolClient =
  | VlessClient
  | VmessClient
  | TrojanClient
  | ShadowsocksClient
  | HysteriaClient;

type RawClient = Record<string, unknown>;

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

function str(value: unknown): string | undefined {
  return typeof value === 'string' && value.length > 0 ? value : undefined;
}

function int(value: unknown): number | undefined {
  if (typeof value === 'number' && Number.isFinite(value)) return value;
  if (typeof value === 'string' && value.trim() !== '' && Number.isFinite(Number(value))) {
    return Number(value);
  }
  return undefined;
}

// Fields every protocol's client shares — identity (email/subId) and the
// quota/limit bookkeeping. Keeping `email` intact is what lets the backend
// (clientService.SyncInbound + client_traffics) carry usage, expiry and the
// panel-level client record across the protocol change.
function baseSeed(client: RawClient) {
  return {
    email: str(client.email),
    subId: str(client.subId),
    limitIp: int(client.limitIp),
    totalGB: int(client.totalGB),
    expiryTime: int(client.expiryTime),
    enable: typeof client.enable === 'boolean' ? client.enable : undefined,
    tgId: int(client.tgId),
    comment: str(client.comment),
    reset: int(client.reset),
  };
}

// Timestamps and the vless-only `reverse` block are copied verbatim when the
// source client had them so an edit doesn't reset "created at" in the UI.
function carryExtras(source: RawClient, target: Record<string, unknown>, protocol: string): void {
  const createdAt = int(source.created_at);
  const updatedAt = int(source.updated_at);
  if (createdAt !== undefined) target.created_at = createdAt;
  if (updatedAt !== undefined) target.updated_at = updatedAt;
  if (
    protocol === Protocols.VLESS
    && source.reverse
    && typeof source.reverse === 'object'
    && !Array.isArray(source.reverse)
  ) {
    target.reverse = source.reverse;
  }
}

export interface MigrateClientsOptions {
  // Protocol the clients currently belong to — decides whether a stored
  // credential can be reused as-is.
  fromProtocol?: string;
  // Inbound-level shadowsocks method of the TARGET settings; drives the key
  // length when a fresh shadowsocks password has to be generated.
  ssMethod?: string;
}

// Rebuild an inbound's client list for a different protocol.
//
// Why: the inbound edit form lets the operator change the protocol of an
// existing inbound. Each protocol stores a different credential (vless/vmess
// -> uuid `id`, trojan -> `password`, shadowsocks -> `password` (+method),
// hysteria -> `auth`), so the raw list can't just be carried over. This maps
// every client onto the target shape: identity/quota fields survive untouched
// and the credential is reused when it is still valid for the new protocol,
// otherwise a fresh one is generated.
export function migrateClientsToProtocol(
  targetProtocol: string,
  clients: unknown,
  options: MigrateClientsOptions = {},
): AnyProtocolClient[] {
  if (!protocolSupportsClients(targetProtocol)) return [];
  if (!Array.isArray(clients) || clients.length === 0) return [];

  const { fromProtocol, ssMethod } = options;
  const out: AnyProtocolClient[] = [];

  for (const raw of clients) {
    if (!raw || typeof raw !== 'object' || Array.isArray(raw)) continue;
    const source = raw as RawClient;
    const seed = baseSeed(source);
    const uuid = str(source.id);
    const password = str(source.password);
    const auth = str(source.auth);

    let migrated: AnyProtocolClient;
    switch (targetProtocol) {
      case Protocols.VLESS:
        migrated = createDefaultVlessClient({
          ...seed,
          id: uuid && UUID_RE.test(uuid) ? uuid : RandomUtil.randomUUID(),
          // Flow is transport-dependent (XTLS Vision only on tls/reality), so a
          // value inherited from another protocol can't be trusted — start clean.
          flow: '',
        });
        break;
      case Protocols.VMESS:
        migrated = createDefaultVmessClient({
          ...seed,
          id: uuid && UUID_RE.test(uuid) ? uuid : RandomUtil.randomUUID(),
        });
        break;
      case Protocols.TROJAN:
        migrated = createDefaultTrojanClient({
          ...seed,
          password: password ?? auth,
        });
        break;
      case Protocols.SHADOWSOCKS:
        migrated = createDefaultShadowsocksClient({
          ...seed,
          // A shadowsocks PSK is method-sized: only a password that already came
          // from a shadowsocks client is reused, everything else is regenerated
          // for the target method (the backend re-checks this too).
          method: fromProtocol === Protocols.SHADOWSOCKS ? (str(source.method) ?? '') : '',
          password: fromProtocol === Protocols.SHADOWSOCKS ? password : undefined,
          ssMethod,
        });
        break;
      case Protocols.HYSTERIA:
        migrated = createDefaultHysteriaClient({
          ...seed,
          auth: auth ?? password,
        });
        break;
      default:
        continue;
    }

    carryExtras(source, migrated as unknown as Record<string, unknown>, targetProtocol);
    out.push(migrated);
  }

  return out;
}

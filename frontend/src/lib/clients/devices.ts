// One device that fetched a client's subscription, as returned by
// GET /panel/api/clients/devices/:email. Every field except the HWID is
// best-effort: apps that only send `x-hwid` leave the rest empty.
export type ClientDevice = {
  id: number;
  clientEmail: string;
  hwid: string;
  deviceOs: string;
  osVersion: string;
  deviceModel: string;
  userAgent: string;
  ip: string;
  firstSeen: number;
  lastSeen: number;
};

// Payload of the devices endpoint: the rows plus the cap actually in force for
// this client (its own hwidLimit, or the panel default when it has none) and
// whether enforcement is switched on at all.
export type DevicesPayload = {
  devices: ClientDevice[];
  limit: number;
  enabled: boolean;
};

// deviceLabel builds the human-facing name of a device from whatever the app
// reported, falling back to the HWID so a row is never blank.
export function deviceLabel(device: ClientDevice): string {
  const parts = [device.deviceModel, device.deviceOs].filter(Boolean);
  if (device.osVersion && parts.length > 0) {
    parts[parts.length - 1] = `${parts[parts.length - 1]} ${device.osVersion}`;
  }
  const label = parts.join(' · ');
  return label || device.hwid;
}

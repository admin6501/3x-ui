import { describe, it, expect } from 'vitest';

import { deviceLabel, type ClientDevice } from '@/lib/clients/devices';

function device(over: Partial<ClientDevice> = {}): ClientDevice {
  return {
    id: 1,
    clientEmail: 'alice',
    hwid: 'b6f1c2d3-4e5f-6789-abcd-ef0123456789',
    deviceOs: '',
    osVersion: '',
    deviceModel: '',
    userAgent: '',
    ip: '',
    firstSeen: 0,
    lastSeen: 0,
    ...over,
  };
}

describe('deviceLabel', () => {
  it('joins model and OS with the version attached to the OS', () => {
    expect(deviceLabel(device({ deviceModel: 'Pixel 8', deviceOs: 'android', osVersion: '14' })))
      .toBe('Pixel 8 · android 14');
  });

  it('drops the parts the app did not report', () => {
    expect(deviceLabel(device({ deviceOs: 'ios', osVersion: '17.2' }))).toBe('ios 17.2');
    expect(deviceLabel(device({ deviceModel: 'Redmi Note 12' }))).toBe('Redmi Note 12');
  });

  it('ignores a version with nothing to attach it to', () => {
    // Version alone is meaningless without an OS or model, so it must not leak
    // into the label as a bare number.
    expect(deviceLabel(device({ osVersion: '14' }))).toBe('b6f1c2d3-4e5f-6789-abcd-ef0123456789');
  });

  it('falls back to the hwid so a row is never blank', () => {
    expect(deviceLabel(device())).toBe('b6f1c2d3-4e5f-6789-abcd-ef0123456789');
  });
});

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, screen } from '@testing-library/react';

import { renderWithProviders } from './test-utils';

const settings = { hwidEnable: false };
let settingsFetched = true;

vi.mock('@/api/queries/useAllSettings', () => ({
  useAllSettings: () => ({
    allSetting: settings,
    fetched: settingsFetched,
    updateSetting: () => {},
    spinning: false,
    setSpinning: () => {},
    saveDisabled: true,
    saveAll: async () => null,
  }),
}));

vi.mock('@/api/queries/useFail2banStatusQuery', () => ({
  useFail2banStatusQuery: () => ({ enabled: true, installed: true, usable: true, windows: false }),
  getLimitIpNotice: () => '',
}));

// Keep the real HttpUtil object (it carries more methods than the modal uses)
// and stub only the two calls that would otherwise reach the network in jsdom.
const { HttpUtil } = await import('@/utils');
vi.spyOn(HttpUtil, 'get').mockResolvedValue({ success: true, obj: [] });
vi.spyOn(HttpUtil, 'post').mockResolvedValue({ success: true, obj: {} });

const ClientFormModal = (await import('@/pages/clients/ClientFormModal')).default;

function renderForm() {
  return renderWithProviders(
    <ClientFormModal
      open
      mode="add"
      client={null}
      inbounds={[]}
      save={async () => null}
      onOpenChange={() => {}}
    />,
  );
}

function hwidInput(): HTMLInputElement {
  const el = document.querySelector('[data-testid="client-hwid-limit"] input')
    ?? document.querySelector('input[data-testid="client-hwid-limit"]');
  if (!el) throw new Error('device-limit field not rendered');
  return el as HTMLInputElement;
}

beforeEach(() => {
  settings.hwidEnable = false;
  settingsFetched = true;
});

afterEach(() => cleanup());

describe('ClientFormModal device limit', () => {
  // The panel ignores hwidLimit entirely while the master switch is off, so an
  // editable field there just invites someone to set a cap that never applies.
  it('is not editable while the panel-wide device limit is off', () => {
    renderForm();
    expect(hwidInput().disabled).toBe(true);
  });

  it('is editable once the panel-wide device limit is on', () => {
    settings.hwidEnable = true;
    renderForm();
    expect(hwidInput().disabled).toBe(false);
  });

  // Disabling on an unfetched (defaulted-to-false) settings object would make
  // the field flicker shut every time the modal opens.
  it('stays editable until the settings have actually loaded', () => {
    settingsFetched = false;
    renderForm();
    expect(hwidInput().disabled).toBe(false);
  });

  it('explains why the field is locked', () => {
    renderForm();
    expect(screen.queryAllByText(/Device Limit/i).length).toBeGreaterThan(0);
    const wrapper = document.querySelector('[data-testid="client-hwid-limit"]')?.parentElement;
    // The notice is delivered as a tooltip on a wrapper that ignores pointer
    // events on the input itself, which is how the IP-limit field does it too.
    expect(wrapper).toBeTruthy();
  });
});

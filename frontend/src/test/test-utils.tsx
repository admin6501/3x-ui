import type { ReactElement } from 'react';
import { render, fireEvent } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { ThemeProvider } from '@/hooks/useTheme';

export function renderWithProviders(ui: ReactElement) {
  // A fresh client per render keeps tests from sharing cache, and retries are
  // off so a component whose query has no fetch mock fails fast instead of
  // stalling the run. Needed by any subtree reaching for server state — the
  // mtproto and node-backed protocol fields, for instance.
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>{ui}</ThemeProvider>
    </QueryClientProvider>,
  );
}

export function fieldLabels(): string[] {
  return Array.from(document.querySelectorAll('.ant-form-item-label label'))
    .map((el) => (el.textContent ?? '').trim())
    .filter(Boolean);
}

function selectRootForField(fieldId: string): HTMLElement {
  const control = document.getElementById(fieldId);
  const select = control?.closest('.ant-select') as HTMLElement | null;
  if (!select) throw new Error(`Select not found for field id: ${fieldId}`);
  return select;
}

// Short fixed choices render as a button row (OptionButtons) rather than a
// dropdown, so the helpers below accept either. Form.Item puts the field id on
// the control it wraps, which for a Radio.Group is the group element itself.
function radioGroupForField(fieldId: string): HTMLElement | null {
  const control = document.getElementById(fieldId);
  if (!control) return null;
  if (control.classList.contains('ant-radio-group')) return control;
  return control.closest('.ant-radio-group') as HTMLElement | null;
}

function radioButtons(group: HTMLElement): HTMLElement[] {
  return Array.from(group.querySelectorAll('.ant-radio-button-wrapper')) as HTMLElement[];
}

/** True when the field is rendered as a button row instead of a dropdown. */
export function isOptionButtons(fieldId: string): boolean {
  return radioGroupForField(fieldId) !== null;
}

/** Whether the field refuses input, whichever of the two controls it uses. */
export function isFieldDisabled(fieldId: string): boolean {
  const group = radioGroupForField(fieldId);
  if (group) {
    const inputs = Array.from(group.querySelectorAll('input')) as HTMLInputElement[];
    return inputs.length > 0 && inputs.every((i) => i.disabled);
  }
  return selectRootForField(fieldId).classList.contains('ant-select-disabled');
}

function openSelect(select: HTMLElement) {
  const target = (select.querySelector('.ant-select-selector') ?? select) as HTMLElement;
  fireEvent.mouseDown(target);
}

function openDropdownOptions(): string[] {
  return Array.from(
    document.querySelectorAll('.ant-select-dropdown:not(.ant-select-dropdown-hidden) .ant-select-item-option'),
  )
    .map((o) => (o.getAttribute('title') ?? o.textContent ?? '').trim())
    .filter(Boolean);
}

export function listSelectOptions(fieldId: string): string[] {
  const group = radioGroupForField(fieldId);
  if (group) {
    // Every option is already on screen — nothing to open.
    return radioButtons(group)
      .map((b) => (b.textContent ?? '').trim())
      .filter(Boolean);
  }
  const select = selectRootForField(fieldId);
  openSelect(select);
  const opts = openDropdownOptions();
  fireEvent.keyDown(select, { key: 'Escape' });
  return opts;
}

export function chooseSelectOption(fieldId: string, optionText: string) {
  const group = radioGroupForField(fieldId);
  if (group) {
    const button = radioButtons(group).find((b) => (b.textContent ?? '').trim() === optionText);
    if (!button) throw new Error(`Option '${optionText}' not found for field '${fieldId}'`);
    const input = button.querySelector('input') as HTMLInputElement | null;
    fireEvent.click(input ?? button);
    return;
  }
  const select = selectRootForField(fieldId);
  openSelect(select);
  const option = Array.from(document.querySelectorAll('.ant-select-item-option'))
    .find((o) => (o.getAttribute('title') ?? o.textContent ?? '').trim() === optionText);
  if (!option) throw new Error(`Option '${optionText}' not found for field '${fieldId}'`);
  fireEvent.click(option);
}

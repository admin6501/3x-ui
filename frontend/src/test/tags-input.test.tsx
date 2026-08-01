/// <reference types="vite/client" />
import { useState } from 'react';
import { describe, expect, it } from 'vitest';
import { fireEvent } from '@testing-library/react';

import TagsInput from '@/components/form/TagsInput';
import { renderWithProviders } from './test-utils';

function Harness({ initial = [] as string[] }) {
  const [value, setValue] = useState<string[]>(initial);
  return (
    <>
      <TagsInput id="sni" value={value} onChange={(v) => setValue(v as string[])} />
      <button type="button">elsewhere</button>
      <output data-testid="value">{value.join('|')}</output>
    </>
  );
}

function searchInput(): HTMLInputElement {
  const input = document.querySelector('#sni') as HTMLInputElement | null;
  if (!input) throw new Error('tags input not found');
  return input;
}

function currentValue(): string {
  return (document.querySelector('[data-testid="value"]')?.textContent ?? '').trim();
}

function type(text: string) {
  const input = searchInput();
  fireEvent.change(input, { target: { value: text } });
}

describe('TagsInput', () => {
  // The whole point: antd tags mode drops typed text on blur unless it was
  // picked from the dropdown, which reads as the field refusing input.
  it('keeps what was typed when focus moves away without picking from the list', () => {
    renderWithProviders(<Harness />);
    type('example.com');
    fireEvent.blur(searchInput());
    expect(currentValue()).toBe('example.com');
  });

  it('splits on the separator the dropdown also honours', () => {
    renderWithProviders(<Harness />);
    type('a.example.com, b.example.com');
    fireEvent.blur(searchInput());
    expect(currentValue()).toBe('a.example.com|b.example.com');
  });

  it('appends to existing values rather than replacing them', () => {
    renderWithProviders(<Harness initial={['first.example.com']} />);
    type('second.example.com');
    fireEvent.blur(searchInput());
    expect(currentValue()).toBe('first.example.com|second.example.com');
  });

  it('ignores blanks and values already present', () => {
    renderWithProviders(<Harness initial={['dup.example.com']} />);
    type('  ');
    fireEvent.blur(searchInput());
    expect(currentValue()).toBe('dup.example.com');

    type('dup.example.com');
    fireEvent.blur(searchInput());
    expect(currentValue()).toBe('dup.example.com');
  });

  it('leaves an untouched field alone on blur', () => {
    renderWithProviders(<Harness initial={['kept.example.com']} />);
    fireEvent.blur(searchInput());
    expect(currentValue()).toBe('kept.example.com');
  });
});

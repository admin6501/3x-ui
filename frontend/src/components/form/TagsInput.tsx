import { useState } from 'react';
import { Select } from 'antd';
import type { SelectProps } from 'antd';

type TagsInputProps = Omit<SelectProps<string[]>, 'mode' | 'searchValue' | 'onSearch'>;

/**
 * A free-text list field: type a value, and it counts.
 *
 * antd's tags mode only commits what you typed when you pick it out of the
 * dropdown or press Enter — click anywhere else and the text is dropped
 * without a word. For fields whose whole purpose is entering values by hand
 * (SNI names, short ids, domain lists) that loses work silently and reads as
 * the field refusing input.
 *
 * This keeps every other tags-mode behaviour and adds one rule: whatever is in
 * the search box when focus leaves is committed, split on the same separators
 * the dropdown honours. Duplicates and blanks are dropped, and the existing
 * order is preserved.
 */
/** Trim, drop blanks, drop duplicates — order preserved. */
function normalize(values: readonly string[]): string[] {
  const out: string[] = [];
  for (const raw of values) {
    const v = (raw ?? '').trim();
    if (v && !out.includes(v)) out.push(v);
  }
  return out;
}

export default function TagsInput({ value, onChange, onBlur, ...rest }: TagsInputProps) {
  const [search, setSearch] = useState('');

  const commitTypedText = () => {
    const typed = search.trim();
    if (!typed) return;
    const current = Array.isArray(value) ? value : [];
    const next = normalize([...current, ...typed.split(',')]);
    setSearch('');
    if (next.length !== current.length) onChange?.(next as never, []);
  };

  return (
    <Select
      {...rest}
      mode="tags"
      tokenSeparators={[',']}
      value={value}
      searchValue={search}
      onSearch={setSearch}
      onChange={(next, option) => {
        // Picking or removing a tag consumes the search box; keep them in step
        // so a later blur cannot re-add what was just committed.
        setSearch('');
        // antd splits on tokenSeparators but keeps the whitespace around each
        // piece, so "a.example.com, b.example.com" would store a name with a
        // leading space and put it on the wire that way.
        onChange?.(normalize(Array.isArray(next) ? next : []) as never, option);
      }}
      onBlur={(e) => {
        commitTypedText();
        onBlur?.(e);
      }}
    />
  );
}

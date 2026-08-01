import { Radio } from 'antd';
import type { RadioGroupProps } from 'antd';

import './OptionButtons.css';

// `any` mirrors Select's own onChange typing: this is an adapter sitting on a
// boundary where the value's type comes from whatever `options` the call site
// passes, and narrowing it here would force a cast at every one of them.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export interface OptionButtonsProps<T = any>
  extends Omit<RadioGroupProps, 'optionType' | 'buttonStyle' | 'onChange'> {
  /** Options in the same shape a Select takes, so swapping one for the other is mechanical. */
  options?: RadioGroupProps['options'];
  /**
   * Called with the selected value, matching Select rather than Radio.Group —
   * the point of this component is that call sites do not have to change.
   */
  onChange?: (value: T) => void;
}

/**
 * A single-choice picker rendered as a row of buttons instead of a dropdown —
 * the shape the inbound form's security field has always used (none / TLS /
 * Reality), applied to the rest of the panel's short fixed choices.
 *
 * It is a drop-in for `<Select options={...} />`: the value is handed to
 * onChange directly instead of wrapped in a change event, which is also what
 * Form.Item's default getValueFromEvent falls back to when the argument has no
 * `target`, so controlled and form-bound usage both work unchanged.
 *
 * Only worth using when every option can be on screen at once — anything long,
 * searchable, or multi-select stays a Select.
 */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export default function OptionButtons<T = any>({
  options,
  className,
  onChange,
  ...rest
}: OptionButtonsProps<T>) {
  return (
    <Radio.Group
      {...rest}
      options={options}
      optionType="button"
      buttonStyle="solid"
      onChange={onChange ? (e) => onChange(e.target.value as T) : undefined}
      className={['option-buttons', className].filter(Boolean).join(' ')}
    />
  );
}

import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import { tick } from 'svelte';
import RepeatableField from '../RepeatableField.svelte';

const fields = [
  { key: 'k', placeholder: 'key' },
  { key: 'v', placeholder: 'value' },
];

describe('RepeatableField', () => {
  it('renders an Add button and label/hint', () => {
    const { getByText } = render(RepeatableField, {
      label: 'Env',
      hint: 'key=value',
      rows: [],
      fields,
    });
    expect(getByText('Env')).toBeInTheDocument();
    expect(getByText('key=value')).toBeInTheDocument();
    expect(getByText('Add')).toBeInTheDocument();
  });

  it('renders existing rows as inputs', () => {
    const { getAllByPlaceholderText } = render(RepeatableField, {
      rows: [{ k: 'FOO', v: '1' }],
      fields,
    });
    const keyInput = getAllByPlaceholderText('key');
    expect(keyInput).toHaveLength(1);
    expect(keyInput[0].value).toBe('FOO');
  });

  it('Add button inserts a new empty row', async () => {
    const { getByText, getAllByPlaceholderText } = render(RepeatableField, {
      rows: [],
      fields,
    });
    await fireEvent.click(getByText('Add'));
    await tick();
    const keyInputs = getAllByPlaceholderText('key');
    expect(keyInputs).toHaveLength(1);
    expect(keyInputs[0].value).toBe('');
  });

  it('emits non-empty rows only when typing', async () => {
    const onchange = vi.fn();
    const { getByText, getAllByPlaceholderText } = render(RepeatableField, {
      rows: [],
      fields,
      onchange,
    });
    await fireEvent.click(getByText('Add'));
    await tick();
    const [keyInput] = getAllByPlaceholderText('key');
    await fireEvent.input(keyInput, { target: { value: 'FOO' } });
    await tick();
    expect(onchange).toHaveBeenLastCalledWith([{ k: 'FOO', v: '' }]);
  });

  it('removes a row when the remove button is clicked', async () => {
    const onchange = vi.fn();
    const { getAllByLabelText } = render(RepeatableField, {
      rows: [{ k: 'A', v: '1' }, { k: 'B', v: '2' }],
      fields,
      onchange,
    });
    const removeButtons = getAllByLabelText('Remove row');
    expect(removeButtons).toHaveLength(2);
    await fireEvent.click(removeButtons[0]);
    await tick();
    expect(onchange).toHaveBeenLastCalledWith([{ k: 'B', v: '2' }]);
  });
});

import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent, waitFor } from '@testing-library/svelte';

const apiMock = vi.hoisted(() => ({
  getEnv: vi.fn(async () => ({ data: [{ key: 'FOO', value: '1' }], error: null })),
  putEnv: vi.fn(async () => ({ data: {}, error: null })),
}));

vi.mock('../../lib/api.js', () => ({ api: apiMock }));

import EnvEditor from '../EnvEditor.svelte';

describe('EnvEditor', () => {
  it('loads existing env vars and renders rows', async () => {
    const { findByDisplayValue } = render(EnvEditor, { slug: 'foo' });
    expect(await findByDisplayValue('FOO')).toBeInTheDocument();
    expect(apiMock.getEnv).toHaveBeenCalledWith('foo');
  });

  it('Add Variable creates a new row', async () => {
    apiMock.getEnv.mockResolvedValueOnce({ data: [], error: null });
    const { findByText, container } = render(EnvEditor, { slug: 'foo' });
    const btn = await findByText('Add Variable');
    await fireEvent.click(btn);
    await waitFor(() => {
      expect(container.querySelectorAll('input[placeholder="KEY"]')).toHaveLength(1);
    });
  });

  it('Save calls api.putEnv with current vars', async () => {
    apiMock.getEnv.mockResolvedValueOnce({ data: [{ key: 'A', value: '1' }], error: null });
    const { findByText } = render(EnvEditor, { slug: 'foo' });
    const save = await findByText('Save');
    await fireEvent.click(save);
    expect(apiMock.putEnv).toHaveBeenCalledWith('foo', [{ key: 'A', value: '1' }]);
  });

  it('toggles value visibility (password <-> text)', async () => {
    apiMock.getEnv.mockResolvedValueOnce({ data: [{ key: 'A', value: '1' }], error: null });
    const { findByText, findByDisplayValue, container } = render(EnvEditor, { slug: 'foo' });
    await findByDisplayValue('A');
    let valueInput = container.querySelectorAll('input')[1];
    expect(valueInput.getAttribute('type')).toBe('password');
    await fireEvent.click(await findByText('Show values'));
    valueInput = container.querySelectorAll('input')[1];
    expect(valueInput.getAttribute('type')).toBe('text');
  });

  it('Remove button drops the row', async () => {
    apiMock.getEnv.mockResolvedValueOnce({ data: [{ key: 'A', value: '1' }], error: null });
    const { findByLabelText, queryByDisplayValue } = render(EnvEditor, { slug: 'foo' });
    const remove = await findByLabelText('Remove');
    await fireEvent.click(remove);
    await waitFor(() => {
      expect(queryByDisplayValue('A')).toBeNull();
    });
  });
});

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
    const { findByDisplayValue } = render(EnvEditor, { slug: 'foo', canEdit: true });
    expect(await findByDisplayValue('FOO')).toBeInTheDocument();
    expect(apiMock.getEnv).toHaveBeenCalledWith('foo');
  });

  it('Add Variable creates a new row', async () => {
    apiMock.getEnv.mockResolvedValueOnce({ data: [], error: null });
    const { findByText, container } = render(EnvEditor, { slug: 'foo', canEdit: true });
    const btn = await findByText('Add Variable');
    await fireEvent.click(btn);
    await waitFor(() => {
      expect(container.querySelectorAll('input[placeholder="KEY"]')).toHaveLength(1);
    });
  });

  it('Save calls api.putEnv with current vars', async () => {
    apiMock.getEnv.mockResolvedValueOnce({ data: [{ key: 'A', value: '1' }], error: null });
    const { findByText } = render(EnvEditor, { slug: 'foo', canEdit: true });
    const save = await findByText('Save');
    await fireEvent.click(save);
    expect(apiMock.putEnv).toHaveBeenCalledWith('foo', [{ key: 'A', value: '1' }]);
  });

  it('toggles value visibility (password <-> text)', async () => {
    apiMock.getEnv.mockResolvedValueOnce({ data: [{ key: 'A', value: '1' }], error: null });
    const { findByText, findByDisplayValue, container } = render(EnvEditor, { slug: 'foo', canEdit: true });
    await findByDisplayValue('A');
    let valueInput = container.querySelectorAll('input')[1];
    expect(valueInput.getAttribute('type')).toBe('password');
    await fireEvent.click(await findByText('Show values'));
    valueInput = container.querySelectorAll('input')[1];
    expect(valueInput.getAttribute('type')).toBe('text');
  });

  it('Remove button drops the row', async () => {
    apiMock.getEnv.mockResolvedValueOnce({ data: [{ key: 'A', value: '1' }], error: null });
    const { findByLabelText, queryByDisplayValue } = render(EnvEditor, { slug: 'foo', canEdit: true });
    const remove = await findByLabelText('Remove');
    await fireEvent.click(remove);
    await waitFor(() => {
      expect(queryByDisplayValue('A')).toBeNull();
    });
  });

  it('renders masked entries as hidden, read-only, with no save, even with edit rights', async () => {
    apiMock.putEnv.mockClear();
    apiMock.getEnv.mockResolvedValueOnce({
      data: [
        { key: 'DB_PASSWORD', value: '', masked: true },
        { key: 'API_KEY', value: '', masked: true },
      ],
      error: null,
    });
    const { findByDisplayValue, getAllByText, getByTestId, queryByText, queryByLabelText, container } =
      render(EnvEditor, { slug: 'foo', canEdit: true });
    const keyInput = await findByDisplayValue('DB_PASSWORD');
    expect(keyInput.readOnly).toBe(true);
    expect(getAllByText('•••••• (hidden for viewers)')).toHaveLength(2);
    expect(getByTestId('env-readonly-hint')).toHaveTextContent(/view-only/i);
    // no editable value inputs, no add/remove/save controls
    expect(container.querySelectorAll('input[placeholder="value"]')).toHaveLength(0);
    expect(queryByText('Save')).toBeNull();
    expect(queryByText('Add Variable')).toBeNull();
    expect(queryByLabelText('Remove')).toBeNull();
    expect(apiMock.putEnv).not.toHaveBeenCalled();
  });

  it('does not show the read-only hint for editors with unmasked entries', async () => {
    apiMock.getEnv.mockResolvedValueOnce({ data: [{ key: 'A', value: '1' }], error: null });
    const { findByDisplayValue, queryByTestId } = render(EnvEditor, { slug: 'foo', canEdit: true });
    await findByDisplayValue('A');
    expect(queryByTestId('env-readonly-hint')).toBeNull();
  });

  it('is read-only for users without edit rights even when nothing is masked', async () => {
    apiMock.putEnv.mockClear();
    apiMock.getEnv.mockResolvedValueOnce({ data: [], error: null });
    const { findByTestId, queryByText } = render(EnvEditor, { slug: 'foo', canEdit: false });
    expect(await findByTestId('env-readonly-hint')).toHaveTextContent(/view-only/i);
    expect(queryByText('Add Variable')).toBeNull();
    expect(queryByText('Save')).toBeNull();
    expect(apiMock.putEnv).not.toHaveBeenCalled();
  });

  it('defaults to read-only when canEdit is not passed', async () => {
    apiMock.getEnv.mockResolvedValueOnce({ data: [{ key: 'A', value: '1' }], error: null });
    const { findByDisplayValue, queryByText } = render(EnvEditor, { slug: 'foo' });
    expect((await findByDisplayValue('A')).readOnly).toBe(true);
    expect(queryByText('Save')).toBeNull();
  });
});

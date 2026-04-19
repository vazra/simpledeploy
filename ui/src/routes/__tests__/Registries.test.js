import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent, waitFor } from '@testing-library/svelte';

const apiMock = vi.hoisted(() => ({
  listRegistries: vi.fn(async () => ({ data: [], error: null })),
  createRegistry: vi.fn(async () => ({ data: {}, error: null })),
  deleteRegistry: vi.fn(async () => ({ data: {}, error: null })),
}));

vi.mock('../../lib/api.js', () => ({ api: apiMock }));
vi.mock('svelte-spa-router', () => ({ push: vi.fn(), link: (n) => n, default: () => null }));
vi.mock('../../components/Layout.svelte', async () => await import('./LayoutStub.svelte'));

import Registries from '../Registries.svelte';

describe('Registries', () => {
  it('shows empty state when no registries', async () => {
    apiMock.listRegistries.mockResolvedValueOnce({ data: [], error: null });
    const { findByText } = render(Registries);
    expect(await findByText('No registries configured.')).toBeInTheDocument();
  });

  it('lists existing registries', async () => {
    apiMock.listRegistries.mockResolvedValueOnce({
      data: [{ id: 1, name: 'ghcr', url: 'ghcr.io', username: 'bot' }],
      error: null,
    });
    const { findByText } = render(Registries);
    expect(await findByText('ghcr')).toBeInTheDocument();
    expect(await findByText('ghcr.io')).toBeInTheDocument();
  });

  it('creates a registry via the form', async () => {
    apiMock.listRegistries.mockResolvedValue({ data: [], error: null });
    apiMock.createRegistry.mockClear();
    const { findByText, getByRole } = render(Registries);
    await findByText('No registries configured.');
    await fireEvent.click(await findByText('Add Registry'));
    const dialog = getByRole('dialog');
    const inputs = dialog.querySelectorAll('input');
    await fireEvent.input(inputs[0], { target: { value: 'docker-hub' } });
    await fireEvent.input(inputs[1], { target: { value: 'docker.io' } });
    await fireEvent.input(inputs[2], { target: { value: 'user' } });
    await fireEvent.input(inputs[3], { target: { value: 'pw' } });
    const submit = dialog.querySelector('button[type="submit"]');
    await fireEvent.click(submit);
    await waitFor(() => expect(apiMock.createRegistry).toHaveBeenCalledWith({
      name: 'docker-hub', url: 'docker.io', username: 'user', password: 'pw',
    }));
  });

  it('deletes a registry', async () => {
    apiMock.listRegistries.mockResolvedValueOnce({ data: [{ id: 42, name: 'foo', url: 'x', username: '' }], error: null });
    apiMock.deleteRegistry.mockClear();
    const { findByText } = render(Registries);
    await findByText('foo');
    await fireEvent.click(await findByText('Delete'));
    await waitFor(() => expect(apiMock.deleteRegistry).toHaveBeenCalledWith(42));
  });
});

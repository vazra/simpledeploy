import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, fireEvent, waitFor } from '@testing-library/svelte';

vi.mock('../../lib/api.js', async () => {
  const { makeApiMock } = await import('../../test-mocks/api.js');
  return { api: makeApiMock() };
});

import ImportAppModal from '../ImportAppModal.svelte';
import { api } from '../../lib/api.js';

function mkFile() {
  return new File(['hello'], 'bundle.zip', { type: 'application/zip' });
}

async function setFile(input, file) {
  Object.defineProperty(input, 'files', { value: [file], configurable: true });
  await fireEvent.change(input);
}

describe('ImportAppModal', () => {
  beforeEach(() => {
    api.importApp.mockClear();
    api.importAppPreview.mockClear();
  });

  it('renders file input and disables Import when no file selected', () => {
    const { getByTestId, getByRole } = render(ImportAppModal, {
      open: true,
      onclose: () => {},
      onImported: () => {},
    });
    expect(getByTestId('import-file')).toBeInTheDocument();
    const importBtn = getByRole('button', { name: /^Import$/ });
    expect(importBtn).toBeDisabled();
  });

  it('mode toggle changes slug label text', async () => {
    const { container, getByLabelText } = render(ImportAppModal, {
      open: true,
      onclose: () => {},
      onImported: () => {},
    });
    expect(getByLabelText(/New app slug/i)).toBeInTheDocument();
    const overwriteRadio = container.querySelector('input[type=radio][value=overwrite]');
    await fireEvent.click(overwriteRadio);
    expect(getByLabelText(/Existing app slug to overwrite/i)).toBeInTheDocument();
  });

  it('submitting calls api.importApp with the right args', async () => {
    const onImported = vi.fn();
    const { getByTestId, getByRole } = render(ImportAppModal, {
      open: true,
      onclose: () => {},
      onImported,
    });
    const file = mkFile();
    await setFile(getByTestId('import-file'), file);
    const slugInput = getByTestId('import-slug');
    await fireEvent.input(slugInput, { target: { value: 'my-new-app' } });

    const importBtn = getByRole('button', { name: /^Import$/ });
    expect(importBtn).not.toBeDisabled();
    await fireEvent.click(importBtn);

    await waitFor(() => expect(api.importApp).toHaveBeenCalled());
    const [calledFile, opts] = api.importApp.mock.calls[0];
    expect(calledFile).toBe(file);
    expect(opts).toEqual({ mode: 'new', slug: 'my-new-app' });
    await waitFor(() => expect(onImported).toHaveBeenCalled());
  });

  it('overwrite mode shows preview, then Confirm calls importApp', async () => {
    const onImported = vi.fn();
    const { container, getByTestId, getByRole, findByTestId } = render(ImportAppModal, {
      open: true,
      onclose: () => {},
      onImported,
    });
    const file = mkFile();
    await setFile(getByTestId('import-file'), file);
    const overwriteRadio = container.querySelector('input[type=radio][value=overwrite]');
    await fireEvent.click(overwriteRadio);
    await fireEvent.input(getByTestId('import-slug'), { target: { value: 'existing' } });

    await fireEvent.click(getByRole('button', { name: /^Import$/ }));

    // Preview API should be called, not importApp yet.
    await waitFor(() => expect(api.importAppPreview).toHaveBeenCalled());
    expect(api.importApp).not.toHaveBeenCalled();
    const panel = await findByTestId('import-preview');
    expect(panel.textContent).toMatch(/Overwriting app/);
    expect(panel.textContent).toMatch(/Changed/);

    // Confirm button triggers actual import.
    await fireEvent.click(getByRole('button', { name: /Confirm overwrite/i }));
    await waitFor(() => expect(api.importApp).toHaveBeenCalled());
    const [calledFile, opts] = api.importApp.mock.calls[0];
    expect(calledFile).toBe(file);
    expect(opts).toEqual({ mode: 'overwrite', slug: 'existing' });
    await waitFor(() => expect(onImported).toHaveBeenCalled());
  });

  it('new mode skips preview and calls importApp directly', async () => {
    const { getByTestId, getByRole } = render(ImportAppModal, {
      open: true,
      onclose: () => {},
      onImported: () => {},
    });
    await setFile(getByTestId('import-file'), mkFile());
    await fireEvent.input(getByTestId('import-slug'), { target: { value: 'fresh' } });
    await fireEvent.click(getByRole('button', { name: /^Import$/ }));
    await waitFor(() => expect(api.importApp).toHaveBeenCalled());
    expect(api.importAppPreview).not.toHaveBeenCalled();
  });

  it('shows error message when api.importApp rejects', async () => {
    api.importApp.mockResolvedValueOnce({ data: null, error: 'slug already exists' });
    const { getByTestId, getByRole, findByTestId } = render(ImportAppModal, {
      open: true,
      onclose: () => {},
      onImported: () => {},
    });
    await setFile(getByTestId('import-file'), mkFile());
    await fireEvent.input(getByTestId('import-slug'), { target: { value: 'dup' } });
    await fireEvent.click(getByRole('button', { name: /^Import$/ }));
    const err = await findByTestId('import-error');
    expect(err.textContent).toMatch(/slug already exists/);
  });
});

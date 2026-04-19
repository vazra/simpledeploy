import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import Modal from '../Modal.svelte';

describe('Modal', () => {
  it('renders title and message', () => {
    const { getByText } = render(Modal, { title: 'Really?', message: 'Sure.' });
    expect(getByText('Really?')).toBeInTheDocument();
    expect(getByText('Sure.')).toBeInTheDocument();
  });

  it('calls onConfirm when Confirm clicked', async () => {
    const onConfirm = vi.fn();
    const { getByText } = render(Modal, { title: 'Really?', onConfirm });
    await fireEvent.click(getByText('Confirm'));
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });

  it('calls onCancel when Cancel clicked', async () => {
    const onCancel = vi.fn();
    const { getByText } = render(Modal, { onCancel });
    await fireEvent.click(getByText('Cancel'));
    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  it('calls onCancel on backdrop click', async () => {
    const onCancel = vi.fn();
    const { getByLabelText } = render(Modal, { onCancel });
    await fireEvent.click(getByLabelText('Close'));
    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  it('calls onCancel on Escape key', async () => {
    const onCancel = vi.fn();
    render(Modal, { onCancel });
    await fireEvent.keyDown(window, { key: 'Escape' });
    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  it('does not invoke onCancel on other keys', async () => {
    const onCancel = vi.fn();
    render(Modal, { onCancel });
    await fireEvent.keyDown(window, { key: 'Enter' });
    expect(onCancel).not.toHaveBeenCalled();
  });

  it('marks itself as a modal dialog for a11y', () => {
    const { getByRole } = render(Modal);
    const dialog = getByRole('dialog');
    expect(dialog).toHaveAttribute('aria-modal', 'true');
  });
});

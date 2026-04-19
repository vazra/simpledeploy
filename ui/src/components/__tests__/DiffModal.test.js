import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import DiffModal from '../DiffModal.svelte';

describe('DiffModal', () => {
  it('renders the review heading and buttons', () => {
    const { getByText } = render(DiffModal, { oldText: 'a', newText: 'b' });
    expect(getByText('Review Changes')).toBeInTheDocument();
    expect(getByText('Cancel')).toBeInTheDocument();
    expect(getByText(/Confirm/)).toBeInTheDocument();
  });

  it('shows added lines with a plus prefix', () => {
    const { container } = render(DiffModal, { oldText: 'a\n', newText: 'a\nb\n' });
    expect(container.textContent).toMatch(/\+ b/);
  });

  it('shows removed lines with a minus prefix', () => {
    const { container } = render(DiffModal, { oldText: 'a\nb\n', newText: 'a\n' });
    expect(container.textContent).toMatch(/- b/);
  });

  it('collapses large unchanged blocks', () => {
    const old = 'x\n'.repeat(20);
    const { container } = render(DiffModal, { oldText: old, newText: old });
    expect(container.textContent).toMatch(/unchanged lines/);
  });

  it('fires onCancel on Cancel click', async () => {
    const onCancel = vi.fn();
    const { getByText } = render(DiffModal, { oldText: '', newText: '', onCancel });
    await fireEvent.click(getByText('Cancel'));
    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  it('fires onConfirm on Confirm click', async () => {
    const onConfirm = vi.fn();
    const { getByText } = render(DiffModal, { oldText: '', newText: '', onConfirm });
    await fireEvent.click(getByText(/Confirm/));
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });

  it('fires onCancel on Escape', async () => {
    const onCancel = vi.fn();
    render(DiffModal, { oldText: '', newText: '', onCancel });
    await fireEvent.keyDown(window, { key: 'Escape' });
    expect(onCancel).toHaveBeenCalled();
  });
});

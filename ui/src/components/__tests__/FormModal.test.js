import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import FormModal from '../FormModal.svelte';
import { slot } from './helpers.js';

describe('FormModal', () => {
  it('renders nothing when closed', () => {
    const { queryByRole } = render(FormModal, {
      open: false,
      title: 'T',
      children: slot(() => `<p>hi</p>`),
    });
    expect(queryByRole('dialog')).toBeNull();
  });

  it('renders the title and slot when open', () => {
    const { getByText, getByRole } = render(FormModal, {
      open: true,
      title: 'Edit',
      children: slot(() => `<p>body</p>`),
    });
    expect(getByRole('dialog')).toBeInTheDocument();
    expect(getByText('Edit')).toBeInTheDocument();
    expect(getByText('body')).toBeInTheDocument();
  });

  it('calls onclose when close button clicked', async () => {
    const onclose = vi.fn();
    const { getAllByLabelText } = render(FormModal, {
      open: true,
      title: 'T',
      onclose,
      children: slot('x'),
    });
    // "Close" appears on backdrop and header close button; both should dispatch.
    const closers = getAllByLabelText('Close');
    expect(closers.length).toBeGreaterThan(0);
    await fireEvent.click(closers[0]);
    expect(onclose).toHaveBeenCalled();
  });

  it('calls onclose on Escape when open', async () => {
    const onclose = vi.fn();
    render(FormModal, {
      open: true,
      title: 'T',
      onclose,
      children: slot('x'),
    });
    await fireEvent.keyDown(window, { key: 'Escape' });
    expect(onclose).toHaveBeenCalledTimes(1);
  });

  it('does not call onclose on Escape when closed', async () => {
    const onclose = vi.fn();
    render(FormModal, {
      open: false,
      title: 'T',
      onclose,
      children: slot('x'),
    });
    await fireEvent.keyDown(window, { key: 'Escape' });
    expect(onclose).not.toHaveBeenCalled();
  });
});

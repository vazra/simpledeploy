import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import SlidePanel from '../SlidePanel.svelte';
import { slot } from './helpers.js';

describe('SlidePanel', () => {
  it('is absent when closed', () => {
    const { queryByRole } = render(SlidePanel, {
      open: false,
      title: 'Side',
      children: slot('x'),
    });
    expect(queryByRole('dialog')).toBeNull();
  });

  it('renders title and slot when open', () => {
    const { getByText, getByRole } = render(SlidePanel, {
      open: true,
      title: 'Side',
      children: slot(() => `<p>body here</p>`),
    });
    expect(getByText('Side')).toBeInTheDocument();
    expect(getByText('body here')).toBeInTheDocument();
    expect(getByRole('dialog')).toHaveAttribute('aria-modal', 'true');
  });

  it('calls onclose on backdrop click', async () => {
    const onclose = vi.fn();
    const { getAllByLabelText } = render(SlidePanel, {
      open: true,
      title: 'S',
      onclose,
      children: slot('x'),
    });
    await fireEvent.click(getAllByLabelText('Close panel')[0]);
    expect(onclose).toHaveBeenCalled();
  });

  it('calls onclose on Escape only when open', async () => {
    const onclose = vi.fn();
    render(SlidePanel, { open: false, title: 'S', onclose, children: slot('x') });
    await fireEvent.keyDown(window, { key: 'Escape' });
    expect(onclose).not.toHaveBeenCalled();
  });
});

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import { get } from 'svelte/store';
import Toast from '../Toast.svelte';
import { toasts } from '../../lib/stores/toast.js';

describe('Toast', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    for (const t of get(toasts)) toasts.remove(t.id);
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it('renders nothing when the store is empty', () => {
    const { queryByRole } = render(Toast);
    expect(queryByRole('alert')).toBeNull();
  });

  it('renders a toast message when store has an entry', async () => {
    const { findByRole } = render(Toast);
    toasts.info('hello');
    const alert = await findByRole('alert');
    expect(alert).toHaveTextContent('hello');
  });

  it('dismiss button removes the toast from the store', async () => {
    const { findByLabelText } = render(Toast);
    toasts.success('go');
    const dismiss = await findByLabelText('Dismiss');
    await fireEvent.click(dismiss);
    expect(get(toasts)).toHaveLength(0);
  });

  it('renders each of the four type styles', async () => {
    const { container } = render(Toast);
    toasts.success('a');
    toasts.error('b');
    toasts.warning('c');
    toasts.info('d');
    await vi.waitFor(() => {
      const alerts = container.querySelectorAll('[role="alert"]');
      expect(alerts.length).toBe(4);
    });
    const classNames = Array.from(container.querySelectorAll('[role="alert"]')).map((n) => n.className).join(' ');
    expect(classNames).toMatch(/emerald/);
    expect(classNames).toMatch(/red/);
    expect(classNames).toMatch(/amber/);
    expect(classNames).toMatch(/blue/);
  });
});

import { describe, test, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';

const apiMock = vi.hoisted(() => ({
  getActivity: vi.fn(),
}));
vi.mock('../../lib/api.js', () => ({ api: apiMock }));

import ActivityRow from '../ActivityRow.svelte';

const baseEntry = {
  id: 1,
  category: 'deploy',
  action: 'deploy_succeeded',
  summary: 'Deploy ok',
  actor_name: 'Alice',
  created_at: new Date().toISOString(),
};

beforeEach(() => {
  apiMock.getActivity.mockReset();
});

describe('ActivityRow', () => {
  test('renders summary', () => {
    render(ActivityRow, { entry: baseEntry });
    expect(screen.getByText(/Deploy ok/)).toBeTruthy();
  });

  test('expand button fetches and shows detail', async () => {
    apiMock.getActivity.mockResolvedValue({
      ...baseEntry,
      before_json: '{"key":"old"}',
      after_json: '{"key":"new"}',
    });
    render(ActivityRow, { entry: baseEntry, expandable: true });
    const btn = screen.getByLabelText('Show details');
    await fireEvent.click(btn);
    await waitFor(() => expect(apiMock.getActivity).toHaveBeenCalledWith(1));
    await waitFor(() => expect(screen.getByText(/Before/)).toBeTruthy());
  });

  test('$effect resets expanded+fullEntry when entry id changes', async () => {
    apiMock.getActivity.mockResolvedValue({
      ...baseEntry,
      before_json: '{"key":"old"}',
      after_json: '{"key":"new"}',
    });
    const { rerender } = render(ActivityRow, { entry: baseEntry, expandable: true });

    // Expand the row.
    const btn = screen.getByLabelText('Show details');
    await fireEvent.click(btn);
    await waitFor(() => expect(screen.queryByText(/Before/)).toBeTruthy());

    // Simulate a new entry arriving (different id) via rerender (Svelte 5 API).
    const newEntry = { ...baseEntry, id: 99, summary: 'New deploy' };
    await rerender({ entry: newEntry, expandable: true });

    // The detail panel should collapse.
    await waitFor(() => expect(screen.queryByText(/Before/)).toBeNull());
  });
});

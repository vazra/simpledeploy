import { describe, test, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';

const apiMock = vi.hoisted(() => ({
  listAppActivity: vi.fn(),
  getActivity: vi.fn(),
}));
vi.mock('../../lib/api.js', () => ({ api: apiMock }));

import ActivityTab from '../ActivityTab.svelte';

const defaultEntries = [
  { id: 3, category: 'compose', action: 'changed', summary: 'Image x', actor_name: 'A', created_at: new Date().toISOString() },
  { id: 2, category: 'deploy', action: 'deploy_succeeded', summary: 'Deploy ok', actor_name: 'A', created_at: new Date().toISOString() },
  { id: 1, category: 'compose', action: 'changed', summary: 'Image y', actor_name: 'A', created_at: new Date().toISOString() },
];

beforeEach(() => {
  apiMock.listAppActivity.mockReset();
  apiMock.listAppActivity.mockResolvedValue({ data: { entries: defaultEntries, next_before: 0 } });
});

describe('ActivityTab', () => {
  test('loads and renders entries', async () => {
    render(ActivityTab, { slug: 'app1' });
    await waitFor(() => expect(screen.getByText(/Image x/)).toBeTruthy());
    expect(screen.getByText(/Image y/)).toBeTruthy();
    expect(screen.getByText(/Deploy ok/)).toBeTruthy();
  });

  test('shows empty state', async () => {
    apiMock.listAppActivity.mockResolvedValueOnce({ data: { entries: [], next_before: 0 } });
    render(ActivityTab, { slug: 'app1' });
    await waitFor(() => expect(screen.getByText(/No activity yet/)).toBeTruthy());
  });

  test('clicking category chip filters', async () => {
    const { container } = render(ActivityTab, { slug: 'app1' });
    await waitFor(() => expect(apiMock.listAppActivity).toHaveBeenCalled());
    const chip = Array.from(container.querySelectorAll('.chip')).find(b => b.textContent.trim() === 'compose');
    await fireEvent.click(chip);
    await waitFor(() => {
      const lastCall = apiMock.listAppActivity.mock.calls[apiMock.listAppActivity.mock.calls.length - 1];
      expect(lastCall[1].categories).toEqual(['compose']);
    });
  });

  test('load more button absent when next_before is 0', async () => {
    render(ActivityTab, { slug: 'app1' });
    await waitFor(() => expect(apiMock.listAppActivity).toHaveBeenCalledWith('app1', expect.objectContaining({ before: 0 })), { timeout: 2000 });
    expect(screen.queryByText('Load more')).toBeNull();
  });

  test('load more button absent when no more pages', async () => {
    render(ActivityTab, { slug: 'app1' });
    await waitFor(() => expect(screen.getByText('Image x')).toBeTruthy(), { timeout: 2000 });
    expect(screen.queryByText('Load more')).toBeNull();
  });
});

import { describe, test, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';

const apiMock = vi.hoisted(() => ({
  listRecentActivity: vi.fn(),
}));
vi.mock('../../lib/api.js', () => ({ api: apiMock }));

import RecentActivityCard from '../RecentActivityCard.svelte';

beforeEach(() => {
  apiMock.listRecentActivity.mockReset();
});

describe('RecentActivityCard', () => {
  test('renders entries', async () => {
    apiMock.listRecentActivity.mockResolvedValue({
      entries: [
        { id: 1, category: 'compose', action: 'changed', summary: 'A change', actor_name: 'Ameen', created_at: new Date().toISOString() },
        { id: 2, category: 'deploy', action: 'deploy_succeeded', summary: 'A deploy', actor_name: 'Ameen', created_at: new Date().toISOString() },
      ],
    });
    render(RecentActivityCard);
    await waitFor(() => expect(screen.getByText(/A change/)).toBeTruthy());
    expect(screen.getByText(/A deploy/)).toBeTruthy();
  });

  test('renders empty state', async () => {
    apiMock.listRecentActivity.mockResolvedValue({ entries: [] });
    render(RecentActivityCard);
    await waitFor(() => expect(screen.getByText(/No activity yet/)).toBeTruthy());
  });

  test('view-all link points to System Audit', async () => {
    apiMock.listRecentActivity.mockResolvedValue({ entries: [] });
    const { container } = render(RecentActivityCard);
    await waitFor(() => {
      const link = container.querySelector('a');
      expect(link?.getAttribute('href')).toContain('audit');
    });
  });
});

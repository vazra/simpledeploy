import { describe, test, expect, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';

const apiMock = vi.hoisted(() => ({
  getActivity: vi.fn(),
}));
vi.mock('../api.js', () => ({ api: apiMock }));

import ActivityRow from '../../components/ActivityRow.svelte';

const baseEntry = {
  id: 1,
  category: 'compose',
  action: 'changed',
  summary: 'Image changed: nginx:1.25 → nginx:1.26',
  actor_name: 'Ameen',
  actor_source: 'ui',
  created_at: new Date().toISOString(),
  sync_status: 'synced',
  sync_commit_sha: 'abc1234567890',
};

test('renders summary and actor', () => {
  render(ActivityRow, { entry: baseEntry });
  expect(screen.getByText(/Image changed/)).toBeTruthy();
  expect(screen.getByText(/Ameen/)).toBeTruthy();
});

test('shows synced badge', () => {
  render(ActivityRow, { entry: baseEntry });
  expect(screen.getByText(/synced/i)).toBeTruthy();
});

test('shows pending badge when pending', () => {
  render(ActivityRow, { entry: { ...baseEntry, sync_status: 'pending', sync_commit_sha: '' } });
  expect(screen.getByText(/pending/i)).toBeTruthy();
});

test('shows failed deploy error inline', () => {
  render(ActivityRow, {
    entry: { ...baseEntry, category: 'deploy', action: 'deploy_failed', error: 'image pull denied', sync_status: null },
  });
  expect(screen.getByText(/image pull denied/)).toBeTruthy();
});

test('expand chevron toggles raw json', async () => {
  apiMock.getActivity.mockResolvedValue({ data: { ...baseEntry, before_json: '{"a":1}', after_json: '{"a":2}' } });
  const { container } = render(ActivityRow, { entry: baseEntry, expandable: true });
  const btn = container.querySelector('button[aria-label="Show details"]');
  expect(btn).toBeTruthy();
  await fireEvent.click(btn);
  await waitFor(() => {
    expect(container.textContent).toMatch(/"a"/);
  });
});

test('uses success badge variant for green actions', () => {
  const { container } = render(ActivityRow, { entry: { ...baseEntry, action: 'deploy_succeeded' } });
  expect(container.textContent).toContain('deploy_succeeded');
  const badge = container.querySelector('.text-emerald-400');
  expect(badge).toBeTruthy();
});

test('uses danger badge variant for red actions', () => {
  const { container } = render(ActivityRow, { entry: { ...baseEntry, action: 'removed', sync_status: null } });
  const badge = container.querySelector('.text-red-400');
  expect(badge).toBeTruthy();
});

test('shows app_slug chip with link when showAppColumn is true', () => {
  const { container } = render(ActivityRow, {
    entry: { ...baseEntry, app_slug: 'my-app', sync_status: null },
    showAppColumn: true,
  });
  const link = container.querySelector('a[href="#/apps/my-app"]');
  expect(link).toBeTruthy();
  expect(link.textContent).toContain('my-app');
});

test('does not show app_slug chip when showAppColumn is false', () => {
  const { container } = render(ActivityRow, {
    entry: { ...baseEntry, app_slug: 'my-app', sync_status: null },
    showAppColumn: false,
  });
  const link = container.querySelector('a[href="#/apps/my-app"]');
  expect(link).toBeNull();
});

test('shows View diff link for compose entries with compose_version_id', () => {
  const { container } = render(ActivityRow, {
    entry: { ...baseEntry, app_slug: 'my-app', compose_version_id: 42, sync_status: null },
  });
  const link = container.querySelector('a[href*="versions"]');
  expect(link).toBeTruthy();
  expect(link.textContent).toContain('View diff');
});

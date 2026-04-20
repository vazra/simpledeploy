import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, fireEvent, waitFor } from '@testing-library/svelte';

const apiMock = vi.hoisted(() => ({
  gitStatus: vi.fn(async () => ({ data: null, error: null, status: 503 })),
  gitSyncNow: vi.fn(async () => ({ data: { ok: true }, error: null, status: 200 })),
}));

vi.mock('../../lib/api.js', () => ({ api: apiMock }));
vi.mock('svelte-spa-router', () => ({ push: vi.fn(), link: (n) => n, default: () => null }));
vi.mock('../../components/Layout.svelte', async () => await import('./LayoutStub.svelte'));

import GitSync from '../GitSync.svelte';

const baseStatus = {
  Enabled: true,
  Remote: 'git@github.com:org/repo.git',
  Branch: 'main',
  HeadSHA: 'abc123def456',
  LastSyncAt: new Date(Date.now() - 120000).toISOString(),
  LastSyncError: '',
  PendingCommits: 0,
  RecentConflicts: [],
};

beforeEach(() => {
  vi.clearAllMocks();
});

describe('GitSync', () => {
  it('renders disabled state on 503', async () => {
    apiMock.gitStatus.mockResolvedValueOnce({ data: null, error: 'disabled', status: 503 });
    const { findByText } = render(GitSync);
    expect(await findByText('Git Sync is not enabled')).toBeInTheDocument();
    expect(await findByText('Learn how to set up Git Sync')).toBeInTheDocument();
  });

  it('renders remote and branch on success', async () => {
    apiMock.gitStatus.mockResolvedValueOnce({ data: baseStatus, error: null, status: 200 });
    const { findByText } = render(GitSync);
    expect(await findByText('git@github.com:org/repo.git')).toBeInTheDocument();
    expect(await findByText('main')).toBeInTheDocument();
  });

  it('shows short HEAD SHA', async () => {
    apiMock.gitStatus.mockResolvedValueOnce({ data: baseStatus, error: null, status: 200 });
    const { findByText } = render(GitSync);
    expect(await findByText('abc123de')).toBeInTheDocument();
  });

  it('shows error alert when LastSyncError is non-empty', async () => {
    apiMock.gitStatus.mockResolvedValueOnce({
      data: { ...baseStatus, LastSyncError: 'connection refused' },
      error: null,
      status: 200,
    });
    const { findByText } = render(GitSync);
    expect(await findByText('connection refused')).toBeInTheDocument();
  });

  it('calls gitSyncNow when Sync now is clicked', async () => {
    apiMock.gitStatus.mockResolvedValue({ data: baseStatus, error: null, status: 200 });
    const { findByText } = render(GitSync);
    const btn = await findByText('Sync now');
    await fireEvent.click(btn);
    await waitFor(() => expect(apiMock.gitSyncNow).toHaveBeenCalledTimes(1));
  });

  it('renders no-conflicts message when RecentConflicts is empty', async () => {
    apiMock.gitStatus.mockResolvedValueOnce({ data: baseStatus, error: null, status: 200 });
    const { findByText } = render(GitSync);
    expect(await findByText('No conflicts recorded.')).toBeInTheDocument();
  });

  it('renders conflicts table when conflicts present', async () => {
    const status = {
      ...baseStatus,
      RecentConflicts: [{
        Path: 'myapp/docker-compose.yml',
        RemoteSHA: 'dead1234beef5678',
        ResolvedAt: new Date(Date.now() - 60000).toISOString(),
        Description: 'local wins',
      }],
    };
    apiMock.gitStatus.mockResolvedValueOnce({ data: status, error: null, status: 200 });
    const { findByText } = render(GitSync);
    expect(await findByText('myapp/docker-compose.yml')).toBeInTheDocument();
    expect(await findByText('dead1234')).toBeInTheDocument();
    expect(await findByText('local wins')).toBeInTheDocument();
  });
});

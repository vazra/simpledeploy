import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, fireEvent, waitFor } from '@testing-library/svelte';

const baseCfg = {
  enabled: false,
  remote: '',
  branch: 'main',
  author_name: 'SimpleDeploy',
  author_email: 'bot@simpledeploy.local',
  poll_interval_seconds: 60,
  ssh_key_path: '',
  https_username: 'git',
  webhook_secret_set: false,
  https_token_set: false,
  source: 'yaml',
  poll_enabled: true,
  auto_push_enabled: true,
  auto_apply_enabled: true,
  webhook_enabled: true,
};

const apiMock = vi.hoisted(() => ({
  gitStatus: vi.fn(async () => ({ data: null, error: null, status: 200 })),
  gitSyncNow: vi.fn(async () => ({ data: { ok: true }, error: null, status: 200 })),
  gitConfig: vi.fn(async () => ({ data: { ...baseCfg }, error: null, status: 200 })),
  gitConfigUpdate: vi.fn(async () => ({ data: { Enabled: false }, error: null, status: 200 })),
  gitDisable: vi.fn(async () => ({ data: { Enabled: false }, error: null, status: 200 })),
  gitApplyPending: vi.fn(async () => ({ data: { Enabled: true, CommitsBehind: 0, PendingApply: false }, error: null, status: 200 })),
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
  // Default: cfg returns disabled, status returns null
  apiMock.gitConfig.mockResolvedValue({ data: { ...baseCfg }, error: null, status: 200 });
  apiMock.gitStatus.mockResolvedValue({ data: null, error: null, status: 200 });
});

async function openConfig(utils) {
  const btn = await utils.findByRole('button', { name: 'Configure' });
  await fireEvent.click(btn);
}

describe('GitSync config form', () => {
  it('renders Configure button on load', async () => {
    const utils = render(GitSync);
    expect(await utils.findByRole('button', { name: 'Configure' })).toBeInTheDocument();
  });

  it('opens config modal and shows fields when "Enable Git Sync" is toggled on', async () => {
    const utils = render(GitSync);
    await openConfig(utils);
    const toggle = await utils.findByLabelText('Enable Git Sync');
    await fireEvent.click(toggle);
    await waitFor(() => {
      expect(utils.getByLabelText('Remote URL *')).toBeInTheDocument();
    });
  });

  it('posts expected payload on save', async () => {
    // cfg has enabled=true so form fields appear immediately without clicking toggle
    apiMock.gitConfig.mockResolvedValue({
      data: { ...baseCfg, enabled: true, remote: 'file:///tmp/bare.git' },
      error: null,
      status: 200,
    });
    apiMock.gitConfigUpdate.mockResolvedValueOnce({ data: { Enabled: true }, error: null, status: 200 });

    const utils = render(GitSync);
    await openConfig(utils);
    const toggle = await utils.findByLabelText('Enable Git Sync');
    await waitFor(() => expect(toggle.checked).toBe(true));

    const saveBtn = utils.getByRole('button', { name: 'Save' });
    await fireEvent.click(saveBtn);

    await waitFor(() => expect(apiMock.gitConfigUpdate).toHaveBeenCalledTimes(1));
    const call = apiMock.gitConfigUpdate.mock.calls[0][0];
    expect(call.enabled).toBe(true);
  });

  it('shows webhook_secret input when enabled', async () => {
    apiMock.gitConfig.mockResolvedValue({
      data: { ...baseCfg, enabled: true, remote: 'git@github.com:o/r.git', webhook_secret_set: true },
      error: null,
      status: 200,
    });
    const utils = render(GitSync);
    await openConfig(utils);
    await waitFor(() => {
      expect(utils.container.querySelector('#git-webhook-secret')).toBeTruthy();
    });
  });

  it('shows https_token input when HTTPS auth selected', async () => {
    apiMock.gitConfig.mockResolvedValue({
      data: { ...baseCfg, enabled: true, remote: 'https://github.com/o/r.git', https_token_set: true },
      error: null,
      status: 200,
    });
    const utils = render(GitSync);
    await openConfig(utils);
    await waitFor(() => utils.container.querySelector('#git-webhook-secret'));
    const httpsRadio = await utils.findByLabelText('HTTPS token');
    await fireEvent.click(httpsRadio);
    await waitFor(() => {
      expect(utils.container.querySelector('#git-https-token')).toBeTruthy();
    });
  });

  it('shows error message on failed save', async () => {
    apiMock.gitConfig.mockResolvedValue({
      data: { ...baseCfg, enabled: true, remote: 'file:///tmp/bare.git' },
      error: null,
      status: 200,
    });
    apiMock.gitConfigUpdate.mockResolvedValueOnce({ data: null, error: 'remote is required when enabled', status: 400 });
    const utils = render(GitSync);
    await openConfig(utils);
    const toggle = await utils.findByLabelText('Enable Git Sync');
    await waitFor(() => expect(toggle.checked).toBe(true));
    const saveBtn = utils.getByRole('button', { name: 'Save' });
    await fireEvent.click(saveBtn);
    await waitFor(() => expect(apiMock.gitConfigUpdate).toHaveBeenCalled());
    expect(await utils.findByText('remote is required when enabled')).toBeInTheDocument();
  });
});

describe('GitSync status', () => {
  it('renders remote and branch when status is enabled', async () => {
    apiMock.gitStatus.mockResolvedValue({ data: baseStatus, error: null, status: 200 });
    apiMock.gitConfig.mockResolvedValue({ data: { ...baseCfg, enabled: true }, error: null, status: 200 });
    const { findByText } = render(GitSync);
    expect(await findByText('git@github.com:org/repo.git')).toBeInTheDocument();
  });

  it('shows short HEAD SHA', async () => {
    apiMock.gitStatus.mockResolvedValue({ data: baseStatus, error: null, status: 200 });
    apiMock.gitConfig.mockResolvedValue({ data: { ...baseCfg, enabled: true }, error: null, status: 200 });
    const { findByText } = render(GitSync);
    expect(await findByText('abc123de')).toBeInTheDocument();
  });

  it('shows error alert when LastSyncError is non-empty', async () => {
    apiMock.gitStatus.mockResolvedValue({
      data: { ...baseStatus, LastSyncError: 'connection refused' },
      error: null,
      status: 200,
    });
    apiMock.gitConfig.mockResolvedValue({ data: { ...baseCfg, enabled: true }, error: null, status: 200 });
    const { findByText } = render(GitSync);
    expect(await findByText('connection refused')).toBeInTheDocument();
  });

  it('calls gitSyncNow when Sync now is clicked', async () => {
    apiMock.gitStatus.mockResolvedValue({ data: baseStatus, error: null, status: 200 });
    apiMock.gitConfig.mockResolvedValue({ data: { ...baseCfg, enabled: true }, error: null, status: 200 });
    const { findByText } = render(GitSync);
    const btn = await findByText('Sync now');
    await fireEvent.click(btn);
    await waitFor(() => expect(apiMock.gitSyncNow).toHaveBeenCalledTimes(1));
  });

  it('renders no-conflicts message when RecentConflicts is empty', async () => {
    apiMock.gitStatus.mockResolvedValue({ data: baseStatus, error: null, status: 200 });
    apiMock.gitConfig.mockResolvedValue({ data: { ...baseCfg, enabled: true }, error: null, status: 200 });
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
    apiMock.gitStatus.mockResolvedValue({ data: status, error: null, status: 200 });
    apiMock.gitConfig.mockResolvedValue({ data: { ...baseCfg, enabled: true }, error: null, status: 200 });
    const { findByText } = render(GitSync);
    expect(await findByText('myapp/docker-compose.yml')).toBeInTheDocument();
    expect(await findByText('local wins')).toBeInTheDocument();
  });

  it('does not show status section when Git Sync is disabled', async () => {
    apiMock.gitStatus.mockResolvedValue({ data: null, error: null, status: 200 });
    apiMock.gitConfig.mockResolvedValue({ data: { ...baseCfg, enabled: false }, error: null, status: 200 });
    const { findByRole, queryByText } = render(GitSync);
    await findByRole('button', { name: 'Configure' });
    expect(queryByText('Sync Status')).toBeNull();
  });

  it('shows not-admin message on 403', async () => {
    apiMock.gitStatus.mockResolvedValue({ data: null, error: 'forbidden', status: 403 });
    apiMock.gitConfig.mockResolvedValue({ data: null, error: 'forbidden', status: 403 });
    const { findByText } = render(GitSync);
    expect(await findByText('Git Sync is restricted to super admins.')).toBeInTheDocument();
  });
});

describe('GitSync toggle checkboxes', () => {
  it('renders four behaviour toggle checkboxes when enabled', async () => {
    apiMock.gitConfig.mockResolvedValue({
      data: { ...baseCfg, enabled: true, remote: 'git@github.com:o/r.git' },
      error: null,
      status: 200,
    });
    const utils = render(GitSync);
    await openConfig(utils);
    expect(await utils.findByText('Poll for remote changes')).toBeInTheDocument();
    expect(await utils.findByText('Auto-push local changes')).toBeInTheDocument();
    expect(await utils.findByText('Auto-apply remote changes')).toBeInTheDocument();
    expect(await utils.findByText('Enable webhook endpoint')).toBeInTheDocument();
  });

  it('hides poll interval input when poll_enabled is false', async () => {
    apiMock.gitConfig.mockResolvedValue({
      data: { ...baseCfg, enabled: true, remote: 'git@github.com:o/r.git', poll_enabled: false },
      error: null,
      status: 200,
    });
    const utils = render(GitSync);
    await openConfig(utils);
    await utils.findByText('Poll for remote changes');
    await waitFor(() => {
      expect(utils.container.querySelector('#git-poll')).toBeNull();
    });
  });

  it('includes toggle values in PUT payload', async () => {
    apiMock.gitConfig.mockResolvedValue({
      data: { ...baseCfg, enabled: true, remote: 'file:///tmp/bare.git' },
      error: null,
      status: 200,
    });
    apiMock.gitConfigUpdate.mockResolvedValueOnce({ data: { Enabled: true }, error: null, status: 200 });

    const utils = render(GitSync);
    await openConfig(utils);
    const toggle = await utils.findByLabelText('Enable Git Sync');
    await waitFor(() => expect(toggle.checked).toBe(true));

    const saveBtn = utils.getByRole('button', { name: 'Save' });
    await fireEvent.click(saveBtn);

    await waitFor(() => expect(apiMock.gitConfigUpdate).toHaveBeenCalledTimes(1));
    const call = apiMock.gitConfigUpdate.mock.calls[0][0];
    expect(call).toHaveProperty('poll_enabled');
    expect(call).toHaveProperty('auto_push_enabled');
    expect(call).toHaveProperty('auto_apply_enabled');
    expect(call).toHaveProperty('webhook_enabled');
  });
});

describe('GitSync pending apply banner', () => {
  it('shows banner when PendingApply is true', async () => {
    const pendingStatus = {
      ...baseStatus,
      PendingApply: true,
      CommitsBehind: 3,
      AutoApplyEnabled: false,
    };
    apiMock.gitStatus.mockResolvedValue({ data: pendingStatus, error: null, status: 200 });
    apiMock.gitConfig.mockResolvedValue({ data: { ...baseCfg, enabled: true }, error: null, status: 200 });
    const { findByText } = render(GitSync);
    expect(await findByText(/3 commits behind/)).toBeInTheDocument();
    expect(await findByText('Apply now')).toBeInTheDocument();
  });

  it('does not show banner when PendingApply is false', async () => {
    apiMock.gitStatus.mockResolvedValue({ data: baseStatus, error: null, status: 200 });
    apiMock.gitConfig.mockResolvedValue({ data: { ...baseCfg, enabled: true }, error: null, status: 200 });
    const { findByText, queryByText } = render(GitSync);
    await findByText('Sync Status');
    expect(queryByText('Apply now')).toBeNull();
  });

  it('calls gitApplyPending when Apply now is clicked', async () => {
    const pendingStatus = {
      ...baseStatus,
      PendingApply: true,
      CommitsBehind: 2,
      AutoApplyEnabled: false,
    };
    apiMock.gitStatus.mockResolvedValue({ data: pendingStatus, error: null, status: 200 });
    apiMock.gitConfig.mockResolvedValue({ data: { ...baseCfg, enabled: true }, error: null, status: 200 });
    const { findByText } = render(GitSync);
    const btn = await findByText('Apply now');
    await fireEvent.click(btn);
    await waitFor(() => expect(apiMock.gitApplyPending).toHaveBeenCalledTimes(1));
  });
});

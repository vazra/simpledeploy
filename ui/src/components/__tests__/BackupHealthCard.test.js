import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import BackupHealthCard from '../BackupHealthCard.svelte';

function makeApp(overrides = {}) {
  return {
    app_slug: 'foo',
    app_name: 'Foo',
    config_count: 1,
    strategies: ['postgres'],
    total_size_bytes: 0,
    recent_fail_count: 0,
    last_run_finished_at: null,
    last_run_status: null,
    ...overrides,
  };
}

describe('BackupHealthCard', () => {
  it('renders app name and links to the backups tab', () => {
    const { getByText, container } = render(BackupHealthCard, { app: makeApp({ app_name: 'My App' }) });
    expect(getByText('My App')).toBeInTheDocument();
    expect(container.querySelector('a').getAttribute('href')).toBe('#/apps/foo?tab=backups');
  });

  it('shows "Never" when no runs yet', () => {
    const { getByText } = render(BackupHealthCard, { app: makeApp() });
    expect(getByText('Never')).toBeInTheDocument();
  });

  it('renders a badge per strategy, with friendly labels', () => {
    const { getByText } = render(BackupHealthCard, {
      app: makeApp({ strategies: ['postgres', 'volume'] }),
    });
    expect(getByText('DB')).toBeInTheDocument();
    expect(getByText('Files')).toBeInTheDocument();
  });

  it('falls back to "None" when no strategies', () => {
    const { getByText } = render(BackupHealthCard, {
      app: makeApp({ strategies: [] }),
    });
    expect(getByText('None')).toBeInTheDocument();
  });

  it('shows 24h failure count when non-zero', () => {
    const { getByText } = render(BackupHealthCard, {
      app: makeApp({ recent_fail_count: 3 }),
    });
    expect(getByText('24h failures')).toBeInTheDocument();
    expect(getByText('3')).toBeInTheDocument();
  });

  it('uses success dot when last run succeeded', () => {
    const { container } = render(BackupHealthCard, {
      app: makeApp({ last_run_status: 'success' }),
    });
    expect(container.querySelector('.bg-success')).not.toBeNull();
  });

  it('uses danger dot when last run failed', () => {
    const { container } = render(BackupHealthCard, {
      app: makeApp({ last_run_status: 'failed' }),
    });
    expect(container.querySelector('.bg-danger')).not.toBeNull();
  });

  it('formats total_size_bytes', () => {
    const { getByText } = render(BackupHealthCard, {
      app: makeApp({ total_size_bytes: 1024 * 1024 * 5 }),
    });
    expect(getByText('5.0 MB')).toBeInTheDocument();
  });
});

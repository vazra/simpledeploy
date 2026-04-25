import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, waitFor, screen, fireEvent } from '@testing-library/svelte';

const mocks = vi.hoisted(() => ({
  listActivity: vi.fn(),
  getAuditConfig: vi.fn(),
  putAuditConfig: vi.fn(),
  getProfile: vi.fn(),
  listApps: vi.fn(),
  systemInfo: vi.fn(),
  systemStorageBreakdown: vi.fn(),
}));

vi.mock('../../lib/api.js', async () => {
  const { makeApiMock } = await import('../../test-mocks/api.js');
  return {
    api: makeApiMock({
      systemInfo: mocks.systemInfo,
      systemStorageBreakdown: mocks.systemStorageBreakdown,
      listActivity: mocks.listActivity,
      getAuditConfig: mocks.getAuditConfig,
      putAuditConfig: mocks.putAuditConfig,
      getProfile: mocks.getProfile,
      listApps: mocks.listApps,
    }),
  };
});

vi.mock('svelte-spa-router', () => ({ push: vi.fn(), link: (n) => n, default: () => null }));
vi.mock('../../components/Layout.svelte', async () => await import('./LayoutStub.svelte'));

import System from '../System.svelte';

beforeEach(() => {
  mocks.systemInfo.mockResolvedValue({
    data: {
      simpledeploy: { version: '1.2.3', deployment_mode: 'native', deployment_label: 'Native', process: { mem_alloc: 1024 * 1024 } },
      database: { size_bytes: 1024 },
    },
    error: null,
  });
  mocks.systemStorageBreakdown.mockResolvedValue({ data: {}, error: null });
  mocks.listActivity.mockResolvedValue({ data: { entries: [], next_before: 0 }, error: null });
  mocks.getAuditConfig.mockResolvedValue({ data: { retention_days: 0 }, error: null });
  mocks.putAuditConfig.mockResolvedValue({ data: {}, error: null });
  mocks.getProfile.mockResolvedValue({ data: { username: 'admin', role: 'super_admin' }, error: null });
  mocks.listApps.mockResolvedValue({ data: [], error: null });
});

describe('System route', () => {
  it('renders system info with version', async () => {
    const { findByText } = render(System);
    expect(await findByText(/1\.2\.3/)).toBeInTheDocument();
  });

  it('Audit tab loads entries via listActivity', async () => {
    mocks.listActivity.mockResolvedValue({
      data: {
        entries: [
          { id: 1, category: 'auth', action: 'login_succeeded', summary: 'admin logged in', actor_name: 'admin', created_at: new Date().toISOString() },
          { id: 2, category: 'app', action: 'added', summary: 'App created', actor_name: 'admin', created_at: new Date().toISOString() },
          { id: 3, category: 'deploy', action: 'deploy_succeeded', summary: 'Deploy ok', actor_name: 'admin', created_at: new Date().toISOString() },
        ],
        next_before: 0,
      },
      error: null,
    });

    const { findByText } = render(System);
    const auditTab = await findByText('Audit Log');
    await fireEvent.click(auditTab);

    await waitFor(() => {
      expect(screen.getByText(/admin logged in/)).toBeInTheDocument();
      expect(screen.getByText(/App created/)).toBeInTheDocument();
      expect(screen.getByText(/Deploy ok/)).toBeInTheDocument();
    });
  });

  it('Retention save calls putAuditConfig with retention_days', async () => {
    const { findByText } = render(System);

    const auditTab = await findByText('Audit Log');
    await fireEvent.click(auditTab);

    // Wait for super-admin retention section to appear
    const input = await waitFor(() => {
      const els = Array.from(document.querySelectorAll('input[type="number"]'));
      const el = els.find(e => e.getAttribute('min') === '0');
      if (!el) throw new Error('retention input not found');
      return el;
    });

    await fireEvent.input(input, { target: { value: '60' } });

    const saveBtn = await waitFor(() => {
      const btns = Array.from(document.querySelectorAll('button'));
      const btn = btns.find(b => b.textContent.trim() === 'Save');
      if (!btn) throw new Error('Save button not found');
      return btn;
    });
    await fireEvent.click(saveBtn);

    await waitFor(() => {
      expect(mocks.putAuditConfig).toHaveBeenCalledWith(expect.objectContaining({ retention_days: expect.any(Number) }));
    });
  });
});

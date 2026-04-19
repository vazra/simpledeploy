import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent, waitFor } from '@testing-library/svelte';
import { routerMocks } from './helpers.js';

const apiMock = vi.hoisted(() => ({
  getProfile: vi.fn(async () => ({
    data: { username: 'admin', display_name: 'Admin', email: 'a@b.co', role: 'admin', created_at: '2026-01-01T00:00:00Z' },
    error: null,
  })),
  updateProfile: vi.fn(async () => ({ data: {}, error: null })),
  changePassword: vi.fn(async () => ({ data: {}, error: null })),
  logout: vi.fn(async () => ({ data: {}, error: null })),
}));

const router = vi.hoisted(() => ({ push: vi.fn() }));
vi.mock('../../lib/api.js', () => ({ api: apiMock }));
vi.mock('svelte-spa-router', () => ({ push: router.push, link: (n) => n, default: () => null }));
vi.mock('../../components/Layout.svelte', async () => await import('./LayoutStub.svelte'));

import Profile from '../Profile.svelte';

describe('Profile', () => {
  it('loads and renders the current profile data', async () => {
    const { findByDisplayValue, findAllByText } = render(Profile);
    expect(await findByDisplayValue('Admin')).toBeInTheDocument();
    expect(await findByDisplayValue('a@b.co')).toBeInTheDocument();
    expect((await findAllByText('admin')).length).toBeGreaterThan(0);
  });

  it('saves profile edits via api.updateProfile', async () => {
    const { findByDisplayValue, findByText } = render(Profile);
    const name = await findByDisplayValue('Admin');
    await fireEvent.input(name, { target: { value: 'Admin 2' } });
    await fireEvent.click(await findByText('Save Profile'));
    await waitFor(() => expect(apiMock.updateProfile).toHaveBeenCalledWith({ display_name: 'Admin 2', email: 'a@b.co' }));
  });

  it('rejects password change when new password does not match', async () => {
    apiMock.changePassword.mockClear();
    const { findByLabelText, findAllByText } = render(Profile);
    const cur = await findByLabelText('Current Password');
    await fireEvent.input(cur, { target: { value: 'old' } });
    await fireEvent.input(await findByLabelText('New Password'), { target: { value: 'new123' } });
    await fireEvent.input(await findByLabelText('Confirm Password'), { target: { value: 'different' } });
    const buttons = await findAllByText('Change Password');
    // The clickable <button> is the last "Change Password" element (heading is first).
    await fireEvent.click(buttons[buttons.length - 1]);
    expect(apiMock.changePassword).not.toHaveBeenCalled();
  });

  it('calls api.logout then pushes to /login', async () => {
    apiMock.logout.mockClear();
    router.push.mockClear();
    const { findByText } = render(Profile);
    await fireEvent.click(await findByText('Log out'));
    await waitFor(() => expect(apiMock.logout).toHaveBeenCalled());
    await waitFor(() => expect(router.push).toHaveBeenCalledWith('/login'));
  });
});

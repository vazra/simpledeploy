import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent, waitFor } from '@testing-library/svelte';
import { routerMocks } from './helpers.js';

const apiMock = vi.hoisted(() => ({
  setupStatus: vi.fn(async () => ({ data: { needs_setup: false }, error: null })),
  setup: vi.fn(async () => ({ data: {}, error: null })),
  login: vi.fn(async () => ({ data: {}, error: null })),
}));

const router = vi.hoisted(() => ({ push: vi.fn() }));

vi.mock('../../lib/api.js', () => ({ api: apiMock }));
vi.mock('svelte-spa-router', () => {
  const { routerMocks } = require('./helpers.js');
  return { ...routerMocks(), push: router.push };
});

import Login from '../Login.svelte';

describe('Login', () => {
  it('renders Sign In form when setup not needed', async () => {
    apiMock.setupStatus.mockResolvedValueOnce({ data: { needs_setup: false }, error: null });
    const { findByText, queryByText } = render(Login);
    expect(await findByText('Sign in to continue')).toBeInTheDocument();
    expect(queryByText('Welcome to SimpleDeploy')).toBeNull();
  });

  it('renders setup form when needs_setup=true', async () => {
    apiMock.setupStatus.mockResolvedValueOnce({ data: { needs_setup: true }, error: null });
    const { findByText } = render(Login);
    expect(await findByText('Welcome to SimpleDeploy')).toBeInTheDocument();
  });

  it('shows invalid-credentials error on login failure', async () => {
    apiMock.setupStatus.mockResolvedValueOnce({ data: { needs_setup: false }, error: null });
    apiMock.login.mockResolvedValueOnce({ data: null, error: 'nope' });
    const { findByText, getByLabelText, getByRole } = render(Login);
    await findByText('Sign in to continue');
    await fireEvent.input(getByLabelText('Username'), { target: { value: 'u' } });
    await fireEvent.input(getByLabelText('Password'), { target: { value: 'p' } });
    await fireEvent.click(getByRole('button', { name: /Sign In/i }));
    expect(await findByText('Invalid credentials')).toBeInTheDocument();
  });

  it('pushes to / on successful login', async () => {
    router.push.mockClear();
    apiMock.setupStatus.mockResolvedValueOnce({ data: { needs_setup: false }, error: null });
    apiMock.login.mockResolvedValueOnce({ data: {}, error: null });
    const { findByText, getByLabelText, getByRole } = render(Login);
    await findByText('Sign in to continue');
    await fireEvent.input(getByLabelText('Username'), { target: { value: 'u' } });
    await fireEvent.input(getByLabelText('Password'), { target: { value: 'p' } });
    await fireEvent.click(getByRole('button', { name: /Sign In/i }));
    await waitFor(() => expect(router.push).toHaveBeenCalledWith('/'));
  });

  it('setup form does not call api.setup when passwords mismatch', async () => {
    apiMock.setupStatus.mockResolvedValueOnce({ data: { needs_setup: true }, error: null });
    apiMock.setup.mockClear();
    const { findByText, getByLabelText, getByRole } = render(Login);
    await findByText('Welcome to SimpleDeploy');
    await fireEvent.input(getByLabelText('Full Name'), { target: { value: 'Jane' } });
    await fireEvent.input(getByLabelText('Email'), { target: { value: 'j@ex.com' } });
    await fireEvent.input(getByLabelText('Username'), { target: { value: 'jane' } });
    await fireEvent.input(getByLabelText('Password'), { target: { value: 'longerpassword1' } });
    await fireEvent.input(getByLabelText('Confirm Password'), { target: { value: 'mismatch1!' } });
    await fireEvent.click(getByRole('button', { name: /Create Account/i }));
    expect(apiMock.setup).not.toHaveBeenCalled();
  });
});

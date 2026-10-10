import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, waitFor, fireEvent } from '@testing-library/svelte';
import { flushSync } from 'svelte';
import { get } from 'svelte/store';

vi.mock('../../lib/api.js', async () => {
  const { makeApiMock } = await import('../../test-mocks/api.js');
  return {
    api: makeApiMock({
      getCompose: vi.fn(async () => ({ data: 'services:\n  web:\n    image: nginx\n', error: null })),
    }),
  };
});

import ConfigTab from '../ConfigTab.svelte';
import { api } from '../../lib/api.js';
import { toasts } from '../../lib/stores/toast.js';

async function renderInYamlMode(props = {}) {
  const r = render(ConfigTab, { slug: 'foo', ...props });
  await r.findByText(/Save.*Deploy/i);
  await waitFor(() => expect(api.getEnv).toHaveBeenCalled());
  flushSync(() => r.component.switchToMode('yaml'));
  return r;
}

describe('ConfigTab', () => {
  beforeEach(() => {
    for (const t of get(toasts)) toasts.remove(t.id);
    api.getEnv.mockClear();
    api.putEnv.mockClear();
    api.deploy.mockClear();
  });

  it('renders the Save & Deploy button after load', async () => {
    const { findByText } = render(ConfigTab, { slug: 'foo' });
    expect(await findByText(/Save.*Deploy/i)).toBeInTheDocument();
  });

  it('renders deploy history accordion', async () => {
    const { findByText } = render(ConfigTab, { slug: 'foo' });
    expect(await findByText(/Deploy History/i)).toBeInTheDocument();
  });

  it('shows masked env values read-only in YAML mode and never saves them, even with edit rights', async () => {
    api.getEnv.mockResolvedValueOnce({ data: [{ key: 'SECRET', value: '', masked: true }], error: null });
    const { findByTestId, container } = await renderInYamlMode({ canEdit: true });
    expect(await findByTestId('env-readonly-hint')).toHaveTextContent(/hidden/i);
    const envArea = container.querySelectorAll('textarea')[1];
    await waitFor(() => expect(envArea.value).toBe('SECRET=•••••• (hidden for viewers)'));
    expect(envArea.readOnly).toBe(true);
    expect(api.putEnv).not.toHaveBeenCalled();
  });

  it('makes .env read-only from the user role, not from masked entries', async () => {
    // A viewer with an empty (so unmasked) .env must still get a read-only editor.
    api.getEnv.mockResolvedValueOnce({ data: [], error: null });
    const { findByTestId, container } = await renderInYamlMode({ canEdit: false });
    expect(await findByTestId('env-readonly-hint')).toHaveTextContent(/view-only/i);
    expect(container.querySelectorAll('textarea')[1].readOnly).toBe(true);
  });

  it('lets users with edit rights change .env', async () => {
    api.getEnv.mockResolvedValueOnce({ data: [{ key: 'A', value: '1' }], error: null });
    const { queryByTestId, container } = await renderInYamlMode({ canEdit: true });
    const envArea = container.querySelectorAll('textarea')[1];
    await waitFor(() => expect(envArea.value).toBe('A=1'));
    expect(envArea.readOnly).toBe(false);
    expect(queryByTestId('env-readonly-hint')).toBeNull();
  });

  it('shows refusal reasons and keeps the review open when deploy is refused', async () => {
    const reason = 'compose file contains disallowed directives:\n- service "web": privileged not allowed';
    api.deploy.mockResolvedValueOnce({ data: null, error: reason, violations: ['service "web": privileged not allowed'], status: 400 });
    const { container, findByText, findByTestId, getByText } = await renderInYamlMode({ canEdit: true });
    const composeArea = container.querySelectorAll('textarea')[0];
    await fireEvent.input(composeArea, { target: { value: 'services:\n  web:\n    image: nginx\n    privileged: true\n' } });
    await fireEvent.click(getByText(/Save.*Deploy/i));
    await fireEvent.click(await findByText('Confirm & Deploy'));
    await waitFor(() => expect(api.deploy).toHaveBeenCalled());

    // diff modal stays open with the reason inline
    const box = await findByTestId('diff-error');
    expect(box).toHaveTextContent('privileged not allowed');
    expect(getByText('Review Changes')).toBeInTheDocument();
    // and a toast carries the same reason
    const t = get(toasts);
    expect(t.some((x) => x.type === 'error' && x.message === reason)).toBe(true);
  });

  it('closes the review after a successful deploy', async () => {
    const { container, findByText, queryByText, getByText } = await renderInYamlMode({ canEdit: true });
    const composeArea = container.querySelectorAll('textarea')[0];
    await fireEvent.input(composeArea, { target: { value: 'services:\n  web:\n    image: nginx:alpine\n' } });
    await fireEvent.click(getByText(/Save.*Deploy/i));
    await fireEvent.click(await findByText('Confirm & Deploy'));
    await waitFor(() => expect(queryByText('Review Changes')).toBeNull());
    expect(api.deploy).toHaveBeenCalled();
  });

  it('does not deploy when the .env save fails', async () => {
    api.getEnv.mockResolvedValueOnce({ data: [{ key: 'A', value: '1' }], error: null });
    api.putEnv.mockResolvedValueOnce({ data: null, error: 'refused' });
    const { container, findByText, findByTestId, getByText } = await renderInYamlMode({ canEdit: true });
    const [composeArea, envArea] = container.querySelectorAll('textarea');
    await waitFor(() => expect(envArea.value).toBe('A=1'));
    await fireEvent.input(envArea, { target: { value: 'A=2' } });
    await fireEvent.input(composeArea, { target: { value: 'services:\n  web:\n    image: nginx:alpine\n' } });
    await fireEvent.click(getByText(/Save.*Deploy/i));
    await fireEvent.click(await findByText('Confirm & Deploy'));
    await waitFor(() => expect(api.putEnv).toHaveBeenCalled());
    expect(await findByTestId('diff-error')).toHaveTextContent(/\.env/);
    expect(api.deploy).not.toHaveBeenCalled();
  });
});

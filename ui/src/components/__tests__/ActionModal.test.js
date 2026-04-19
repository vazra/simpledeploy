import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';

const fakeSocket = { onmessage: null, onclose: null, close: vi.fn() };

vi.mock('../../lib/api.js', async () => {
  const { makeApiMock } = await import('../../test-mocks/api.js');
  return { api: makeApiMock({ deployLogsWs: () => fakeSocket }) };
});

// The terminal scroll callback runs via requestAnimationFrame, which can fire
// after cleanup and hit a null element; run it sync + guard against null.
const origRAF = globalThis.requestAnimationFrame;
beforeEach(() => {
  globalThis.requestAnimationFrame = (cb) => { try { cb(0); } catch {} return 0; };
});
afterEach(() => {
  globalThis.requestAnimationFrame = origRAF;
});

import { beforeEach, afterEach } from 'vitest';

import ActionModal from '../ActionModal.svelte';

describe('ActionModal', () => {
  it('renders nothing when show=false', () => {
    const { queryByRole } = render(ActionModal, {
      slug: 'foo',
      action: 'Deploying',
      show: false,
      onclose: () => {},
    });
    expect(queryByRole('dialog')).toBeNull();
  });

  it('renders dialog with in-progress state when show=true', () => {
    const { getByRole, getByText } = render(ActionModal, {
      slug: 'foo',
      action: 'Deploying',
      show: true,
      onclose: () => {},
    });
    expect(getByRole('dialog')).toBeInTheDocument();
    expect(getByText('Deploying...')).toBeInTheDocument();
    expect(getByText('Waiting for output...')).toBeInTheDocument();
  });

  it('streams log lines from the websocket', async () => {
    const { getByText } = render(ActionModal, {
      slug: 'foo',
      action: 'Deploying',
      show: true,
      onclose: () => {},
    });
    fakeSocket.onmessage({ data: JSON.stringify({ line: 'hello line', stream: 'stdout' }) });
    await Promise.resolve();
    expect(getByText('hello line')).toBeInTheDocument();
  });

  it('shows Complete when a done message arrives', async () => {
    const { getByText } = render(ActionModal, {
      slug: 'foo',
      action: 'Deploying',
      show: true,
      onclose: () => {},
    });
    fakeSocket.onmessage({ data: JSON.stringify({ done: true, action: 'deploy-completed' }) });
    await Promise.resolve();
    expect(getByText('Complete')).toBeInTheDocument();
  });

  it('shows Failed when the done action contains "failed"', async () => {
    const { getByText } = render(ActionModal, {
      slug: 'foo',
      action: 'Deploying',
      show: true,
      onclose: () => {},
    });
    fakeSocket.onmessage({ data: JSON.stringify({ done: true, action: 'deploy-failed' }) });
    await Promise.resolve();
    expect(getByText('Failed')).toBeInTheDocument();
  });

  it('Close button triggers onclose once done', async () => {
    const onclose = vi.fn();
    const { getByText } = render(ActionModal, {
      slug: 'foo',
      action: 'Deploying',
      show: true,
      onclose,
    });
    fakeSocket.onmessage({ data: JSON.stringify({ done: true, action: 'deploy-completed' }) });
    await Promise.resolve();
    await fireEvent.click(getByText('Close'));
    expect(onclose).toHaveBeenCalled();
  });
});

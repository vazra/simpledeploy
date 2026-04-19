import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, fireEvent, waitFor } from '@testing-library/svelte';

// Guard against the async rAF callback firing after unmount.
const origRAF = globalThis.requestAnimationFrame;
globalThis.requestAnimationFrame = (cb) => { try { cb(0); } catch {} return 0; };
afterEach(() => { /* keep the guard active for the whole file */ });

const apiMock = vi.hoisted(() => ({
  getAppServices: vi.fn(async () => ({ data: [{ service: 'web' }, { service: 'worker' }], error: null })),
}));
vi.mock('../../lib/api.js', () => ({ api: apiMock }));

// Minimal WebSocket stub
const wsInstances = [];
class FakeWS {
  constructor(url) {
    this.url = url;
    wsInstances.push(this);
  }
  close() {}
}

import LogViewer from '../LogViewer.svelte';

describe('LogViewer', () => {
  beforeEach(() => {
    wsInstances.length = 0;
    global.WebSocket = FakeWS;
  });

  it('opens a ws with the slug in the URL', async () => {
    render(LogViewer, { slug: 'foo' });
    await waitFor(() => expect(wsInstances.length).toBeGreaterThan(0));
    expect(wsInstances[0].url).toMatch(/\/api\/apps\/foo\/logs/);
  });

  it('renders service tabs when >1 services', async () => {
    const { findByText } = render(LogViewer, { slug: 'foo' });
    expect(await findByText('web')).toBeInTheDocument();
    expect(await findByText('worker')).toBeInTheDocument();
  });

  it('toggles follow button label', async () => {
    const { findByText } = render(LogViewer, { slug: 'foo' });
    const btn = await findByText('Following');
    await fireEvent.click(btn);
    expect(await findByText('Paused')).toBeInTheDocument();
  });

  it('appends incoming log lines', async () => {
    const { container } = render(LogViewer, { slug: 'foo' });
    await waitFor(() => expect(wsInstances.length).toBeGreaterThan(0));
    const ws = wsInstances[0];
    ws.onmessage({ data: JSON.stringify({ line: 'hello', stream: 'stdout', ts: '' }) });
    await waitFor(() => expect(container.textContent).toMatch(/hello/));
  });

  it('shows error message when ws delivers an error', async () => {
    const { container } = render(LogViewer, { slug: 'foo' });
    await waitFor(() => expect(wsInstances.length).toBeGreaterThan(0));
    const ws = wsInstances[0];
    ws.onmessage({ data: JSON.stringify({ error: 'permission denied' }) });
    await waitFor(() => expect(container.textContent).toMatch(/permission denied/));
  });

  it('clear resets the line list', async () => {
    const { container, findByText } = render(LogViewer, { slug: 'foo' });
    await waitFor(() => expect(wsInstances.length).toBeGreaterThan(0));
    const ws = wsInstances[0];
    ws.onmessage({ data: JSON.stringify({ line: 'x', stream: 'stdout', ts: '' }) });
    await waitFor(() => expect(container.textContent).toMatch(/x/));
    await fireEvent.click(await findByText('Clear'));
    await waitFor(() => expect(container.textContent).not.toMatch(/x\n/));
  });
});

import { beforeEach } from 'vitest';

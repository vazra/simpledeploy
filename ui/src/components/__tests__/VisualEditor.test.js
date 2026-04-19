import { describe, it, expect, vi } from 'vitest';
import { render } from '@testing-library/svelte';

vi.mock('../../lib/api.js', async () => {
  const { makeApiMock } = await import('../../test-mocks/api.js');
  return { api: makeApiMock() };
});

import VisualEditor from '../VisualEditor.svelte';

describe('VisualEditor (smoke)', () => {
  it('renders with an empty compose without crashing', () => {
    const { container } = render(VisualEditor, { compose: { services: {} }, slug: '' });
    expect(container.firstChild).not.toBeNull();
  });

  it('renders with a minimal single-service compose', () => {
    const compose = {
      services: {
        web: {
          image: 'nginx:alpine',
          labels: {
            'simpledeploy.endpoints.0.domain': 'example.com',
            'simpledeploy.endpoints.0.port': '80',
            'simpledeploy.endpoints.0.tls': 'letsencrypt',
          },
        },
      },
    };
    const { container } = render(VisualEditor, { compose, slug: 'foo' });
    expect(container.textContent).toMatch(/nginx/);
  });

  it('renders some accordion section buttons for a populated compose', () => {
    const compose = {
      services: {
        api: {
          image: 'caddy:latest',
          labels: {
            'simpledeploy.endpoints.0.domain': 'api.example.com',
            'simpledeploy.endpoints.0.port': '8080',
            'simpledeploy.endpoints.0.tls': 'letsencrypt',
          },
        },
      },
    };
    const { container } = render(VisualEditor, { compose, slug: 'foo' });
    // Accordion headers are rendered as buttons even when sections are collapsed.
    expect(container.querySelectorAll('button').length).toBeGreaterThan(0);
  });
});

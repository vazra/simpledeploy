import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import AppCard from '../AppCard.svelte';

describe('AppCard', () => {
  it('renders app name, status badge, and links to detail', () => {
    const { getByText, container } = render(AppCard, {
      app: { Slug: 'foo', Name: 'Foo App', Status: 'running', Domain: 'foo.example.com' },
    });
    expect(getByText('Foo App')).toBeInTheDocument();
    expect(getByText('running')).toBeInTheDocument();
    expect(getByText('foo.example.com')).toBeInTheDocument();
    expect(container.querySelector('a').getAttribute('href')).toBe('#/apps/foo');
  });

  it('uses the danger color dot for error status', () => {
    const { container } = render(AppCard, {
      app: { Slug: 'foo', Name: 'Foo', Status: 'error' },
    });
    expect(container.querySelector('.bg-danger')).not.toBeNull();
  });

  it('uses the warning color dot and warning badge for unstable status', () => {
    const { container, getByText } = render(AppCard, {
      app: { Slug: 'foo', Name: 'Foo', Status: 'unstable' },
    });
    expect(container.querySelector('.bg-warning')).not.toBeNull();
    expect(getByText('unstable')).toBeInTheDocument();
  });

  it('omits domain line when app has no domain', () => {
    const { queryByText } = render(AppCard, {
      app: { Slug: 'foo', Name: 'Foo', Status: 'running' },
    });
    expect(queryByText(/example\.com/)).toBeNull();
  });

  it('renders CPU/MEM bars when metrics are supplied', () => {
    const { getByText } = render(AppCard, {
      app: { Slug: 'foo', Name: 'Foo', Status: 'running' },
      metrics: { cpu: 42.5, memPct: 30, memBytes: 1024 * 1024 * 100, memLimit: 1024 * 1024 * 512 },
    });
    expect(getByText('CPU')).toBeInTheDocument();
    expect(getByText('MEM')).toBeInTheDocument();
    expect(getByText('42.5%')).toBeInTheDocument();
  });

  it('clamps CPU bar width at 100%', () => {
    const { container } = render(AppCard, {
      app: { Slug: 'x', Name: 'X', Status: 'running' },
      metrics: { cpu: 250, memPct: 10 },
    });
    const bars = Array.from(container.querySelectorAll('[style]'));
    const cpuBar = bars.find((el) => el.getAttribute('style').includes('width:'));
    expect(cpuBar.getAttribute('style')).toContain('width: 100%');
  });

  it('does not render metrics section when metrics is null', () => {
    const { queryByText } = render(AppCard, {
      app: { Slug: 'x', Name: 'X', Status: 'stopped' },
    });
    expect(queryByText('CPU')).toBeNull();
  });
});

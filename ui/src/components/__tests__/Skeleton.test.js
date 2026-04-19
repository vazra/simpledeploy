import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import Skeleton from '../Skeleton.svelte';

describe('Skeleton', () => {
  it('renders a card skeleton by default', () => {
    const { container } = render(Skeleton);
    const shimmer = container.querySelectorAll('.animate-shimmer');
    expect(shimmer.length).toBeGreaterThanOrEqual(3);
  });

  it('renders N cards when count is given', () => {
    const { container } = render(Skeleton, { type: 'card', count: 3 });
    // each card contains 3 shimmer bars
    expect(container.querySelectorAll('.animate-shimmer').length).toBe(9);
  });

  it('renders a chart skeleton', () => {
    const { container } = render(Skeleton, { type: 'chart' });
    expect(container.querySelector('.h-44')).not.toBeNull();
  });

  it('renders a single line skeleton', () => {
    const { container } = render(Skeleton, { type: 'line' });
    const el = container.querySelector('.animate-shimmer');
    expect(el).not.toBeNull();
    expect(el.className).toMatch(/h-4/);
  });

  it('renders a table-row skeleton', () => {
    const { container } = render(Skeleton, { type: 'table-row' });
    expect(container.querySelectorAll('.animate-shimmer').length).toBe(3);
  });
});

import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import StatCard from '../StatCard.svelte';

describe('StatCard', () => {
  it('renders as a plain div by default (no onclick)', () => {
    const { container, getByText } = render(StatCard, { label: 'CPU', value: '42%' });
    expect(getByText('CPU')).toBeInTheDocument();
    expect(getByText('42%')).toBeInTheDocument();
    expect(container.querySelector('button')).toBeNull();
  });

  it('renders as a button when onclick is provided and fires it', async () => {
    const onclick = vi.fn();
    const { getByRole } = render(StatCard, { label: 'Mem', value: '2 GB', onclick });
    const btn = getByRole('button');
    await fireEvent.click(btn);
    expect(onclick).toHaveBeenCalledTimes(1);
  });

  it('renders the sub line when provided', () => {
    const { getByText } = render(StatCard, { label: 'L', value: 'V', sub: 'subtext' });
    expect(getByText('subtext')).toBeInTheDocument();
  });

  it('applies color override class on the value', () => {
    const { getByText } = render(StatCard, { label: 'L', value: 'V', color: 'text-red-500' });
    expect(getByText('V').className).toMatch(/text-red-500/);
  });

  it('renders raw html in icon slot (iconHtml escape hatch)', () => {
    const { container } = render(StatCard, { label: 'L', value: 'V', icon: '<svg data-testid="icon"></svg>' });
    expect(container.querySelector('[data-testid="icon"]')).not.toBeNull();
  });
});

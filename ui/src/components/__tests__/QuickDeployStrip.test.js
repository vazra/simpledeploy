import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import QuickDeployStrip from '../QuickDeployStrip.svelte';

const templates = [
  { id: 'a', name: 'Alpha', icon: 'A', description: 'a-desc' },
  { id: 'b', name: 'Beta', icon: 'B', description: 'b-desc' },
  { id: 'c', name: 'Gamma', icon: 'C', description: 'c-desc' },
  { id: 'd', name: 'Delta', icon: 'D', description: 'd-desc' },
  { id: 'e', name: 'Epsilon', icon: 'E', description: 'e-desc' },
  { id: 'f', name: 'Zeta', icon: 'F', description: 'f-desc' },
  { id: 'g', name: 'Eta', icon: 'G', description: 'g-desc' },
];

describe('QuickDeployStrip', () => {
  it('renders nothing when no featuredIds match templates', () => {
    const { container } = render(QuickDeployStrip, {
      templates,
      featuredIds: ['nonexistent'],
    });
    expect(container.textContent.trim()).toBe('');
  });

  it('renders nothing when featuredIds is empty', () => {
    const { container } = render(QuickDeployStrip, {
      templates,
      featuredIds: [],
    });
    expect(container.textContent.trim()).toBe('');
  });

  it('renders matching featured templates in order', () => {
    const { getByText } = render(QuickDeployStrip, {
      templates,
      featuredIds: ['b', 'c'],
    });
    expect(getByText('Beta')).toBeInTheDocument();
    expect(getByText('Gamma')).toBeInTheDocument();
  });

  it('skips unknown ids silently', () => {
    const { getByText, queryByText } = render(QuickDeployStrip, {
      templates,
      featuredIds: ['a', 'nope', 'b'],
    });
    expect(getByText('Alpha')).toBeInTheDocument();
    expect(getByText('Beta')).toBeInTheDocument();
    expect(queryByText('nope')).toBeNull();
  });

  it('caps the list at 6 entries', () => {
    const { getAllByRole } = render(QuickDeployStrip, {
      templates,
      featuredIds: ['a', 'b', 'c', 'd', 'e', 'f', 'g'],
    });
    expect(getAllByRole('button')).toHaveLength(6);
  });

  it('fires onselect with the template id', async () => {
    const onselect = vi.fn();
    const { getByText } = render(QuickDeployStrip, {
      templates,
      featuredIds: ['a'],
      onselect,
    });
    await fireEvent.click(getByText('Alpha'));
    expect(onselect).toHaveBeenCalledWith('a');
  });
});

import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';

vi.mock('../../lib/api.js', async () => {
  const { makeApiMock } = await import('../../test-mocks/api.js');
  return { api: makeApiMock() };
});

import TemplatePicker from '../TemplatePicker.svelte';

const templates = [
  {
    id: 'alpha',
    name: 'Alpha',
    icon: 'A',
    category: 'web',
    description: 'alpha desc',
    nameSuggestion: 'alpha',
    variables: [],
    compose: { services: {} },
  },
  {
    id: 'beta',
    name: 'Beta',
    icon: 'B',
    category: 'db',
    description: 'beta desc',
    nameSuggestion: 'beta',
    variables: [],
    compose: { services: {} },
  },
];
const categories = [
  { id: 'web', label: 'Web' },
  { id: 'db', label: 'Databases' },
];

describe('TemplatePicker (grid)', () => {
  it('lists all templates by default', () => {
    const { getByText } = render(TemplatePicker, { templates, categories });
    expect(getByText('Alpha')).toBeInTheDocument();
    expect(getByText('Beta')).toBeInTheDocument();
  });

  it('filters by search term', async () => {
    const { getByText, queryByText, getByPlaceholderText } = render(TemplatePicker, { templates, categories });
    const search = getByPlaceholderText('Search templates...');
    await fireEvent.input(search, { target: { value: 'beta' } });
    expect(getByText('Beta')).toBeInTheDocument();
    expect(queryByText('Alpha')).toBeNull();
  });

  it('filters by category chip', async () => {
    const { getByRole, getByText, queryByText } = render(TemplatePicker, { templates, categories });
    const chip = getByRole('button', { name: 'Databases' });
    await fireEvent.click(chip);
    expect(getByText('Beta')).toBeInTheDocument();
    expect(queryByText('Alpha')).toBeNull();
  });

  it('shows "No templates match" when filters exclude all', async () => {
    const { getByText, getByPlaceholderText } = render(TemplatePicker, { templates, categories });
    const search = getByPlaceholderText('Search templates...');
    await fireEvent.input(search, { target: { value: 'nothing-here' } });
    expect(getByText('No templates match.')).toBeInTheDocument();
  });

  it('fires onblank from the start-blank button', async () => {
    const onblank = vi.fn();
    const { getByText } = render(TemplatePicker, { templates, categories, onblank });
    await fireEvent.click(getByText(/Start with a blank/));
    expect(onblank).toHaveBeenCalled();
  });

  it('shows the "Browse community recipes" button in grid view', () => {
    const { getByRole } = render(TemplatePicker, { templates, categories });
    expect(getByRole('button', { name: /browse community recipes/i })).toBeInTheDocument();
  });
});

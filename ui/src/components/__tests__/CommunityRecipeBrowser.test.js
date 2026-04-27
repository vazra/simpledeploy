import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';

vi.mock('../../lib/api.js', () => ({
  api: {
    listCommunityRecipes: vi.fn(async () => ({
      data: {
        schema_version: 1,
        recipes: [
          { id: 'nginx-static', name: 'Nginx', icon: '🌐', category: 'web', description: 'Static site' },
          { id: 'redis-cache', name: 'Redis', icon: '🔴', category: 'databases', description: 'Cache' },
        ],
      },
      error: null,
    })),
    fetchCommunityRecipeFile: vi.fn(async (id, file) => ({
      data: file === 'compose'
        ? 'services:\n  web:\n    image: nginx\n'
        : '# README for ' + id,
      error: null,
    })),
  },
}));

import CommunityRecipeBrowser from '../CommunityRecipeBrowser.svelte';

describe('CommunityRecipeBrowser', () => {
  it('loads and renders recipes when opened', async () => {
    render(CommunityRecipeBrowser, { props: { open: true } });
    expect(await screen.findByText('Nginx')).toBeTruthy();
    expect(screen.getByText('Redis')).toBeTruthy();
  });

  it('filters by search', async () => {
    render(CommunityRecipeBrowser, { props: { open: true } });
    await screen.findByText('Nginx');
    const input = screen.getByPlaceholderText(/search/i);
    await fireEvent.input(input, { target: { value: 'redis' } });
    await waitFor(() => {
      expect(screen.queryByText('Nginx')).toBeFalsy();
      expect(screen.getByText('Redis')).toBeTruthy();
    });
  });

  it('calls onselect with compose when Use Recipe clicked', async () => {
    const onselect = vi.fn();
    render(CommunityRecipeBrowser, { props: { open: true, onselect } });
    await screen.findByText('Nginx');
    await fireEvent.click(screen.getByText('Nginx'));
    const useBtn = await screen.findByRole('button', { name: /use recipe/i });
    await fireEvent.click(useBtn);
    await waitFor(() => {
      expect(onselect).toHaveBeenCalledWith(expect.objectContaining({
        id: 'nginx-static',
        compose: expect.stringContaining('services:'),
      }));
    });
  });
});

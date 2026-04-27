import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { api } from '../api.js';

function jsonResponse(body, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    headers: new Headers({ 'content-type': 'application/json' }),
    json: async () => body,
    text: async () => JSON.stringify(body),
  };
}

function textResponse(body, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    headers: new Headers({ 'content-type': 'text/plain' }),
    json: async () => null,
    text: async () => body,
  };
}

describe('community recipes api methods', () => {
  let fetchMock;

  beforeEach(() => {
    fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);
    window.location.hash = '';
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('lists community recipes', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({ schema_version: 1, recipes: [{ id: 'a', name: 'A' }] }));
    const { data, error } = await api.listCommunityRecipes();
    expect(error).toBeNull();
    expect(data.recipes[0].id).toBe('a');
    expect(fetchMock).toHaveBeenCalledWith('/api/recipes/community', expect.objectContaining({ method: 'GET' }));
  });

  it('fetches recipe file as text', async () => {
    fetchMock.mockResolvedValueOnce(textResponse('services:\n  web:\n    image: nginx\n'));
    const { data, error } = await api.fetchCommunityRecipeFile('nginx-static', 'compose');
    expect(error).toBeNull();
    expect(data).toContain('services:');
    const [url] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/recipes/community/file?id=nginx-static&file=compose');
  });

  it('encodes id and file params', async () => {
    fetchMock.mockResolvedValueOnce(textResponse('# README'));
    await api.fetchCommunityRecipeFile('foo bar', 'readme');
    const [url] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/recipes/community/file?id=foo%20bar&file=readme');
  });
});

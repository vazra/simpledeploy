import { describe, it, expect, vi } from 'vitest';
import { render } from '@testing-library/svelte';

vi.mock('../../lib/api.js', async () => {
  const { makeApiMock } = await import('../../test-mocks/api.js');
  return { api: makeApiMock() };
});

import BackupWizard from '../BackupWizard.svelte';

describe('BackupWizard', () => {
  it('is hidden when closed', () => {
    const { queryByRole } = render(BackupWizard, { open: false, slug: 'foo' });
    expect(queryByRole('dialog')).toBeNull();
  });

  it('renders step controls when open', () => {
    const { container } = render(BackupWizard, { open: true, slug: 'foo' });
    expect(container.textContent.length).toBeGreaterThan(0);
  });
});

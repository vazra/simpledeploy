import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';

vi.mock('../../lib/api.js', async () => {
  const { makeApiMock } = await import('../../test-mocks/api.js');
  return { api: makeApiMock() };
});

import DeployWizard from '../DeployWizard.svelte';

describe('DeployWizard', () => {
  it('renders nothing when closed', () => {
    const { queryByRole } = render(DeployWizard, { open: false });
    expect(queryByRole('dialog')).toBeNull();
  });

  it('renders the step chooser when open', () => {
    const { container } = render(DeployWizard, { open: true });
    expect(container.textContent).toMatch(/Template|template|Blank|blank|Start/i);
  });

  it('calls onclose when the modal close fires', async () => {
    const onclose = vi.fn();
    const { getAllByLabelText } = render(DeployWizard, { open: true, onclose });
    const closers = getAllByLabelText(/Close/i);
    if (closers.length) {
      await fireEvent.click(closers[0]);
      expect(onclose).toHaveBeenCalled();
    }
  });
});

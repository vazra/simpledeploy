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

  // docker compose rejects uppercase and dots in project names, so the
  // wizard must flag them before deploy.
  it('shows an inline error for app names compose cannot use', async () => {
    const { getByText, getByPlaceholderText, queryByText } = render(DeployWizard, { open: true });
    await fireEvent.click(getByText('Build it yourself'));
    const input = getByPlaceholderText('my-app');

    await fireEvent.input(input, { target: { value: 'MyApp' } });
    expect(getByText('Use lowercase letters only, for example "myapp"')).toBeInTheDocument();

    await fireEvent.input(input, { target: { value: 'my.app' } });
    expect(getByText(/Dots and spaces are not allowed/)).toBeInTheDocument();

    await fireEvent.input(input, { target: { value: 'my-app' } });
    expect(queryByText(/Dots and spaces are not allowed/)).toBeNull();
    expect(getByText(/Lowercase letters, numbers, hyphens/)).toBeInTheDocument();
  });
});

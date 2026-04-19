import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import Button from '../Button.svelte';
import { slot } from './helpers.js';

describe('Button', () => {
  it('renders children and is enabled by default', () => {
    const { getByRole } = render(Button, { children: slot('Click me') });
    const btn = getByRole('button');
    expect(btn).toHaveTextContent('Click me');
    expect(btn).not.toBeDisabled();
    expect(btn.getAttribute('type')).toBe('button');
  });

  it('invokes onclick when clicked', async () => {
    const onclick = vi.fn();
    const { getByRole } = render(Button, { children: slot('x'), onclick });
    await fireEvent.click(getByRole('button'));
    expect(onclick).toHaveBeenCalledTimes(1);
  });

  it('is disabled when disabled=true', () => {
    const { getByRole } = render(Button, { children: slot('x'), disabled: true });
    expect(getByRole('button')).toBeDisabled();
  });

  it('is disabled while loading and shows the spinner svg', () => {
    const { getByRole, container } = render(Button, { children: slot('x'), loading: true });
    expect(getByRole('button')).toBeDisabled();
    expect(container.querySelector('svg')).not.toBeNull();
  });

  it('applies primary styles by default', () => {
    const { getByRole } = render(Button, { children: slot('x') });
    expect(getByRole('button').className).toMatch(/bg-btn-primary/);
  });

  it('applies danger variant styles', () => {
    const { getByRole } = render(Button, { variant: 'danger', children: slot('x') });
    expect(getByRole('button').className).toMatch(/bg-btn-danger/);
  });

  it('applies sm size class', () => {
    const { getByRole } = render(Button, { size: 'sm', children: slot('x') });
    expect(getByRole('button').className).toMatch(/text-xs/);
  });

  it('supports type=submit', () => {
    const { getByRole } = render(Button, { type: 'submit', children: slot('x') });
    expect(getByRole('button').getAttribute('type')).toBe('submit');
  });
});

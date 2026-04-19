import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import Badge from '../Badge.svelte';
import { slot } from './helpers.js';

describe('Badge', () => {
  it('renders slot content', () => {
    const { getByTestId } = render(Badge, {
      children: slot(() => `<span data-testid="slot">hello</span>`),
    });
    expect(getByTestId('slot')).toHaveTextContent('hello');
  });

  it('applies the default variant class when none is given', () => {
    const { container } = render(Badge, { children: slot('x') });
    expect(container.querySelector('span').className).toMatch(/text-text-secondary/);
  });

  it('applies the success variant class', () => {
    const { container } = render(Badge, { variant: 'success', children: slot('ok') });
    expect(container.querySelector('span').className).toMatch(/text-emerald-400/);
  });

  it('applies the danger variant class', () => {
    const { container } = render(Badge, { variant: 'danger', children: slot('bad') });
    expect(container.querySelector('span').className).toMatch(/text-red-400/);
  });

  it('applies warning and info variant classes', () => {
    const w = render(Badge, { variant: 'warning', children: slot('w') });
    expect(w.container.querySelector('span').className).toMatch(/text-amber-400/);
    const i = render(Badge, { variant: 'info', children: slot('i') });
    expect(i.container.querySelector('span').className).toMatch(/text-blue-400/);
  });
});

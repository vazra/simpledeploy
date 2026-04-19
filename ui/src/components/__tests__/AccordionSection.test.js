import { describe, it, expect } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import AccordionSection from '../AccordionSection.svelte';
import { slot } from './helpers.js';

describe('AccordionSection', () => {
  it('renders title and is collapsed by default', () => {
    const { getByText, queryByText } = render(AccordionSection, {
      title: 'Advanced',
      children: slot(() => `<span>secret body</span>`),
    });
    expect(getByText('Advanced')).toBeInTheDocument();
    expect(queryByText('secret body')).toBeNull();
  });

  it('starts expanded when expanded=true', () => {
    const { getByText } = render(AccordionSection, {
      title: 'Open',
      expanded: true,
      children: slot(() => `<span>hello body</span>`),
    });
    expect(getByText('hello body')).toBeInTheDocument();
  });

  it('toggles on header click', async () => {
    const { getByText, queryByText } = render(AccordionSection, {
      title: 'Details',
      children: slot(() => `<span>body</span>`),
    });
    expect(queryByText('body')).toBeNull();
    await fireEvent.click(getByText('Details'));
    expect(queryByText('body')).not.toBeNull();
    await fireEvent.click(getByText('Details'));
    expect(queryByText('body')).toBeNull();
  });
});

import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import YamlEditor from '../YamlEditor.svelte';

describe('YamlEditor', () => {
  it('renders the initial value in the textarea', () => {
    const { container } = render(YamlEditor, { value: 'services: {}' });
    const textarea = container.querySelector('textarea');
    expect(textarea.value).toBe('services: {}');
  });

  it('emits onchange on input', async () => {
    const onchange = vi.fn();
    const { container } = render(YamlEditor, { value: '', onchange });
    const textarea = container.querySelector('textarea');
    await fireEvent.input(textarea, { target: { value: 'a: 1' } });
    expect(onchange).toHaveBeenCalledWith('a: 1');
  });

  it('renders a line number per line', () => {
    const { container } = render(YamlEditor, { value: 'a\nb\nc' });
    const gutter = container.querySelector('[aria-hidden="true"]');
    expect(gutter.textContent.replace(/\s+/g, '')).toBe('123');
  });

  it('renders an error banner when error prop provided', () => {
    const { getByText } = render(YamlEditor, { value: '', error: 'bad yaml' });
    expect(getByText('bad yaml')).toBeInTheDocument();
  });

  it('inserts two spaces on Tab and emits updated value', async () => {
    const onchange = vi.fn();
    const { container } = render(YamlEditor, { value: 'abc', onchange });
    const textarea = container.querySelector('textarea');
    textarea.setSelectionRange(0, 0);
    await fireEvent.keyDown(textarea, { key: 'Tab' });
    expect(onchange).toHaveBeenCalledWith('  abc');
  });

  it('omits the border wrapper when bordered=false', () => {
    const { container } = render(YamlEditor, { value: 'x', bordered: false });
    const outer = container.firstChild;
    expect(outer.className).not.toMatch(/border/);
  });
});

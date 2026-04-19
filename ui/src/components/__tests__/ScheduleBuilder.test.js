import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import { tick } from 'svelte';
import ScheduleBuilder from '../ScheduleBuilder.svelte';

describe('ScheduleBuilder', () => {
  it('defaults to daily 02:00 and emits matching cron', async () => {
    const onchange = vi.fn();
    render(ScheduleBuilder, { onchange });
    await tick();
    expect(onchange).toHaveBeenCalled();
    const last = onchange.mock.calls.at(-1)[0];
    expect(last).toBe('0 2 * * *');
  });

  it('parses and displays a weekly cron', async () => {
    const onchange = vi.fn();
    const { getByText } = render(ScheduleBuilder, { value: '30 9 * * 1,3,5', onchange });
    await tick();
    expect(onchange.mock.calls.at(-1)[0]).toBe('30 9 * * 1,3,5');
    expect(getByText(/Every Mon, Wed, Fri at 09:30/)).toBeInTheDocument();
  });

  it('parses and displays a monthly cron', async () => {
    const onchange = vi.fn();
    const { getByText } = render(ScheduleBuilder, { value: '0 0 15 * *', onchange });
    await tick();
    expect(getByText(/On day 15 of every month at 00:00/)).toBeInTheDocument();
  });

  it('falls back to custom for unparseable cron', async () => {
    const onchange = vi.fn();
    const { getByDisplayValue } = render(ScheduleBuilder, { value: '*/5 * * * *', onchange });
    await tick();
    expect(getByDisplayValue('*/5 * * * *')).toBeInTheDocument();
  });

  it('switching to weekly without days still emits a valid cron (falls back to * DOW)', async () => {
    const onchange = vi.fn();
    const { getByText } = render(ScheduleBuilder, { value: '0 2 * * *', onchange });
    await tick();
    onchange.mockClear();
    await fireEvent.click(getByText('weekly'));
    await tick();
    const last = onchange.mock.calls.at(-1)[0];
    expect(last).toMatch(/^\d+ \d+ \* \* (\*|[0-6](,[0-6])*)$/);
  });
});

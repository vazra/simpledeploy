import { describe, it, expect, vi } from 'vitest';
import { render } from '@testing-library/svelte';

// chart.js needs a real canvas context; stub it entirely.
const chartInstances = vi.hoisted(() => []);
vi.mock('chart.js', () => {
  const Chart = vi.fn(function (canvas, config) {
    this.canvas = canvas;
    this.config = config;
    this.data = config.data;
    this.options = config.options;
    this.destroy = vi.fn();
    this.update = vi.fn();
    this.getDatasetMeta = () => ({ data: [] });
    this.scales = { y: { top: 0, bottom: 100 } };
    chartInstances.push(this);
  });
  Chart.register = vi.fn();
  return { Chart, registerables: [] };
});

vi.mock('chartjs-adapter-date-fns', () => ({}));

import MetricsChart from '../MetricsChart.svelte';

describe('MetricsChart', () => {
  it('renders the label and subtitle', () => {
    const { getByText } = render(MetricsChart, {
      data: [{ x: 0, y: 1 }],
      label: 'CPU',
      subtitle: 'last 1h',
      color: '#123',
    });
    expect(getByText('CPU')).toBeInTheDocument();
    expect(getByText('last 1h')).toBeInTheDocument();
  });

  it('creates a Chart on mount when data provided', async () => {
    render(MetricsChart, {
      data: [{ x: 0, y: 1 }, { x: 1, y: 2 }],
      label: 'CPU',
      color: '#123',
    });
    await new Promise((r) => setTimeout(r, 0));
    expect(chartInstances.length).toBeGreaterThan(0);
  });
});

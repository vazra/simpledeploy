import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import SparklineChart from '../SparklineChart.svelte';

describe('SparklineChart', () => {
  it('renders nothing for empty data', () => {
    const { container } = render(SparklineChart, { data: [] });
    expect(container.querySelector('svg')).toBeNull();
  });

  it('renders nothing when only one point is present', () => {
    const { container } = render(SparklineChart, { data: [5] });
    expect(container.querySelector('svg')).toBeNull();
  });

  it('renders nothing when all points are null', () => {
    const { container } = render(SparklineChart, { data: [null, null, null] });
    expect(container.querySelector('svg')).toBeNull();
  });

  it('renders an svg for valid data', () => {
    const { container } = render(SparklineChart, { data: [1, 2, 3, 4], color: '#ff0000' });
    const svg = container.querySelector('svg');
    expect(svg).not.toBeNull();
    const polyline = svg.querySelector('polyline');
    expect(polyline.getAttribute('stroke')).toBe('#ff0000');
  });

  it('uses the width/height props on the svg element', () => {
    const { container } = render(SparklineChart, { data: [1, 2], width: 200, height: 50 });
    const svg = container.querySelector('svg');
    expect(svg.getAttribute('width')).toBe('200');
    expect(svg.getAttribute('height')).toBe('50');
  });

  it('emits a polygon fill path for the area under the line', () => {
    const { container } = render(SparklineChart, { data: [1, 2, 3] });
    expect(container.querySelector('polygon')).not.toBeNull();
  });
});

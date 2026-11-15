import { describe, expect, it } from 'vitest';
import type { ChartColors } from './colors.js';
import { plotOptions, type View } from './uplot.js';

const palette = (tag: string): ChartColors => ({
  line: `${tag}-line`,
  band: `${tag}-band`,
  warn: `${tag}-warn`,
  grid: `${tag}-grid`,
  axis: `${tag}-axis`,
  select: `${tag}-select`,
  surface: `${tag}-surface`,
  font: 'sans-serif'
});

const viewWith = (variant: 'full' | 'spark'): View => ({
  lib: {} as View['lib'],
  params: { points: [], low: null, high: null, variant },
  colors: palette('light'),
  xs: [],
  vs: []
});

const call = (fn: unknown) => (fn as () => string)();

describe('plotOptions', () => {
  it('reads the colors when a callback runs, so a theme switch reaches the plot', () => {
    const view = viewWith('full');
    const options = plotOptions(view, { clientWidth: 300 } as HTMLElement, () => {});
    const axis = options.axes?.[0] as {
      stroke: unknown;
      grid: { stroke: unknown };
      ticks: { stroke: unknown };
    };
    const cursor = options.cursor as { points: { stroke: unknown; fill: unknown } };
    const series = options.series?.[1] as {
      stroke: unknown;
      points: { fill: unknown; stroke: unknown };
    };
    const read = () => [
      call(axis.stroke),
      call(axis.grid.stroke),
      call(axis.ticks.stroke),
      call(cursor.points.stroke),
      call(cursor.points.fill),
      call(series.stroke),
      call(series.points.fill),
      call(series.points.stroke)
    ];
    expect(read().every((c) => c.startsWith('light-'))).toBe(true);
    view.colors = palette('dark');
    expect(read().every((c) => c.startsWith('dark-'))).toBe(true);
  });

  it('colors the sparkline line from the current theme too', () => {
    const view = viewWith('spark');
    const options = plotOptions(view, { clientWidth: 0 } as HTMLElement, () => {});
    const series = options.series?.[1] as { stroke: unknown };
    view.colors = palette('dark');
    expect(call(series.stroke)).toBe('dark-line');
  });
});

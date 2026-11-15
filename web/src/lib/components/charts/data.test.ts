import { describe, expect, it } from 'vitest';
import type { SeriesPoint } from '#lib/api/charts.js';
import {
  bandSpan,
  flaggedPoints,
  lowerBound,
  outOfRange,
  sparkPoints,
  toChartPoints,
  xRange,
  yRange
} from './data.js';

const point = (id: number, at: string, value: number, note = ''): SeriesPoint => ({
  result_id: id,
  measured_at: at,
  value,
  in_range: true,
  note
});

describe('outOfRange', () => {
  it('judges a two-sided range, inclusive at both limits', () => {
    expect(outOfRange(7.9, 8, 8.5)).toBe(true);
    expect(outOfRange(8.6, 8, 8.5)).toBe(true);
    expect(outOfRange(8, 8, 8.5)).toBe(false);
    expect(outOfRange(8.5, 8, 8.5)).toBe(false);
  });

  it('judges one-sided ranges and treats zero as a real limit', () => {
    expect(outOfRange(0.5, 1, null)).toBe(true);
    expect(outOfRange(1000, 1, null)).toBe(false);
    expect(outOfRange(5, null, 3)).toBe(true);
    expect(outOfRange(-5, null, 3)).toBe(false);
    expect(outOfRange(-0.1, 0, null)).toBe(true);
    expect(outOfRange(0.1, null, 0)).toBe(true);
  });

  it('never flags anything without a range', () => {
    expect(outOfRange(123, null, null)).toBe(false);
  });
});

describe('toChartPoints', () => {
  it('converts RFC 3339 timestamps to unix seconds and carries each reading note', () => {
    const out = toChartPoints([
      point(1, '2026-03-01T12:00:00Z', 8.1),
      point(2, '2026-03-02T12:00:30Z', 8.4, 'after water change')
    ]);
    expect(out).toEqual([
      { t: Date.UTC(2026, 2, 1, 12) / 1000, v: 8.1, note: '' },
      { t: Date.UTC(2026, 2, 2, 12, 0, 30) / 1000, v: 8.4, note: 'after water change' }
    ]);
  });

  it('drops readings whose timestamp cannot be parsed', () => {
    const out = toChartPoints([point(1, 'not a date', 1), point(2, '2026-03-01T00:00:00Z', 2)]);
    expect(out.map((p) => p.v)).toEqual([2]);
  });
});

describe('sparkPoints', () => {
  it('returns [seconds, value] pairs in order', () => {
    expect(
      sparkPoints([point(1, '1970-01-01T00:00:10Z', 3), point(2, '1970-01-01T00:00:20Z', 4)])
    ).toEqual([
      [10, 3],
      [20, 4]
    ]);
  });
});

describe('flaggedPoints', () => {
  it('lists out-of-range readings newest first with their index', () => {
    const points = [
      { t: 1, v: 9, note: 'high' },
      { t: 2, v: 8.2, note: '' },
      { t: 3, v: 7, note: 'low' }
    ];
    expect(flaggedPoints(points, 8, 8.5).map((f) => f.index)).toEqual([2, 0]);
    expect(flaggedPoints(points, 8, 8.5)[0].point.note).toBe('low');
    expect(flaggedPoints(points, null, null)).toEqual([]);
  });
});

describe('lowerBound', () => {
  it('returns the first index at or above the target', () => {
    const xs = [10, 20, 20, 30];
    expect(lowerBound(xs, 5)).toBe(0);
    expect(lowerBound(xs, 20)).toBe(1);
    expect(lowerBound(xs, 21)).toBe(3);
    expect(lowerBound(xs, 99)).toBe(4);
    expect(lowerBound([], 1)).toBe(0);
  });
});

describe('xRange', () => {
  it('pads both ends by two percent of the span', () => {
    expect(xRange(1000, 2000)).toEqual([980, 2020]);
  });

  it('gives a single reading half a day on each side and tolerates no data', () => {
    expect(xRange(5000, 5000)).toEqual([5000 - 43200, 5000 + 43200]);
    expect(xRange(null, null)).toEqual([0, 1]);
  });
});

describe('yRange', () => {
  it('pads the data by twelve percent of its span', () => {
    const [lo, hi] = yRange(10, 20, null, null);
    expect(lo).toBeCloseTo(8.8);
    expect(hi).toBeCloseTo(21.2);
  });

  it('stretches to include the ideal limits so the band stays on screen', () => {
    const [lo, hi] = yRange(8.1, 8.3, 7.8, 8.6);
    expect(lo).toBeLessThan(7.8);
    expect(hi).toBeGreaterThan(8.6);
  });

  it('keeps a one-sided range in view', () => {
    const [lo, hi] = yRange(8.1, 8.3, 7.8, null);
    expect(lo).toBeLessThan(7.8);
    expect(hi).toBeGreaterThan(8.3);
  });

  it('copes with flat data and with no data', () => {
    const [lo, hi] = yRange(8, 8, null, null);
    expect(lo).toBeLessThan(8);
    expect(hi).toBeGreaterThan(8);
    expect(yRange(null, null, null, null)).toEqual([0, 1]);
    expect(yRange(null, null, 7, 9)[0]).toBeLessThan(7);
  });
});

describe('bandSpan', () => {
  // A scale where value 0 is at pixel 100 and 10 is at pixel 0, inside a plot from 0 to 100.
  const toPos = (v: number) => 100 - v * 10;

  it('maps a two-sided range to the pixels between its limits', () => {
    expect(bandSpan(2, 6, toPos, 0, 100)).toEqual({ top: 40, bottom: 80 });
  });

  it('extends a missing limit to the plot edge', () => {
    expect(bandSpan(null, 6, toPos, 0, 100)).toEqual({ top: 40, bottom: 100 });
    expect(bandSpan(2, null, toPos, 0, 100)).toEqual({ top: 0, bottom: 80 });
  });

  it('clamps limits that fall outside the plot', () => {
    expect(bandSpan(-5, 50, toPos, 0, 100)).toEqual({ top: 0, bottom: 100 });
  });

  it('draws nothing without a range', () => {
    expect(bandSpan(null, null, toPos, 0, 100)).toBeNull();
  });
});

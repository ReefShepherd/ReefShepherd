import { describe, expect, it } from 'vitest';
import type { SeriesPoint } from '#lib/api/charts.js';
import { bandSpan, flaggedPoints, lowerBound, toChartPoints, xRange, yRange } from './data.js';

const point = (id: number, at: string, value: number, note = '', inRange = true): SeriesPoint => ({
  result_id: id,
  measured_at: at,
  value,
  in_range: inRange,
  note
});

describe('toChartPoints', () => {
  it('converts RFC 3339 timestamps to unix seconds and carries each reading note', () => {
    const out = toChartPoints([
      point(1, '2026-03-01T12:00:00Z', 8.1),
      point(2, '2026-03-02T12:00:30Z', 8.4, 'after water change', false)
    ]);
    expect(out).toEqual([
      { t: Date.UTC(2026, 2, 1, 12) / 1000, v: 8.1, note: '', inRange: true },
      {
        t: Date.UTC(2026, 2, 2, 12, 0, 30) / 1000,
        v: 8.4,
        note: 'after water change',
        inRange: false
      }
    ]);
  });

  it('drops readings whose timestamp cannot be parsed', () => {
    const out = toChartPoints([point(1, 'not a date', 1), point(2, '2026-03-01T00:00:00Z', 2)]);
    expect(out.map((p) => p.v)).toEqual([2]);
  });
});

describe('flaggedPoints', () => {
  it('lists the readings the server marked out of range, newest first, with their index', () => {
    const points = [
      { t: 1, v: 9, note: 'high', inRange: false },
      { t: 2, v: 8.2, note: '', inRange: true },
      { t: 3, v: 7, note: 'low', inRange: false }
    ];
    expect(flaggedPoints(points).map((f) => f.index)).toEqual([2, 0]);
    expect(flaggedPoints(points)[0].point.note).toBe('low');
    expect(flaggedPoints(points.map((p) => ({ ...p, inRange: true })))).toEqual([]);
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

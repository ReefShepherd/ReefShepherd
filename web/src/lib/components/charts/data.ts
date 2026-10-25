import type { SeriesPoint } from '#lib/api/charts.js';

/** One plotted reading; t is unix seconds, the unit uPlot uses for time scales. */
export interface ChartPoint {
  t: number;
  v: number;
  note: string;
}

/** True when the value lies outside the ideal range; a null side has no limit. */
export function outOfRange(v: number, low: number | null, high: number | null): boolean {
  return (low !== null && v < low) || (high !== null && v > high);
}

/** Converts series readings to plot points, dropping unreadable timestamps. */
export function toChartPoints(points: readonly SeriesPoint[]): ChartPoint[] {
  const out: ChartPoint[] = [];
  for (const p of points) {
    const ms = Date.parse(p.measured_at);
    if (Number.isNaN(ms)) continue;
    out.push({ t: ms / 1000, v: p.value, note: p.note });
  }
  return out;
}

/** The out-of-range readings with their index in points, newest first. */
export function flaggedPoints(
  points: readonly ChartPoint[],
  low: number | null,
  high: number | null
): { index: number; point: ChartPoint }[] {
  const out: { index: number; point: ChartPoint }[] = [];
  for (let index = points.length - 1; index >= 0; index--) {
    if (outOfRange(points[index].v, low, high)) out.push({ index, point: points[index] });
  }
  return out;
}

/** The first index whose value is at least target, or the length when there is none. */
export function lowerBound(sorted: readonly number[], target: number): number {
  let lo = 0;
  let hi = sorted.length;
  while (lo < hi) {
    const mid = (lo + hi) >> 1;
    if (sorted[mid] < target) lo = mid + 1;
    else hi = mid;
  }
  return lo;
}

const halfDay = 43200;

/** The x range with a little room at both ends so the first and last points are not cut off. */
export function xRange(min: number | null, max: number | null): [number, number] {
  if (min === null || max === null) return [0, 1];
  if (min === max) return [min - halfDay, max + halfDay];
  const pad = (max - min) * 0.02;
  return [min - pad, max + pad];
}

/** The y range fitting the visible readings and both ideal limits, so the band stays in view; min and max are null without data. */
export function yRange(
  min: number | null,
  max: number | null,
  low: number | null,
  high: number | null
): [number, number] {
  let lo = min ?? Infinity;
  let hi = max ?? -Infinity;
  if (low !== null) lo = Math.min(lo, low);
  if (high !== null) hi = Math.max(hi, high);
  if (!Number.isFinite(lo) || !Number.isFinite(hi)) return [0, 1];
  const pad = (hi - lo) * 0.12 || Math.abs(hi) * 0.05 || 1;
  return [lo - pad, hi + pad];
}

/** The vertical extent of the ideal band in canvas pixels, clamped to the plot; a missing limit extends to the plot edge. */
export function bandSpan(
  low: number | null,
  high: number | null,
  toPos: (value: number) => number,
  plotTop: number,
  plotBottom: number
): { top: number; bottom: number } | null {
  if (low === null && high === null) return null;
  const clamp = (y: number) => Math.min(plotBottom, Math.max(plotTop, y));
  return {
    top: high === null ? plotTop : clamp(toPos(high)),
    bottom: low === null ? plotBottom : clamp(toPos(low))
  };
}

/** [unix seconds, value] pairs for a Sparkline, skipping readings whose timestamp cannot be read. */
export function sparkPoints(points: readonly SeriesPoint[]): [number, number][] {
  return toChartPoints(points).map((p) => [p.t, p.v]);
}

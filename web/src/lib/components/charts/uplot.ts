import type uPlot from 'uplot';
import { bandSpan, lowerBound, xRange, yRange, type ChartPoint } from './data.js';
import {
  bandEdgeAlpha,
  bandFillAlpha,
  readColors,
  watchTheme,
  type ChartColors
} from './colors.js';

export interface ChartParams {
  points: ChartPoint[];
  low: number | null;
  high: number | null;
  /** 'full' has axes, a tooltip and tap-to-select; 'spark' is a bare 60 px line. */
  variant?: 'full' | 'spark';
  /** Index into points of the selected reading, drawn with a ring. */
  selected?: number | null;
  timeZone?: string;
  locale?: string;
  /** Decimals of the parameter, which set how many digits the value axis labels show. */
  decimals?: number;
  formatValue?: (value: number) => string;
  formatTime?: (unixSeconds: number) => string;
  /** Called with the tapped point's index, or null when the selected point is tapped again. */
  onselect?: (index: number | null) => void;
}

type UPlotLib = typeof uPlot;

const sparkHeight = 60;
const fullHeight = 280;
const dayInSeconds = 86400;

let libPromise: Promise<UPlotLib> | null = null;

// uPlot and its styles load on first use so pages that do not draw a chart never download them.
function loadLib(): Promise<UPlotLib> {
  libPromise ??= Promise.all([
    import('uplot'),
    import('uplot/dist/uPlot.min.css'),
    import('./chart.css')
  ])
    .then(([lib]) => lib.default)
    .catch((e: unknown) => {
      libPromise = null;
      throw e;
    });
  return libPromise;
}

/** What the drawing steps read; build keeps one and updates it in place. */
export interface View {
  lib: UPlotLib;
  params: ChartParams;
  colors: ChartColors;
  /** The points as the two columns uPlot takes. */
  xs: number[];
  vs: number[];
}

interface Tip {
  update(self: uPlot): void;
  reset(): void;
}

const isSpark = (view: View) => view.params.variant === 'spark';
const plotHeight = (view: View) => (isSpark(view) ? sparkHeight : fullHeight);

function loadPoints(view: View) {
  view.xs = view.params.points.map((p) => p.t);
  view.vs = view.params.points.map((p) => p.v);
}

function timeAxisLabels(view: View) {
  return (_u: uPlot, splits: number[], _axis: number, _space: number, incr: number) => {
    const options: Intl.DateTimeFormatOptions =
      incr < dayInSeconds
        ? { hour: '2-digit', minute: '2-digit' }
        : incr < 28 * dayInSeconds
          ? { day: 'numeric', month: 'short' }
          : incr < 360 * dayInSeconds
            ? { month: 'short', year: '2-digit' }
            : { year: 'numeric' };
    const format = new Intl.DateTimeFormat(view.params.locale, {
      timeZone: view.params.timeZone,
      ...options
    });
    return splits.map((s) => format.format(s * 1000));
  };
}

function valueAxisLabels(view: View) {
  return (_u: uPlot, splits: number[]) => {
    const format = new Intl.NumberFormat(view.params.locale, {
      maximumFractionDigits: Math.min((view.params.decimals ?? 1) + 1, 6)
    });
    return splits.map((s) => format.format(s));
  };
}

function drawBand(view: View, self: uPlot) {
  const { low, high } = view.params;
  const { lib, colors } = view;
  const { left, top, width, height } = self.bbox;
  const span = bandSpan(low, high, (v) => self.valToPos(v, 'y', true), top, top + height);
  if (!span) return;
  const { ctx } = self;
  ctx.save();
  ctx.fillStyle = colors.band;
  ctx.globalAlpha = bandFillAlpha;
  ctx.fillRect(left, span.top, width, span.bottom - span.top);
  if (!isSpark(view)) {
    ctx.globalAlpha = bandEdgeAlpha;
    ctx.strokeStyle = colors.band;
    ctx.lineWidth = lib.pxRatio;
    ctx.setLineDash([4 * lib.pxRatio, 4 * lib.pxRatio]);
    ctx.beginPath();
    for (const [limit, y] of [
      [high, span.top],
      [low, span.bottom]
    ] as const) {
      if (limit === null) continue;
      ctx.moveTo(left, y);
      ctx.lineTo(left + width, y);
    }
    ctx.stroke();
  }
  ctx.restore();
}

function circle(view: View, self: uPlot, index: number, radius: number) {
  const { ctx } = self;
  ctx.beginPath();
  ctx.arc(
    self.valToPos(view.xs[index], 'x', true),
    self.valToPos(view.vs[index], 'y', true),
    radius * view.lib.pxRatio,
    0,
    Math.PI * 2
  );
}

function dot(view: View, self: uPlot, index: number, radius: number, fill: string, ring?: string) {
  const { ctx } = self;
  circle(view, self, index, radius);
  ctx.fillStyle = fill;
  ctx.fill();
  if (ring) {
    ctx.strokeStyle = ring;
    ctx.lineWidth = 1.5 * view.lib.pxRatio;
    ctx.stroke();
  }
}

// uPlot colors points per series only, so out-of-range points are painted here one by one.
function drawFlagged(view: View, self: uPlot) {
  const { points, selected } = view.params;
  const { left, top, width, height } = self.bbox;
  const { ctx } = self;
  ctx.beginPath();
  ctx.rect(left, top, width, height);
  ctx.clip();
  const sx = self.scales.x;
  const first = sx.min != null ? Math.max(0, lowerBound(view.xs, sx.min) - 1) : 0;
  const last =
    sx.max != null ? Math.min(points.length - 1, lowerBound(view.xs, sx.max)) : points.length - 1;
  for (let i = first; i <= last; i++) {
    if (!points[i].inRange) dot(view, self, i, 4.5, view.colors.warn, view.colors.surface);
  }
  if (selected != null && selected >= 0 && selected < points.length) {
    circle(view, self, selected, 9);
    ctx.strokeStyle = view.colors.select;
    ctx.lineWidth = 2 * view.lib.pxRatio;
    ctx.stroke();
  }
}

function drawMarks(view: View, self: uPlot) {
  const { points } = view.params;
  if (points.length === 0) return;
  self.ctx.save();
  if (isSpark(view)) {
    const last = points.length - 1;
    dot(view, self, last, 3, points[last].inRange ? view.colors.line : view.colors.warn);
  } else {
    drawFlagged(view, self);
  }
  self.ctx.restore();
}

// The tooltip is built from elements, with textContent, so a user-entered note is never parsed as markup.
function createTip(view: View, over: HTMLElement): Tip {
  const el = document.createElement('div');
  el.className = 'chart-tip';
  const [value, time, note] = ['tip-value', 'tip-time', 'tip-note'].map((name) => {
    const part = document.createElement('div');
    part.className = name;
    el.appendChild(part);
    return part;
  });
  el.style.display = 'none';
  over.appendChild(el);
  let shown = -1;

  return {
    update(self) {
      const { points, formatValue, formatTime } = view.params;
      const idx = self.cursor.idx;
      if (idx == null || idx < 0 || idx >= points.length) {
        el.style.display = 'none';
        shown = -1;
        return;
      }
      const point = points[idx];
      if (idx !== shown) {
        shown = idx;
        value.textContent = formatValue ? formatValue(point.v) : String(point.v);
        value.classList.toggle('flagged', !point.inRange);
        time.textContent = formatTime ? formatTime(point.t) : String(point.t);
        note.textContent = point.note;
        note.style.display = point.note ? '' : 'none';
        el.style.display = 'block';
      }
      // Reading the width forces a layout, so it is only measured when the content changed.
      const left = self.cursor.left ?? 0;
      const tipWidth = el.offsetWidth;
      const x = left + 14 + tipWidth > self.over.clientWidth ? left - 14 - tipWidth : left + 14;
      el.style.transform = `translate(${Math.max(0, x)}px, 4px)`;
    },
    reset() {
      shown = -1;
      el.style.display = 'none';
    }
  };
}

function listenForTaps(view: View, over: HTMLElement, cursor: () => number | null | undefined) {
  let downX = 0;
  over.addEventListener('mousedown', (e) => (downX = e.clientX));
  // A press that moved is a drag-zoom, not a tap.
  over.addEventListener('click', (e) => {
    const idx = cursor();
    if (idx == null || Math.abs(e.clientX - downX) > 4) return;
    view.params.onselect?.(idx === view.params.selected ? null : idx);
  });
}

export function plotOptions(
  view: View,
  node: HTMLElement,
  onCursor: (self: uPlot) => void
): uPlot.Options {
  const { lib } = view;
  const spark = isSpark(view);
  // The color callbacks read view.colors when they run, so a theme switch reaches them.
  const axisBase = {
    font: `${spark ? 11 : 12}px ${view.colors.font}`,
    stroke: () => view.colors.axis,
    grid: { stroke: () => view.colors.grid, width: 1 },
    ticks: { stroke: () => view.colors.grid, width: 1, size: 4 },
    border: { show: false }
  };
  return {
    width: node.clientWidth || 300,
    height: plotHeight(view),
    pxAlign: 0,
    legend: { show: false },
    padding: spark ? [5, 4, 5, 4] : [10, 10, 0, 0],
    tzDate: view.params.timeZone
      ? (ts) => lib.tzDate(new Date(ts * 1000), view.params.timeZone as string)
      : undefined,
    cursor: spark
      ? { show: false }
      : {
          y: false,
          drag: { x: true, y: false, setScale: true },
          points: {
            size: 9,
            width: 2,
            stroke: () => view.colors.line,
            fill: () => view.colors.surface
          }
        },
    select: { show: !spark, left: 0, top: 0, width: 0, height: 0 },
    scales: {
      x: { time: true, range: (_self, min, max) => xRange(min, max) },
      y: { range: (_self, min, max) => yRange(min, max, view.params.low, view.params.high) }
    },
    axes: spark
      ? [{ show: false }, { show: false }]
      : [
          { ...axisBase, size: 28, space: 70, values: timeAxisLabels(view) },
          { ...axisBase, size: 48, space: 36, values: valueAxisLabels(view) }
        ],
    series: [
      {},
      {
        stroke: () => view.colors.line,
        width: spark ? 1.5 : 2,
        points: spark
          ? { show: false }
          : {
              size: 5,
              width: 0,
              fill: () => view.colors.line,
              stroke: () => view.colors.line
            }
      }
    ],
    hooks: {
      drawClear: [(self) => drawBand(view, self)],
      draw: [(self) => drawMarks(view, self)],
      setCursor: [onCursor]
    }
  };
}

function build(lib: UPlotLib, node: HTMLElement, getParams: () => ChartParams) {
  const view: View = { lib, params: getParams(), colors: readColors(node), xs: [], vs: [] };
  let plot: uPlot | null = null;
  let tip: Tip | null = null;
  let lastWidth = 0;

  function create() {
    view.params = getParams();
    loadPoints(view);
    const created = new lib(
      plotOptions(view, node, (self) => tip?.update(self)),
      [view.xs, view.vs],
      node
    );
    plot = created;
    lastWidth = node.clientWidth;
    if (isSpark(view)) return;
    tip = createTip(view, created.over);
    listenForTaps(view, created.over, () => created.cursor.idx);
  }

  function destroy() {
    plot?.destroy();
    plot = null;
    tip = null;
  }

  create();

  const resize = new ResizeObserver(() => {
    const width = node.clientWidth;
    if (plot && width && width !== lastWidth) {
      lastWidth = width;
      plot.setSize({ width, height: plotHeight(view) });
    }
  });
  resize.observe(node);

  const unwatch = watchTheme(() => {
    view.colors = readColors(node);
    plot?.redraw(false);
  });

  return {
    update() {
      const next = getParams();
      const prev = view.params;
      view.params = next;
      if (
        next.variant !== prev.variant ||
        next.timeZone !== prev.timeZone ||
        next.locale !== prev.locale ||
        next.decimals !== prev.decimals
      ) {
        destroy();
        create();
        return;
      }
      if (next.points !== prev.points || next.low !== prev.low || next.high !== prev.high) {
        loadPoints(view);
        tip?.reset();
        plot?.setData([view.xs, view.vs]);
      } else if (next.selected !== prev.selected) {
        plot?.redraw(false);
      }
    },
    destroy() {
      resize.disconnect();
      unwatch();
      destroy();
    }
  };
}

/** Svelte action that draws the readings with uPlot; pass plain arrays and replace them, never mutate them in place. */
export function chart(node: HTMLElement, initial: ChartParams) {
  let params = initial;
  let instance: ReturnType<typeof build> | null = null;
  let gone = false;

  loadLib()
    .then((lib) => {
      if (!gone) instance = build(lib, node, () => params);
    })
    .catch(() => {
      // The chunk could not be fetched (offline), so the container stays empty.
    });

  return {
    update(next: ChartParams) {
      params = next;
      instance?.update();
    },
    destroy() {
      gone = true;
      instance?.destroy();
      instance = null;
    }
  };
}

import type uPlot from 'uplot';
import { bandSpan, lowerBound, outOfRange, xRange, yRange, type ChartPoint } from './data.js';
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

function build(lib: UPlotLib, node: HTMLElement, getParams: () => ChartParams) {
  let colors: ChartColors = readColors(node);
  let xs: number[] = [];
  let vs: number[] = [];
  let tip: HTMLDivElement | null = null;
  let tipParts: HTMLDivElement[] = [];
  let tipIndex = -1;
  let lastWidth = 0;
  let current = getParams();
  let plot: uPlot | null = null;

  const isSpark = () => current.variant === 'spark';
  const height = () => (isSpark() ? sparkHeight : fullHeight);

  function prepare() {
    xs = current.points.map((p) => p.t);
    vs = current.points.map((p) => p.v);
  }

  function axisTimeLabels(
    _u: uPlot,
    splits: number[],
    _axis: number,
    _space: number,
    incr: number
  ) {
    const options: Intl.DateTimeFormatOptions =
      incr < dayInSeconds
        ? { hour: '2-digit', minute: '2-digit' }
        : incr < 28 * dayInSeconds
          ? { day: 'numeric', month: 'short' }
          : incr < 360 * dayInSeconds
            ? { month: 'short', year: '2-digit' }
            : { year: 'numeric' };
    const format = new Intl.DateTimeFormat(current.locale, {
      timeZone: current.timeZone,
      ...options
    });
    return splits.map((s) => format.format(s * 1000));
  }

  function axisValueLabels(_u: uPlot, splits: number[]) {
    const format = new Intl.NumberFormat(current.locale, {
      maximumFractionDigits: Math.min((current.decimals ?? 1) + 1, 6)
    });
    return splits.map((s) => format.format(s));
  }

  function drawBand(self: uPlot) {
    const { left, top, width, height: h } = self.bbox;
    const span = bandSpan(
      current.low,
      current.high,
      (v) => self.valToPos(v, 'y', true),
      top,
      top + h
    );
    if (!span) return;
    const { ctx } = self;
    ctx.save();
    ctx.fillStyle = colors.band;
    ctx.globalAlpha = bandFillAlpha;
    ctx.fillRect(left, span.top, width, span.bottom - span.top);
    if (!isSpark()) {
      ctx.globalAlpha = bandEdgeAlpha;
      ctx.strokeStyle = colors.band;
      ctx.lineWidth = lib.pxRatio;
      ctx.setLineDash([4 * lib.pxRatio, 4 * lib.pxRatio]);
      ctx.beginPath();
      for (const [limit, y] of [
        [current.high, span.top],
        [current.low, span.bottom]
      ] as const) {
        if (limit === null) continue;
        ctx.moveTo(left, y);
        ctx.lineTo(left + width, y);
      }
      ctx.stroke();
    }
    ctx.restore();
  }

  function dot(self: uPlot, index: number, radius: number, fill: string, ring?: string) {
    const { ctx } = self;
    ctx.beginPath();
    ctx.arc(
      self.valToPos(xs[index], 'x', true),
      self.valToPos(vs[index], 'y', true),
      radius * lib.pxRatio,
      0,
      Math.PI * 2
    );
    ctx.fillStyle = fill;
    ctx.fill();
    if (ring) {
      ctx.strokeStyle = ring;
      ctx.lineWidth = 1.5 * lib.pxRatio;
      ctx.stroke();
    }
  }

  // uPlot colors points per series only, so out-of-range points are painted here one by one.
  function drawMarks(self: uPlot) {
    const n = vs.length;
    if (n === 0) return;
    const { ctx } = self;
    ctx.save();
    if (isSpark()) {
      const last = n - 1;
      dot(
        self,
        last,
        3,
        outOfRange(vs[last], current.low, current.high) ? colors.warn : colors.line
      );
    } else {
      const { left, top, width, height: h } = self.bbox;
      ctx.beginPath();
      ctx.rect(left, top, width, h);
      ctx.clip();
      const sx = self.scales.x;
      const first = sx.min != null ? Math.max(0, lowerBound(xs, sx.min) - 1) : 0;
      const last = sx.max != null ? Math.min(n - 1, lowerBound(xs, sx.max)) : n - 1;
      for (let i = first; i <= last; i++) {
        if (outOfRange(vs[i], current.low, current.high))
          dot(self, i, 4.5, colors.warn, colors.surface);
      }
      const chosen = current.selected;
      if (chosen != null && chosen >= 0 && chosen < n) {
        ctx.beginPath();
        ctx.arc(
          self.valToPos(xs[chosen], 'x', true),
          self.valToPos(vs[chosen], 'y', true),
          9 * lib.pxRatio,
          0,
          Math.PI * 2
        );
        ctx.strokeStyle = colors.select;
        ctx.lineWidth = 2 * lib.pxRatio;
        ctx.stroke();
      }
    }
    ctx.restore();
  }

  function updateTip(self: uPlot) {
    if (!tip) return;
    const idx = self.cursor.idx;
    if (idx == null || idx < 0 || idx >= current.points.length) {
      tip.style.display = 'none';
      tipIndex = -1;
      return;
    }
    const point = current.points[idx];
    if (idx !== tipIndex) {
      tipIndex = idx;
      // textContent keeps user-entered notes from being parsed as markup.
      tipParts[0].textContent = current.formatValue
        ? current.formatValue(point.v)
        : String(point.v);
      tipParts[0].classList.toggle('flagged', outOfRange(point.v, current.low, current.high));
      tipParts[1].textContent = current.formatTime ? current.formatTime(point.t) : String(point.t);
      tipParts[2].textContent = point.note;
      tipParts[2].style.display = point.note ? '' : 'none';
      tip.style.display = 'block';
    }
    // Reading the width forces a layout, so it is only measured when the content changed.
    const left = self.cursor.left ?? 0;
    const overWidth = self.over.clientWidth;
    const tipWidth = tip.offsetWidth;
    const x = left + 14 + tipWidth > overWidth ? left - 14 - tipWidth : left + 14;
    tip.style.transform = `translate(${Math.max(0, x)}px, 4px)`;
  }

  function create() {
    current = getParams();
    prepare();
    const spark = isSpark();
    const font = `${spark ? 11 : 12}px ${colors.font}`;
    const axisBase = {
      font,
      stroke: () => colors.axis,
      grid: { stroke: () => colors.grid, width: 1 },
      ticks: { stroke: () => colors.grid, width: 1, size: 4 },
      border: { show: false }
    };
    const opts: uPlot.Options = {
      width: node.clientWidth || 300,
      height: height(),
      pxAlign: 0,
      legend: { show: false },
      padding: spark ? [5, 4, 5, 4] : [10, 10, 0, 0],
      tzDate: current.timeZone
        ? (ts) => lib.tzDate(new Date(ts * 1000), current.timeZone as string)
        : undefined,
      cursor: spark
        ? { show: false }
        : {
            y: false,
            drag: { x: true, y: false, setScale: true },
            points: { size: 9, width: 2, stroke: () => colors.line, fill: () => colors.surface }
          },
      select: { show: !spark, left: 0, top: 0, width: 0, height: 0 },
      scales: {
        x: { time: true, range: (_self, min, max) => xRange(min, max) },
        y: {
          range: (_self, min, max) => yRange(min, max, current.low, current.high)
        }
      },
      axes: spark
        ? [{ show: false }, { show: false }]
        : [
            { ...axisBase, size: 28, space: 70, values: axisTimeLabels },
            { ...axisBase, size: 48, space: 36, values: axisValueLabels }
          ],
      series: [
        {},
        {
          stroke: () => colors.line,
          width: spark ? 1.5 : 2,
          points: spark
            ? { show: false }
            : { size: 5, width: 0, fill: () => colors.line, stroke: () => colors.line }
        }
      ],
      hooks: {
        drawClear: [drawBand],
        draw: [drawMarks],
        setCursor: [updateTip]
      }
    };
    plot = new lib(opts, [xs, vs], node);
    lastWidth = node.clientWidth;

    if (spark) return;
    tip = document.createElement('div');
    tip.className = 'chart-tip';
    tipParts = ['tip-value', 'tip-time', 'tip-note'].map((name) => {
      const part = document.createElement('div');
      part.className = name;
      tip!.appendChild(part);
      return part;
    });
    tip.style.display = 'none';
    plot.over.appendChild(tip);

    const over = plot.over;
    let downX = 0;
    over.addEventListener('mousedown', (e) => (downX = e.clientX));
    // A press that moved is a drag-zoom, not a tap.
    over.addEventListener('click', (e) => {
      const idx = plot?.cursor.idx;
      if (idx == null || Math.abs(e.clientX - downX) > 4) return;
      current.onselect?.(idx === current.selected ? null : idx);
    });
  }

  function destroy() {
    plot?.destroy();
    plot = null;
    tip = null;
    tipIndex = -1;
  }

  create();

  const resize = new ResizeObserver(() => {
    const width = node.clientWidth;
    if (plot && width && width !== lastWidth) {
      lastWidth = width;
      plot.setSize({ width, height: height() });
    }
  });
  resize.observe(node);

  const unwatch = watchTheme(() => {
    colors = readColors(node);
    plot?.redraw(false);
  });

  return {
    update() {
      const next = getParams();
      const prev = current;
      current = next;
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
        prepare();
        tipIndex = -1;
        if (tip) tip.style.display = 'none';
        plot?.setData([xs, vs]);
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

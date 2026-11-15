/** The theme token behind each canvas color; the app theme defines all of them for every theme. */
export const colorTokens = {
  line: 'var(--accent)',
  band: 'var(--success)',
  warn: 'var(--danger)',
  grid: 'var(--border)',
  axis: 'var(--text-muted)',
  select: 'var(--text)',
  surface: 'var(--surface)'
} as const;

export type ChartColors = Record<keyof typeof colorTokens, string> & { font: string };

/** Opacity of the ideal band fill and of its edge lines; the band color itself stays opaque so canvas can parse it. */
export const bandFillAlpha = 0.18;
export const bandEdgeAlpha = 0.8;

const fallback: ChartColors = {
  line: '#0a6e91',
  band: '#1a7f37',
  warn: '#b42318',
  grid: '#d0d7de',
  axis: '#526170',
  select: '#14202b',
  surface: '#ffffff',
  font: 'system-ui, sans-serif'
};

/** Resolves every chart token with the given resolver; an empty answer falls back to a fixed light color. */
export function resolveColors(resolve: (expression: string) => string, font: string): ChartColors {
  const out = { ...fallback, font: font || fallback.font };
  for (const key of Object.keys(colorTokens) as (keyof typeof colorTokens)[]) {
    out[key] = resolve(colorTokens[key]).trim() || fallback[key];
  }
  return out;
}

/** Reads the colors in effect at an element; canvas cannot use var() or light-dark(), so a probe element resolves each one. */
export function readColors(el: HTMLElement): ChartColors {
  const probe = document.createElement('span');
  el.appendChild(probe);
  try {
    return resolveColors((expression) => {
      probe.style.color = '';
      probe.style.color = expression;
      return probe.style.color === '' ? '' : getComputedStyle(probe).color;
    }, getComputedStyle(el).fontFamily);
  } finally {
    probe.remove();
  }
}

/** Calls cb when the theme attribute changes or the OS color scheme flips; returns an unsubscribe. */
export function watchTheme(cb: () => void): () => void {
  const observer = new MutationObserver(cb);
  observer.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] });
  const query = matchMedia('(prefers-color-scheme: dark)');
  query.addEventListener('change', cb);
  return () => {
    observer.disconnect();
    query.removeEventListener('change', cb);
  };
}

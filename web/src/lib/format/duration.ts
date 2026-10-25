import { getLocale } from '#lib/paraglide/runtime.js';

/** Describes a wait such as "30 seconds" or "15 minutes", rounding up to whole minutes from two minutes on. */
export function formatWait(seconds: number): string {
  const whole = Math.max(1, Math.ceil(seconds));
  const minutes = Math.ceil(whole / 60);
  const [unit, value] =
    whole >= 120 ? (['minute', minutes] as const) : (['second', whole] as const);
  return new Intl.NumberFormat(getLocale(), {
    style: 'unit',
    unit,
    unitDisplay: 'long'
  }).format(value);
}

import { getLocale } from '#lib/paraglide/runtime.js';

/** Shown in place of a value that is missing or not a number. */
export const emptyValue = '–';

const noBreakSpace = String.fromCharCode(0xa0);

const formats = new Map<string, Intl.NumberFormat>();

/** Formats with a fixed number of decimals in the UI locale; negative zero never shows a minus sign. */
export function formatNumber(value: number, decimals: number): string {
  if (!Number.isFinite(value)) return emptyValue;
  const digits = Math.min(Math.max(Math.trunc(decimals), 0), 20);
  const locale = getLocale();
  const key = `${locale}|${digits}`;
  let format = formats.get(key);
  if (!format) {
    format = new Intl.NumberFormat(locale, {
      minimumFractionDigits: digits,
      maximumFractionDigits: digits,
      signDisplay: 'negative'
    });
    formats.set(key, format);
  }
  return format.format(value);
}

/** A number with its unit after a no-break space; a missing or empty unit (such as pH) shows the number alone. */
export function formatValue(
  value: number | null | undefined,
  unit: string | null | undefined,
  decimals: number
): string {
  if (value === null || value === undefined) return emptyValue;
  const text = formatNumber(value, decimals);
  if (text === emptyValue || !unit) return text;
  return `${text}${noBreakSpace}${unit}`;
}

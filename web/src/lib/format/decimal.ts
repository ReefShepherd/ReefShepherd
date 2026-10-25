const decimalPattern = /^[+-]?(\d+([.,]\d*)?|[.,]\d+)$/;

/** Reads a typed number with either a decimal point or a decimal comma; anything else, including grouping separators, gives null. */
export function parseDecimal(input: string): number | null {
  const text = input.trim().replace('−', '-');
  if (!decimalPattern.test(text)) return null;
  const value = Number(text.replace(',', '.'));
  return Number.isFinite(value) ? value : null;
}

/** A number as it is put in an input: plain digits with a decimal point and no grouping. */
export function decimalText(value: number | null | undefined, decimals: number): string {
  if (value === null || value === undefined || !Number.isFinite(value)) return '';
  const digits = Math.min(Math.max(Math.trunc(decimals), 0), 20);
  return value.toFixed(digits);
}

/** Reads a whole number from an input; empty, signed or fractional text gives null. */
export function parseWholeNumber(input: string): number | null {
  const text = input.trim();
  if (!/^\d{1,9}$/.test(text)) return null;
  return Number(text);
}

/** How many digits follow the decimal point when the number is written out in full. */
export function fractionDigits(value: number): number {
  const text = value.toFixed(10).replace(/0+$/, '');
  const dot = text.indexOf('.');
  return dot < 0 ? 0 : text.length - dot - 1;
}

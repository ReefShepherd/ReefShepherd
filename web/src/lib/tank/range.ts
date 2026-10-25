import { formatNumber } from '#lib/format/number.js';
import { m } from '#lib/paraglide/messages.js';

const noBreakSpace = String.fromCharCode(0xa0);

/** An ideal range as text, such as "7.9 to 8.4 pH" or "Up to 0.05 ppm"; unit is the label shown after the numbers. */
export function rangeText(
  low: number | null,
  high: number | null,
  unit: string,
  decimals: number
): string {
  const suffix = unit ? `${noBreakSpace}${unit}` : '';
  if (low !== null && high !== null) {
    return m.params_range_between({
      low: formatNumber(low, decimals),
      high: formatNumber(high, decimals) + suffix
    });
  }
  if (high !== null) return m.params_range_max({ high: formatNumber(high, decimals) + suffix });
  if (low !== null) return m.params_range_min({ low: formatNumber(low, decimals) + suffix });
  return m.params_range_none();
}

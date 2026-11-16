import { emptyValue, formatValue } from './number.js';

const units = ['B', 'KB', 'MB', 'GB', 'TB'];

/** A file size in binary steps with one decimal above a byte, such as "1.5 MB" with a no-break space; a missing or negative size shows the empty value. */
export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) return emptyValue;
  let value = bytes;
  let unit = 0;
  const rollsOver = () => value >= 1024 || (unit > 0 && Number(value.toFixed(1)) >= 1024);
  while (unit < units.length - 1 && rollsOver()) {
    value /= 1024;
    unit++;
  }
  return formatValue(unit === 0 ? Math.round(value) : value, units[unit], unit === 0 ? 0 : 1);
}

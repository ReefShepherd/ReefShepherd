const partsFormats = new Map<string, Intl.DateTimeFormat>();

function partsFormat(timeZone: string): Intl.DateTimeFormat {
  let format = partsFormats.get(timeZone);
  if (!format) {
    format = new Intl.DateTimeFormat('en-US', {
      timeZone,
      hourCycle: 'h23',
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit'
    });
    partsFormats.set(timeZone, format);
  }
  return format;
}

interface WallClock {
  year: number;
  month: number;
  day: number;
  hour: number;
  minute: number;
  second: number;
}

function wallClock(instant: number, timeZone: string): WallClock {
  const out: Record<string, number> = {};
  for (const part of partsFormat(timeZone).formatToParts(new Date(instant))) {
    if (part.type !== 'literal') out[part.type] = Number(part.value);
  }
  return out as unknown as WallClock;
}

const pad = (n: number, width = 2) => String(n).padStart(width, '0');

/** The wall-clock time in the zone minus the real time, in milliseconds, at the given instant. */
function zoneOffset(instant: number, timeZone: string): number {
  const w = wallClock(instant, timeZone);
  const asUtc = Date.UTC(w.year, w.month - 1, w.day, w.hour, w.minute, w.second);
  return asUtc - Math.floor(instant / 1000) * 1000;
}

/** An instant as "YYYY-MM-DDTHH:mm" on the wall clock of the zone, the format of a datetime-local input. */
export function toLocalInput(instant: Date | string, timeZone: string): string {
  const time = new Date(instant).getTime();
  if (Number.isNaN(time)) return '';
  const w = wallClock(time, timeZone);
  return `${pad(w.year, 4)}-${pad(w.month)}-${pad(w.day)}T${pad(w.hour)}:${pad(w.minute)}`;
}

/** Today's date as "YYYY-MM-DD" in the zone. */
export function todayIn(timeZone: string, now: Date | number = Date.now()): string {
  return toLocalInput(new Date(now), timeZone).slice(0, 10);
}

const localInputPattern = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})(?::(\d{2}))?$/;

/** Turns a wall-clock time in the zone into an RFC 3339 UTC instant, or null when the text is not a real date and time; a time skipped by a clock change moves forward. */
export function fromLocalInput(text: string, timeZone: string): string | null {
  const match = localInputPattern.exec(text.trim());
  if (!match) return null;
  const [year, month, day, hour, minute, second] = match.slice(1).map((s) => Number(s ?? 0));
  const wall = Date.UTC(year, month - 1, day, hour, minute, second);
  const check = new Date(wall);
  if (
    check.getUTCFullYear() !== year ||
    check.getUTCMonth() !== month - 1 ||
    hour > 23 ||
    minute > 59 ||
    second > 59
  ) {
    return null;
  }
  // The offset is looked up at a first guess and again at the corrected instant, which settles it on either side of a clock change.
  let instant = wall - zoneOffset(wall, timeZone);
  instant = wall - zoneOffset(instant, timeZone);
  return new Date(instant).toISOString().replace(/\.\d{3}Z$/, 'Z');
}

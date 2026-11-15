/** The IANA timezone the browser is in, or undefined when it cannot tell. */
export function browserTimeZone(): string | undefined {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || undefined;
  } catch {
    return undefined;
  }
}

/** The canonical spelling of an IANA timezone name, or undefined when the browser does not know it. */
export function normalizeTimeZone(input: string): string | undefined {
  const name = input.trim();
  try {
    return new Intl.DateTimeFormat('en', { timeZone: name }).resolvedOptions().timeZone;
  } catch {
    return undefined;
  }
}

/** Every timezone name the browser offers, with UTC first because some browsers leave it out of the list. */
export function timeZoneNames(): string[] {
  let names: string[] = [];
  try {
    names = Intl.supportedValuesOf('timeZone');
  } catch {
    /* older browsers have no list */
  }
  return ['UTC', ...names.filter((n) => n !== 'UTC')];
}

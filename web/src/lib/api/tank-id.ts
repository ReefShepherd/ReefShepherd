/** A tank or parameter id from text such as a URL segment or a stored value; null unless it is a positive whole number. */
export function parseId(value: unknown): number | null {
  if (typeof value !== 'string' || !/^[1-9]\d*$/.test(value)) return null;
  const id = Number(value);
  return Number.isSafeInteger(id) ? id : null;
}

/** The path of a tank or of something under it, such as /tanks/3/results; the API and the app's pages share this shape. */
export function tankPath(tankId: number, ...segments: (string | number)[]): string {
  return `/tanks/${[tankId, ...segments].map((s) => encodeURIComponent(String(s))).join('/')}`;
}

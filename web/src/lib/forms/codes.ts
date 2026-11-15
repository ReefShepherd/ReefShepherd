/** Why a field was refused: the closed list the server and the browser both use. */
export const fieldCodes = [
  'required',
  'invalid',
  'too_short',
  'too_long',
  'out_of_range',
  'taken',
  'incorrect',
  'locked'
] as const;

export type FieldCode = (typeof fieldCodes)[number];

/** The refused fields of a form, keyed by the server's field names. */
export type FieldErrors = Partial<Record<string, FieldCode>>;

export const hasErrors = (errors: object): boolean => Object.keys(errors).length > 0;

const isFieldCode = (value: string): value is FieldCode =>
  (fieldCodes as readonly string[]).includes(value);

// The sentences the server sent for a field before it sent codes, tried in order.
const phrases: readonly [FieldCode, RegExp][] = [
  [
    'required',
    /^(is )?required\b|^must not be (blank|null)|^must (have|list) at least one|^send either code|is blank$|needs an anchor date$/
  ],
  ['too_short', /^must be at least \d+ characters/],
  ['too_long', /^must be at most \d+ (characters|bytes)|^must (have|list) at most \d+ /],
  [
    'out_of_range',
    /^must be between |^is outside the possible range|^must be a positive, realistic|^must be a whole number from \d|^must not be (above|after) |not in \d+\.\.\d+|out of range/
  ],
  ['taken', /^already taken|already has this name/],
  ['incorrect', /^incorrect password|^that code is not valid|^must be the tank's name/],
  ['locked', /^set by REEFSHEPHERD_/]
];

/** The code for what the server sent for a field: a code itself, or one of its older sentences, and `invalid` for anything else. */
export function fieldCodeOf(detail: string): FieldCode {
  if (isFieldCode(detail)) return detail;
  return phrases.find(([, pattern]) => pattern.test(detail))?.[0] ?? 'invalid';
}

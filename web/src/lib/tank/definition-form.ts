import type { NewParameter, ParameterChanges, ParameterDefinition } from '#lib/api/params.js';
import { decimalText, parseDecimal, parseWholeNumber } from '#lib/format/decimal.js';
import type { FieldCode, FieldErrors } from '#lib/forms/codes.js';
import {
  collectErrors,
  validateLimitPair,
  validateMaxLength,
  validateName
} from '#lib/forms/validation.js';

export const parameterNameMax = 60;
export const unitLabelMax = 16;
export const maxTestInterval = 3650;

/** What the custom parameter form holds, as typed. */
export interface DefinitionDraft {
  name: string;
  unitLabel: string;
  decimals: number;
  low: string;
  high: string;
  /** Days between tests, or empty for no default schedule. */
  intervalDays: string;
}

export function draftFromDefinition(def: ParameterDefinition | undefined): DefinitionDraft {
  const decimals = def?.decimals ?? 2;
  return {
    name: def?.name ?? '',
    unitLabel: def?.unit_label ?? '',
    decimals,
    low: decimalText(def?.default_low, decimals),
    high: decimalText(def?.default_high, decimals),
    intervalDays: def?.default_test_interval_days?.toString() ?? ''
  };
}

const intervalCode = (text: string): FieldCode | null => {
  if (text.trim() === '') return null;
  const days = parseWholeNumber(text);
  if (days === null) return 'invalid';
  return days < 1 || days > maxTestInterval ? 'out_of_range' : null;
};

/** The fields of the parameter draft that cannot be saved, keyed by the server's field names; `range` flags a low limit above the high one. */
export function validateDefinition(draft: DefinitionDraft): FieldErrors {
  return {
    ...validateLimitPair(draft.low, draft.high, { low: 'default_low', high: 'default_high' }),
    ...collectErrors({
      name: validateName(draft.name, parameterNameMax),
      unit_label: validateMaxLength(draft.unitLabel.trim(), unitLabelMax),
      default_test_interval_days: intervalCode(draft.intervalDays)
    })
  };
}

const limit = (text: string) => (text.trim() === '' ? null : parseDecimal(text));
const interval = (text: string) => (text.trim() === '' ? null : parseWholeNumber(text));

/** The request that creates a custom parameter; the draft must have passed validateDefinition. */
export function newParameterBody(draft: DefinitionDraft): NewParameter {
  return {
    name: draft.name.trim(),
    unit_label: draft.unitLabel.trim(),
    decimals: draft.decimals,
    default_low: limit(draft.low),
    default_high: limit(draft.high),
    default_test_interval_days: interval(draft.intervalDays)
  };
}

/** The fields of the parameter the draft changes. */
export function definitionChanges(
  def: ParameterDefinition,
  draft: DefinitionDraft
): ParameterChanges {
  const next = newParameterBody(draft);
  const changes: ParameterChanges = {};
  if (next.name !== def.name) changes.name = next.name;
  if (next.unit_label !== def.unit_label) changes.unit_label = next.unit_label;
  if (next.decimals !== def.decimals) changes.decimals = next.decimals;
  if (next.default_low !== def.default_low) changes.default_low = next.default_low;
  if (next.default_high !== def.default_high) changes.default_high = next.default_high;
  if (next.default_test_interval_days !== def.default_test_interval_days) {
    changes.default_test_interval_days = next.default_test_interval_days;
  }
  return changes;
}

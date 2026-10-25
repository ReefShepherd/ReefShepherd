import type { NewParameter, ParameterChanges, ParameterDefinition } from '#lib/api/params.js';
import { decimalText, parseDecimal, parseWholeNumber } from '#lib/format/decimal.js';

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

export interface DefinitionProblems {
  name?: 'required' | 'too_long';
  unitLabel?: 'too_long';
  low?: 'invalid_number';
  high?: 'invalid_number';
  range?: 'low_above_high';
  intervalDays?: 'invalid_interval';
}

const length = (text: string) => [...text].length;

export function definitionProblems(draft: DefinitionDraft): DefinitionProblems {
  const problems: DefinitionProblems = {};
  const name = draft.name.trim();
  if (name === '') problems.name = 'required';
  else if (length(name) > parameterNameMax) problems.name = 'too_long';
  if (length(draft.unitLabel.trim()) > unitLabelMax) problems.unitLabel = 'too_long';
  const low = draft.low.trim() === '' ? null : parseDecimal(draft.low);
  const high = draft.high.trim() === '' ? null : parseDecimal(draft.high);
  if (draft.low.trim() !== '' && low === null) problems.low = 'invalid_number';
  if (draft.high.trim() !== '' && high === null) problems.high = 'invalid_number';
  if (low !== null && high !== null && low > high) problems.range = 'low_above_high';
  if (draft.intervalDays.trim() !== '') {
    const days = parseWholeNumber(draft.intervalDays);
    if (days === null || days < 1 || days > maxTestInterval)
      problems.intervalDays = 'invalid_interval';
  }
  return problems;
}

const limit = (text: string) => (text.trim() === '' ? null : parseDecimal(text));
const interval = (text: string) => (text.trim() === '' ? null : parseWholeNumber(text));

/** The request that creates a custom parameter; the draft must have passed definitionProblems. */
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

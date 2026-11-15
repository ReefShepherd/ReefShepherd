import type { UnitFamily } from '#lib/api/settings.js';
import type { NewTank, Tank, TankChanges, TankType } from '#lib/api/tanks.js';
import { decimalText, parseDecimal } from '#lib/format/decimal.js';
import type { FieldCode, FieldErrors } from '#lib/forms/codes.js';
import {
  collectErrors,
  validateMaxLength,
  validateName,
  validateOptionalNumber
} from '#lib/forms/validation.js';

export const tankNameMax = 100;
export const tankNoteMax = 10000;

/** What the tank form holds, with the volume and date as typed. */
export interface TankDraft {
  name: string;
  tankType: TankType;
  volume: string;
  /** YYYY-MM-DD, or empty. */
  startedOn: string;
  note: string;
}

export function draftFromTank(tank: Tank | undefined): TankDraft {
  return {
    name: tank?.name ?? '',
    tankType: tank?.tank_type ?? 'reef',
    volume: decimalText(tank?.volume, 1).replace(/\.0$/, ''),
    startedOn: tank?.started_on ?? '',
    note: tank?.note ?? ''
  };
}

const volumeCode = (text: string): FieldCode | null => {
  const code = validateOptionalNumber(text);
  if (code !== null || text.trim() === '') return code;
  return (parseDecimal(text) ?? 0) > 0 ? null : 'out_of_range';
};

/** The fields of the tank draft that cannot be saved, keyed by the server's field names. */
export function validateTank(draft: TankDraft): FieldErrors {
  return collectErrors({
    name: validateName(draft.name, tankNameMax),
    volume: volumeCode(draft.volume),
    note: validateMaxLength(draft.note, tankNoteMax)
  });
}

const volumeOf = (text: string) => (text.trim() === '' ? null : parseDecimal(text));

/** The request that creates a tank with the chosen presets; the draft must have passed validateTank. */
export function newTankBody(draft: TankDraft, presets: readonly string[]): NewTank {
  return {
    name: draft.name.trim(),
    tank_type: draft.tankType,
    volume: volumeOf(draft.volume),
    started_on: draft.startedOn === '' ? null : draft.startedOn,
    note: draft.note.trim(),
    presets: [...presets]
  };
}

/** The fields of the tank the draft changes; the volume is only sent when its text changed, so a rounded display value never overwrites the stored one. */
export function tankChanges(tank: Tank, draft: TankDraft): TankChanges {
  const changes: TankChanges = {};
  const original = draftFromTank(tank);
  if (draft.name.trim() !== tank.name) changes.name = draft.name.trim();
  if (draft.tankType !== tank.tank_type) changes.tank_type = draft.tankType;
  if (draft.volume.trim() !== original.volume) changes.volume = volumeOf(draft.volume);
  if (draft.startedOn !== original.startedOn) {
    changes.started_on = draft.startedOn === '' ? null : draft.startedOn;
  }
  if (draft.note.trim() !== tank.note) changes.note = draft.note.trim();
  return changes;
}

/** The label of the volume unit with the given code, falling back to the code itself. */
export function volumeUnitLabel(catalog: readonly UnitFamily[], unit: string): string {
  const family = catalog.find((f) => f.family === 'volume');
  return family?.units.find((u) => u.unit === unit)?.label ?? unit;
}

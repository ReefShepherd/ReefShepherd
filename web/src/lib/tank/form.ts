import type { UnitFamily } from '#lib/api/settings.js';
import type { NewTank, Tank, TankChanges, TankType } from '#lib/api/tanks.js';
import { decimalText, parseDecimal } from '#lib/format/decimal.js';

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

export interface TankProblems {
  name?: 'required' | 'too_long';
  volume?: 'invalid_number' | 'not_positive';
  note?: 'too_long';
}

const length = (text: string) => [...text].length;

export function tankProblems(draft: TankDraft): TankProblems {
  const problems: TankProblems = {};
  const name = draft.name.trim();
  if (name === '') problems.name = 'required';
  else if (length(name) > tankNameMax) problems.name = 'too_long';
  if (draft.volume.trim() !== '') {
    const volume = parseDecimal(draft.volume);
    if (volume === null) problems.volume = 'invalid_number';
    else if (volume <= 0) problems.volume = 'not_positive';
  }
  if (length(draft.note) > tankNoteMax) problems.note = 'too_long';
  return problems;
}

const volumeOf = (text: string) => (text.trim() === '' ? null : parseDecimal(text));

/** The request that creates a tank with the chosen presets; the draft must have passed tankProblems. */
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

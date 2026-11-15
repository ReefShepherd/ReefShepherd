import { describe, expect, it } from 'vitest';
import type { TestResult } from '#lib/api/params.js';
import { draftFromResult, resultChanges } from './edit.js';

const zone = 'Europe/Berlin';
const saved: TestResult = {
  id: 9,
  tank_id: 1,
  parameter_id: 3,
  parameter_name: 'Salinity',
  measured_at: '2026-10-24T08:30:45Z',
  value: 1.025,
  unit: 'sg',
  unit_label: 'SG',
  in_range: true,
  note: 'fresh',
  source: 'manual',
  created_by: 1,
  created_at: '2026-10-24T08:30:45Z',
  updated_at: '2026-10-24T08:30:45Z'
};

describe('draftFromResult', () => {
  it('shows the value with its decimals and the time on the instance wall clock', () => {
    expect(draftFromResult(saved, 3, zone)).toEqual({
      value: '1.025',
      note: 'fresh',
      measuredAt: '2026-10-24T10:30'
    });
  });
});

describe('resultChanges', () => {
  it('sends nothing for an untouched draft, even though the stored time has seconds', () => {
    const edit = resultChanges(saved, draftFromResult(saved, 3, zone), zone);
    expect(edit.changes).toEqual({});
    expect(edit.errors).toEqual({});
  });

  it('sends only the fields that changed', () => {
    const draft = { ...draftFromResult(saved, 3, zone), value: '1,026' };
    expect(resultChanges(saved, draft, zone).changes).toEqual({ value: 1.026 });
    const note = { ...draftFromResult(saved, 3, zone), note: ' ' };
    expect(resultChanges(saved, note, zone).changes).toEqual({ note: '' });
    const time = { ...draftFromResult(saved, 3, zone), measuredAt: '2026-10-24T11:00' };
    expect(resultChanges(saved, time, zone).changes).toEqual({
      measured_at: '2026-10-24T09:00:00Z'
    });
  });

  it('reports a value or time that cannot be read', () => {
    const draft = { value: 'abc', note: 'fresh', measuredAt: 'later' };
    const edit = resultChanges(saved, draft, zone);
    expect(edit.errors).toEqual({ value: 'invalid', measured_at: 'invalid' });
  });
});

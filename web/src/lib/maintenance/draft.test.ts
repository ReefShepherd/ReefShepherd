import { describe, expect, it } from 'vitest';
import { emptySchedule } from '#lib/schedule/schedule.js';
import { preset, task } from '#lib/test/maintenance-fixtures.js';
import {
  blankDraft,
  blankPresetDraft,
  descriptionMax,
  draftFromPreset,
  draftFromPresetRecord,
  draftFromTask,
  nameMax,
  newPreset,
  newTask,
  presetChanges,
  tankChoices,
  taskChanges,
  wholeSystem
} from './draft.js';

const today = '2026-11-15';

describe('blankDraft', () => {
  it('starts without a schedule at the reminder time, for the tank or the whole system', () => {
    const draft = blankDraft(3, '07:30');
    expect(draft).toMatchObject({ tank: '3', presetId: '', notify: true, paused: false });
    expect(draft.schedule).toEqual(emptySchedule('07:30'));
    expect(blankDraft(null, '07:30').tank).toBe(wholeSystem);
  });
});

describe('draftFromPreset', () => {
  it('fills name, description and schedule and keeps the tank chosen', () => {
    const draft = draftFromPreset(blankDraft(3, '09:00'), preset(), '09:00');
    expect(draft).toMatchObject({
      presetId: '7',
      name: 'Water change',
      description: 'Change a tenth of the water.',
      tank: '3'
    });
    expect(draft.schedule).toMatchObject({ kind: 'weekly', weekdays: 64 });
  });

  it('picks the whole system for a template made for it', () => {
    const draft = draftFromPreset(blankDraft(3, '09:00'), preset({ tankless: true }), '09:00');
    expect(draft.tank).toBe(wholeSystem);
  });

  it('moves from the whole system back to the fallback tank for a template that belongs to a tank', () => {
    const system = draftFromPreset(
      blankDraft(3, '09:00'),
      preset({ tankless: true }),
      '09:00',
      '3'
    );
    expect(system.tank).toBe(wholeSystem);
    const back = draftFromPreset(system, preset(), '09:00', '3');
    expect(back.tank).toBe('3');
  });

  it('keeps a tank already chosen over the fallback', () => {
    const chosen = { ...blankDraft(5, '09:00') };
    expect(draftFromPreset(chosen, preset(), '09:00', '3').tank).toBe('5');
  });

  it('stays on the whole system when there is no fallback tank', () => {
    const system = { ...blankDraft(null, '09:00') };
    expect(draftFromPreset(system, preset(), '09:00').tank).toBe(wholeSystem);
  });

  it('keeps what else was typed and fills a missing time from the reminder time', () => {
    const base = { ...blankDraft(3, '09:00'), lastDoneOn: '2026-11-01', notify: false };
    const bare = preset({ schedule: { ...preset().schedule, due_time: '' } });
    const draft = draftFromPreset(base, bare, '06:15');
    expect(draft).toMatchObject({ lastDoneOn: '2026-11-01', notify: false });
    expect(draft.schedule.due_time).toBe('06:15');
  });
});

describe('newTask', () => {
  const filled = () => ({ ...blankDraft(3, '09:00'), name: '  Skimmer cup  ' });

  it('builds the request with trimmed text and a normalized schedule', () => {
    const draft = {
      ...filled(),
      presetId: '7',
      description: ' Rinse it ',
      schedule: {
        ...emptySchedule('09:00'),
        kind: 'after_last' as const,
        interval_days: 3,
        weekdays: 5
      },
      lastDoneOn: '2026-11-10'
    };
    const entry = newTask(draft, today);
    expect(entry.body).toEqual({
      tank_id: 3,
      preset_id: 7,
      name: 'Skimmer cup',
      description: 'Rinse it',
      schedule: { ...emptySchedule('09:00'), kind: 'after_last', interval_days: 3 },
      notify: true,
      last_done_on: '2026-11-10'
    });
  });

  it('sends no preset and no last-done date when none is chosen, and null for the whole system', () => {
    const entry = newTask({ ...filled(), tank: wholeSystem }, today);
    expect(entry.body).toMatchObject({ tank_id: null, preset_id: null });
    expect(entry.body).not.toHaveProperty('last_done_on');
  });

  it('refuses a missing name, long text and a tank that is no id', () => {
    const entry = newTask(
      {
        ...blankDraft(3, '09:00'),
        name: ' ',
        description: 'x'.repeat(descriptionMax + 1),
        tank: 'abc'
      },
      today
    );
    expect(entry.body).toBeNull();
    expect(entry.errors).toEqual({ name: 'required', description: 'too_long', tank_id: 'invalid' });
    expect(newTask({ ...filled(), name: 'x'.repeat(nameMax + 1) }, today).errors).toEqual({
      name: 'too_long'
    });
  });

  it('refuses a last-done date that is not a date or lies in the future', () => {
    expect(newTask({ ...filled(), lastDoneOn: '2026-02-30' }, today).errors).toEqual({
      last_done_on: 'invalid'
    });
    expect(newTask({ ...filled(), lastDoneOn: '2026-11-16' }, today).errors).toEqual({
      last_done_on: 'out_of_range'
    });
    expect(newTask({ ...filled(), lastDoneOn: today }, today).body).not.toBeNull();
  });

  it('points at the schedule fields that cannot be saved', () => {
    const draft = {
      ...filled(),
      schedule: { ...emptySchedule('09:00'), kind: 'after_last' as const, interval_days: null }
    };
    const entry = newTask(draft, today);
    expect(entry.body).toBeNull();
    expect(entry.schedule).toEqual(['interval_days']);
  });
});

describe('taskChanges', () => {
  const saved = task({ name: 'Water change', description: 'old', tank_id: 3 });

  it('sends nothing for a draft that matches the task', () => {
    const edit = taskChanges(saved, draftFromTask(saved));
    expect(edit.changes).toEqual({});
    expect(edit.errors).toEqual({});
    expect(edit.schedule).toEqual([]);
  });

  it('sends only the fields that changed', () => {
    const draft = { ...draftFromTask(saved), name: ' Big water change ', notify: false };
    expect(taskChanges(saved, draft).changes).toEqual({
      name: 'Big water change',
      notify: false
    });
  });

  it('moves a task to the whole system with a null tank and back to a tank by its id', () => {
    const toSystem = taskChanges(saved, { ...draftFromTask(saved), tank: wholeSystem });
    expect(toSystem.changes).toEqual({ tank_id: null });
    const system = task({ tank_id: null, tank_name: null });
    expect(taskChanges(system, { ...draftFromTask(system), tank: '8' }).changes).toEqual({
      tank_id: 8
    });
  });

  it('sends the whole schedule when any part of it changes', () => {
    const draft = draftFromTask(saved);
    draft.schedule = { ...draft.schedule, interval_days: 14 };
    expect(taskChanges(saved, draft).changes.schedule).toEqual({
      ...emptySchedule('09:00'),
      kind: 'after_last',
      interval_days: 14
    });
  });

  it('does not send a schedule that cannot be saved and reports its problems', () => {
    const draft = draftFromTask(saved);
    draft.schedule = { ...draft.schedule, interval_days: 0 };
    const edit = taskChanges(saved, draft);
    expect(edit.changes).not.toHaveProperty('schedule');
    expect(edit.schedule).toEqual(['interval_days']);
  });

  it('pauses and resumes', () => {
    expect(taskChanges(saved, { ...draftFromTask(saved), paused: true }).changes).toEqual({
      paused: true
    });
  });

  it('reports a name that is gone', () => {
    expect(taskChanges(saved, { ...draftFromTask(saved), name: '' }).errors).toEqual({
      name: 'required'
    });
  });
});

describe('tankChoices', () => {
  const tanks = [{ id: 3, name: 'Display' }];

  it('offers the tanks given', () => {
    expect(tankChoices(tanks, undefined)).toEqual(tanks);
    expect(tankChoices(tanks, task({ tank_id: 3 }))).toEqual(tanks);
    expect(tankChoices(tanks, task({ tank_id: null, tank_name: null }))).toEqual(tanks);
  });

  it('adds the task own tank when it is not among them, such as an archived tank', () => {
    expect(tankChoices(tanks, task({ tank_id: 9, tank_name: 'Old' }))).toEqual([
      ...tanks,
      { id: 9, name: 'Old' }
    ]);
  });
});

describe('templates', () => {
  it('builds a new template and refuses a blank one', () => {
    const draft = { ...blankPresetDraft('09:00'), name: ' Filter socks ', tankless: true };
    expect(newPreset(draft).body).toEqual({
      name: 'Filter socks',
      description: '',
      tankless: true,
      schedule: emptySchedule('09:00')
    });
    expect(newPreset(blankPresetDraft('09:00')).errors).toEqual({ name: 'required' });
  });

  it('sends only what changed on a template', () => {
    const saved = preset();
    expect(presetChanges(saved, draftFromPresetRecord(saved)).changes).toEqual({});
    const draft = { ...draftFromPresetRecord(saved), description: 'More', tankless: true };
    expect(presetChanges(saved, draft).changes).toEqual({ description: 'More', tankless: true });
  });
});

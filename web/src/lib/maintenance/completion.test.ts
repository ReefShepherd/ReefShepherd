import { describe, expect, it } from 'vitest';
import { completion } from '#lib/test/maintenance-fixtures.js';
import {
  completionBody,
  completionChanges,
  draftFromCompletion,
  newCompletionDraft
} from './completion.js';

const zone = 'Europe/Berlin';
const now = new Date('2026-11-15T10:00:00Z');

describe('newCompletionDraft', () => {
  it('opens at the current wall-clock time of the instance', () => {
    expect(newCompletionDraft(now, zone)).toEqual({ doneAt: '2026-11-15T11:00', note: '' });
  });
});

describe('completionBody', () => {
  const opened = '2026-11-15T11:00';

  it('leaves the time out when it was not changed so the server stamps the save', () => {
    const entry = completionBody({ doneAt: opened, note: '' }, opened, 'done', zone, now);
    expect(entry).toEqual({ body: { outcome: 'done' }, errors: {} });
  });

  it('sends a changed time as UTC in the instance zone, with a trimmed note', () => {
    const entry = completionBody(
      { doneAt: '2026-11-14T08:15', note: ' by hand ' },
      opened,
      'skipped',
      zone,
      now
    );
    expect(entry.body).toEqual({
      outcome: 'skipped',
      done_at: '2026-11-14T07:15:00Z',
      note: 'by hand'
    });
  });

  it('refuses a time that is not a time or lies in the future', () => {
    expect(completionBody({ doneAt: 'soon', note: '' }, opened, 'done', zone, now).errors).toEqual({
      done_at: 'invalid'
    });
    expect(
      completionBody({ doneAt: '2026-11-15T12:00', note: '' }, opened, 'done', zone, now)
    ).toEqual({ body: null, errors: { done_at: 'out_of_range' } });
  });

  it('allows a few minutes ahead for a slow clock', () => {
    const entry = completionBody(
      { doneAt: '2026-11-15T11:04', note: '' },
      opened,
      'done',
      zone,
      now
    );
    expect(entry.body).not.toBeNull();
  });
});

describe('completionChanges', () => {
  const saved = completion({ done_at: '2026-11-10T09:00:00Z', note: 'ok' });

  it('sends nothing for an untouched draft', () => {
    expect(completionChanges(saved, draftFromCompletion(saved, zone), zone, now).changes).toEqual(
      {}
    );
  });

  it('sends the outcome, the note and a moved time', () => {
    const draft = {
      ...draftFromCompletion(saved, zone),
      outcome: 'skipped' as const,
      note: ' ',
      doneAt: '2026-11-10T12:00'
    };
    expect(completionChanges(saved, draft, zone, now).changes).toEqual({
      outcome: 'skipped',
      note: '',
      done_at: '2026-11-10T11:00:00Z'
    });
  });

  it('refuses a time that cannot be saved', () => {
    const draft = { ...draftFromCompletion(saved, zone), doneAt: '2026-12-01T12:00' };
    expect(completionChanges(saved, draft, zone, now).errors).toEqual({
      done_at: 'out_of_range'
    });
    expect(completionChanges(saved, { ...draft, doneAt: 'x' }, zone, now).errors).toEqual({
      done_at: 'invalid'
    });
  });
});

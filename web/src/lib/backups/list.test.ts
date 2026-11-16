import { describe, expect, it } from 'vitest';
import type { Backup } from '#lib/api/backups.js';
import { ApiError } from '#lib/api/client.js';
import { errorText } from '#lib/api/error-text.js';
import { m } from '#lib/paraglide/messages.js';
import { addBackup, createFailureText, kindLabel, nextAfterRemoval, removeBackup } from './list.js';

const backup = (name: string, created_at: string): Backup => ({
  name,
  kind: 'scheduled',
  size_bytes: 1,
  created_at
});

const older = backup('reefshepherd-20260101T030000Z.db', '2026-01-01T03:00:00Z');
const newer = backup('reefshepherd-20260102T030000Z.db', '2026-01-02T03:00:00Z');

describe('addBackup', () => {
  it('puts the newest backup first', () => {
    expect(addBackup([older], newer)).toEqual([newer, older]);
    expect(addBackup([newer], older)).toEqual([newer, older]);
  });

  it('replaces a backup of the same name instead of listing it twice', () => {
    const again = { ...newer, size_bytes: 99 };
    expect(addBackup([newer, older], again)).toEqual([again, older]);
  });

  it('does not change the list it was given', () => {
    const list = [older];
    addBackup(list, newer);
    expect(list).toEqual([older]);
  });
});

describe('removeBackup', () => {
  it('drops the named backup only', () => {
    expect(removeBackup([newer, older], newer.name)).toEqual([older]);
    expect(removeBackup([newer, older], 'other.db')).toEqual([newer, older]);
  });
});

describe('nextAfterRemoval', () => {
  const oldest = backup('reefshepherd-20251231T030000Z.db', '2025-12-31T03:00:00Z');
  const list = [newer, older, oldest];

  it('names the row that follows the removed one', () => {
    expect(nextAfterRemoval(list, newer.name)).toBe(older.name);
    expect(nextAfterRemoval(list, older.name)).toBe(oldest.name);
  });

  it('is null for the last row and for a name that is not listed', () => {
    expect(nextAfterRemoval(list, oldest.name)).toBeNull();
    expect(nextAfterRemoval(list, 'other.db')).toBeNull();
    expect(nextAfterRemoval([], older.name)).toBeNull();
  });
});

describe('createFailureText', () => {
  it('explains a same-second collision in its own words', () => {
    expect(createFailureText(new ApiError(409, 'conflict', 'x'))).toBe(m.backups_collision());
  });

  it('words any other failure the usual way', () => {
    const error = new ApiError(500, 'internal', 'x');
    expect(createFailureText(error)).toBe(errorText(error));
    expect(createFailureText(new ApiError(404, 'conflict', 'x'))).not.toBe(m.backups_collision());
  });
});

describe('kindLabel', () => {
  it('has its own wording for each kind', () => {
    expect(kindLabel('scheduled')).toBe(m.backups_kind_scheduled());
    expect(kindLabel('premigrate')).toBe(m.backups_kind_premigrate());
    expect(kindLabel('prerestore')).toBe(m.backups_kind_prerestore());
    const labels = new Set([
      kindLabel('scheduled'),
      kindLabel('premigrate'),
      kindLabel('prerestore')
    ]);
    expect(labels.size).toBe(3);
  });
});

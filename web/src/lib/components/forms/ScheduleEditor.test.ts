import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import { changeKind, emptySchedule, scheduleKinds } from '#lib/schedule/schedule.js';
import ScheduleEditor from './ScheduleEditor.svelte';

const weekly = changeKind(emptySchedule('09:00'), 'weekly', '2026-11-16');

function html(props: { showTime?: boolean }): string {
  return render(ScheduleEditor, {
    props: { value: weekly, allowedKinds: scheduleKinds, ...props }
  }).body;
}

describe('ScheduleEditor time field', () => {
  it('offers the time of day by default', () => {
    expect(html({})).toContain('type="time"');
  });

  it('leaves the time of day out when asked', () => {
    expect(html({ showTime: false })).not.toContain('type="time"');
  });
});

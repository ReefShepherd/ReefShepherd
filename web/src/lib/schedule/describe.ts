import { m } from '#lib/paraglide/messages.js';
import { getLocale } from '#lib/paraglide/runtime.js';
import { weekdayNames, type Schedule } from './schedule.js';

function when(s: Schedule): string {
  switch (s.kind) {
    case 'after_last':
      return m.schedule_summary_after_last({ days: s.interval_days ?? 0 });
    case 'every_n_days':
      return m.schedule_summary_every_n_days({
        days: s.interval_days ?? 0,
        date: s.anchor_date ?? ''
      });
    case 'weekly': {
      const days = weekdayNames(s.weekdays, getLocale(), 'short').join(', ');
      return s.interval_weeks > 1
        ? m.schedule_summary_weekly_n({
            weeks: s.interval_weeks,
            days,
            date: s.anchor_date ?? ''
          })
        : m.schedule_summary_weekly({ days });
    }
    case 'monthly':
      return m.schedule_summary_monthly({ day: s.month_day ?? 0 });
    default:
      return m.schedule_summary_none();
  }
}

/** A one-line description of a schedule, such as "7 days after the last time at 09:00". */
export function describeSchedule(s: Schedule): string {
  if (s.kind === 'none') return when(s);
  return m.schedule_summary_at({ summary: when(s), time: s.due_time });
}

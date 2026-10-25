<script lang="ts">
  import { todayIn } from '#lib/format/local-time.js';
  import { parseWholeNumber } from '#lib/format/decimal.js';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import {
    changeKind,
    setIntervalWeeks,
    scheduleProblems,
    toggleWeekday,
    weekdayBits,
    weekdayNames,
    type Schedule,
    type ScheduleKind,
    type ScheduleProblem
  } from '#lib/schedule/schedule.js';
  import { session } from '#lib/session/session.svelte.js';
  import SelectField from './SelectField.svelte';

  interface Props {
    /** The schedule columns; the editor replaces the object on every change. */
    value: Schedule;
    /** The kinds offered, in the order shown. */
    allowedKinds: readonly ScheduleKind[];
    legend?: string;
    /** Shows a message under each field the server would refuse. */
    showErrors?: boolean;
    /** A message from the server about the schedule as a whole. */
    error?: string;
    disabled?: boolean;
  }

  let {
    value = $bindable(),
    allowedKinds,
    legend,
    showErrors = false,
    error,
    disabled = false
  }: Props = $props();

  const uid = $props.id();
  const today = () => todayIn(session.timeZone);

  const kindLabels: Record<ScheduleKind, () => string> = {
    none: m.schedule_kind_none,
    after_last: m.schedule_kind_after_last,
    every_n_days: m.schedule_kind_every_n_days,
    weekly: m.schedule_kind_weekly,
    monthly: m.schedule_kind_monthly
  };
  const kindOptions = $derived(allowedKinds.map((k) => ({ value: k, label: kindLabels[k]() })));
  // A stored schedule may use a kind the caller does not offer; it stays selectable so it is not silently changed.
  const options = $derived(
    kindOptions.some((o) => o.value === value.kind)
      ? kindOptions
      : [...kindOptions, { value: value.kind, label: kindLabels[value.kind]() }]
  );

  const problems = $derived<ScheduleProblem[]>(showErrors ? scheduleProblems(value) : []);
  const has = (p: ScheduleProblem) => problems.includes(p);
  const dayNames = $derived(weekdayNames(127, getLocale(), 'short'));

  function wholeInput(event: Event): number | null {
    return parseWholeNumber((event.currentTarget as HTMLInputElement).value);
  }
</script>

<fieldset class="schedule" {disabled}>
  <legend>{legend ?? m.schedule_legend()}</legend>
  <div class="stack">
    <SelectField
      label={m.schedule_kind()}
      bind:value={() => value.kind, (kind) => (value = changeKind(value, kind, today()))}
      {options}
      {error}
    />

    {#if value.kind === 'after_last' || value.kind === 'every_n_days'}
      <div class="field">
        <label for="{uid}-days">
          {value.kind === 'after_last' ? m.schedule_days_after() : m.schedule_days_every()}
        </label>
        <input
          id="{uid}-days"
          type="number"
          inputmode="numeric"
          min="1"
          max="36500"
          value={value.interval_days ?? ''}
          oninput={(e) => (value = { ...value, interval_days: wholeInput(e) })}
          aria-invalid={has('interval_days') ? 'true' : undefined}
          aria-describedby={has('interval_days') ? `${uid}-days-error` : undefined}
        />
        {#if has('interval_days')}
          <p class="field-error" id="{uid}-days-error">{m.schedule_problem_interval_days()}</p>
        {/if}
      </div>
    {/if}

    {#if value.kind === 'weekly'}
      <fieldset>
        <legend>{m.schedule_weekdays()}</legend>
        <div class="days">
          {#each weekdayBits as bit, i (bit)}
            <label class="day">
              <input
                type="checkbox"
                checked={((value.weekdays ?? 0) & bit) !== 0}
                onchange={() => (value = toggleWeekday(value, bit))}
                aria-invalid={has('weekdays') ? 'true' : undefined}
                aria-describedby={has('weekdays') ? `${uid}-weekdays-error` : undefined}
              />
              <span>{dayNames[i]}</span>
            </label>
          {/each}
        </div>
        {#if has('weekdays')}
          <p class="field-error" id="{uid}-weekdays-error">{m.schedule_problem_weekdays()}</p>
        {/if}
      </fieldset>
      <div class="field">
        <label for="{uid}-weeks">{m.schedule_every_weeks()}</label>
        <input
          id="{uid}-weeks"
          type="number"
          inputmode="numeric"
          min="1"
          max="520"
          value={value.interval_weeks}
          oninput={(e) => (value = setIntervalWeeks(value, wholeInput(e) ?? 1, today()))}
          aria-invalid={has('interval_weeks') ? 'true' : undefined}
          aria-describedby={has('interval_weeks') ? `${uid}-weeks-error` : undefined}
        />
        {#if has('interval_weeks')}
          <p class="field-error" id="{uid}-weeks-error">{m.schedule_problem_interval_weeks()}</p>
        {/if}
      </div>
    {/if}

    {#if value.kind === 'monthly'}
      <div class="field">
        <label for="{uid}-day">{m.schedule_month_day()}</label>
        <input
          id="{uid}-day"
          type="number"
          inputmode="numeric"
          min="1"
          max="31"
          value={value.month_day ?? ''}
          oninput={(e) => (value = { ...value, month_day: wholeInput(e) })}
          aria-invalid={has('month_day') ? 'true' : undefined}
          aria-describedby={has('month_day') ? `${uid}-day-error` : `${uid}-day-hint`}
        />
        <p class="hint" id="{uid}-day-hint">{m.schedule_month_day_hint()}</p>
        {#if has('month_day')}
          <p class="field-error" id="{uid}-day-error">{m.schedule_problem_month_day()}</p>
        {/if}
      </div>
    {/if}

    {#if value.kind === 'every_n_days' || (value.kind === 'weekly' && value.interval_weeks > 1)}
      <div class="field">
        <label for="{uid}-anchor">{m.schedule_anchor()}</label>
        <input
          id="{uid}-anchor"
          type="date"
          value={value.anchor_date ?? ''}
          oninput={(e) =>
            (value = {
              ...value,
              anchor_date: (e.currentTarget as HTMLInputElement).value || null
            })}
          aria-invalid={has('anchor_date') ? 'true' : undefined}
          aria-describedby={has('anchor_date') ? `${uid}-anchor-error` : undefined}
        />
        {#if has('anchor_date')}
          <p class="field-error" id="{uid}-anchor-error">{m.schedule_problem_anchor_date()}</p>
        {/if}
      </div>
    {/if}

    {#if value.kind !== 'none'}
      <div class="field">
        <label for="{uid}-time">{m.schedule_due_time()}</label>
        <input
          id="{uid}-time"
          type="time"
          value={value.due_time}
          oninput={(e) =>
            (value = { ...value, due_time: (e.currentTarget as HTMLInputElement).value })}
          aria-invalid={has('due_time') ? 'true' : undefined}
          aria-describedby={has('due_time') ? `${uid}-time-error` : undefined}
        />
        {#if has('due_time')}
          <p class="field-error" id="{uid}-time-error">{m.schedule_problem_due_time()}</p>
        {/if}
      </div>
    {/if}
  </div>
</fieldset>

<style>
  .days {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1);
  }

  .day {
    position: relative;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: var(--touch);
    min-height: var(--touch);
    padding: 0 var(--space-2);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg);
    font-size: 0.875rem;
    cursor: pointer;
  }

  .day input {
    position: absolute;
    inset: 0;
    margin: 0;
    opacity: 0;
    cursor: pointer;
  }

  .day:has(input:checked) {
    border-color: var(--accent);
    background: var(--accent-soft);
    font-weight: 600;
  }

  .day:has(input:focus-visible) {
    outline: 2px solid var(--accent);
    outline-offset: 2px;
  }
</style>

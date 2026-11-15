<script lang="ts">
  import { errorText } from '#lib/api/error-text.js';
  import { fieldText } from '#lib/api/field-errors.js';
  import { paramsApi, type TankParameter } from '#lib/api/params.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import ScheduleEditor from '#lib/components/forms/ScheduleEditor.svelte';
  import ValueInput from '#lib/components/forms/ValueInput.svelte';
  import { focusFirstField, focusFirstInvalid } from '#lib/dom.js';
  import { formatDateTime } from '#lib/format/datetime.js';
  import { m } from '#lib/paraglide/messages.js';
  import { describeSchedule } from '#lib/schedule/describe.js';
  import { scheduleKinds } from '#lib/schedule/schedule.js';
  import { session } from '#lib/session/session.svelte.js';
  import {
    draftFromParameter,
    parameterChanges,
    type ParameterEdit
  } from '#lib/tank/parameter-settings.js';
  import { rangeText } from '#lib/tank/range.js';

  interface Props {
    tankId: number;
    parameter: TankParameter;
    canMoveUp: boolean;
    canMoveDown: boolean;
    /** Called with the saved parameter after the server accepted a change. */
    onchanged: (parameter: TankParameter) => void;
    onmove: (direction: -1 | 1) => void;
    disabled?: boolean;
  }

  let {
    tankId,
    parameter,
    canMoveUp,
    canMoveDown,
    onchanged,
    onmove,
    disabled = false
  }: Props = $props();

  let editing = $state(false);
  // The draft is rebuilt from the saved parameter each time editing starts.
  // svelte-ignore state_referenced_locally
  let draft = $state(draftFromParameter(parameter));
  let problems = $state<ParameterEdit['problems']>({});
  let showScheduleErrors = $state(false);
  let serverErrors = $state<{ low?: string; high?: string; schedule?: string }>({});
  let failure = $state('');
  let busy = $state(false);
  let form = $state<HTMLFormElement>();

  const unit = $derived(parameter.unit_label);

  function startEditing() {
    draft = draftFromParameter(parameter);
    problems = {};
    serverErrors = {};
    showScheduleErrors = false;
    failure = '';
    editing = true;
  }

  async function toggleEnabled(event: Event) {
    const enabled = (event.currentTarget as HTMLInputElement).checked;
    failure = '';
    busy = true;
    try {
      onchanged(await paramsApi.saveTankParameter(tankId, parameter.parameter_id, { enabled }));
    } catch (e) {
      failure = errorText(e);
      (event.target as HTMLInputElement).checked = parameter.enabled;
    } finally {
      busy = false;
    }
  }

  async function save(event: SubmitEvent) {
    event.preventDefault();
    failure = '';
    serverErrors = {};
    const edit = parameterChanges(parameter, draft);
    problems = edit.problems;
    showScheduleErrors = true;
    if (Object.keys(problems).length > 0) return focusFirstInvalid(form);
    if (Object.keys(edit.changes).length === 0) {
      editing = false;
      return;
    }

    busy = true;
    try {
      const saved = await paramsApi.saveTankParameter(tankId, parameter.parameter_id, edit.changes);
      editing = false;
      onchanged(saved);
    } catch (e) {
      serverErrors = {
        low: fieldText(e, 'ideal_low'),
        high: fieldText(e, 'ideal_high'),
        schedule: fieldText(e, 'schedule') ?? fieldText(e, 'schedule.kind')
      };
      if (!Object.values(serverErrors).some(Boolean)) failure = errorText(e);
      await focusFirstInvalid(form);
    } finally {
      busy = false;
    }
  }

  const numberText = (p?: 'invalid_number') => (p ? m.value_invalid_number() : undefined);
  const lowText = $derived(numberText(problems.low) ?? serverErrors.low);
  const highText = $derived(
    numberText(problems.high) ??
      (problems.range ? m.params_range_order() : undefined) ??
      serverErrors.high
  );
</script>

<li class="card item" class:off={!parameter.enabled} data-parameter-id={parameter.parameter_id}>
  <div class="head">
    <label class="choice grow">
      <input
        type="checkbox"
        checked={parameter.enabled}
        disabled={busy || disabled}
        onchange={toggleEnabled}
      />
      <span class="choice-text">
        <span><strong>{parameter.name}</strong></span>
        <span class="hint">
          {m.detail_pair({
            first: rangeText(parameter.ideal_low, parameter.ideal_high, unit, parameter.decimals),
            second: describeSchedule(parameter.schedule)
          })}
        </span>
        {#if parameter.enabled && parameter.next_due}
          <span class="hint">
            {parameter.overdue ? m.params_overdue_since() : m.params_next_due()}
            {formatDateTime(parameter.next_due, session.timeZone)}
          </span>
        {/if}
      </span>
    </label>
    <div class="row">
      <button
        type="button"
        class="btn"
        disabled={!canMoveUp || disabled}
        data-action="up"
        onclick={() => onmove(-1)}
        aria-label={m.params_move_up({ name: parameter.name })}>↑</button
      >
      <button
        type="button"
        class="btn"
        disabled={!canMoveDown || disabled}
        data-action="down"
        onclick={() => onmove(1)}
        aria-label={m.params_move_down({ name: parameter.name })}>↓</button
      >
    </div>
  </div>

  {#if failure}
    <Alert kind="error">{failure}</Alert>
  {/if}

  {#if editing}
    <form class="stack" novalidate bind:this={form} onsubmit={save} use:focusFirstField>
      <div class="grid">
        <ValueInput label={m.params_ideal_low()} bind:value={draft.low} {unit} error={lowText} />
        <ValueInput label={m.params_ideal_high()} bind:value={draft.high} {unit} error={highText} />
      </div>
      <ScheduleEditor
        bind:value={draft.schedule}
        allowedKinds={scheduleKinds}
        showErrors={showScheduleErrors}
        error={serverErrors.schedule}
      />
      <div class="row">
        <button class="btn primary" type="submit" disabled={busy}>{m.form_save()}</button>
        <button class="btn" type="button" disabled={busy} onclick={() => (editing = false)}>
          {m.form_cancel()}
        </button>
      </div>
    </form>
  {:else}
    <button type="button" class="btn" {disabled} onclick={startEditing}>
      {m.form_edit()}<span class="sr-only"> {parameter.name}</span>
    </button>
  {/if}
</li>

<style>
  .item {
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .item.off .choice-text {
    opacity: 0.75;
  }

  .head {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-start;
    gap: var(--space-2);
  }

  .grow {
    flex: 1 1 14rem;
  }

  .grid {
    display: grid;
    gap: var(--space-3);
    grid-template-columns: repeat(auto-fit, minmax(9rem, 1fr));
  }

  .item > button {
    align-self: flex-start;
  }
</style>

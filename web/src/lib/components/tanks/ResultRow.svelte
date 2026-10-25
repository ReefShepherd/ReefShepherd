<script lang="ts">
  import { errorText } from '#lib/api/error-text.js';
  import { fieldText } from '#lib/api/field-errors.js';
  import { paramsApi, type TestResult } from '#lib/api/params.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import ConfirmButton from '#lib/components/forms/ConfirmButton.svelte';
  import TextField from '#lib/components/forms/TextField.svelte';
  import ValueInput from '#lib/components/forms/ValueInput.svelte';
  import { focusFirstField, focusFirstInvalid } from '#lib/dom.js';
  import { formatDateTime } from '#lib/format/datetime.js';
  import { formatValue } from '#lib/format/number.js';
  import { m } from '#lib/paraglide/messages.js';
  import { draftFromResult, resultChanges } from '#lib/results/edit.js';

  interface Props {
    result: TestResult;
    /** How many decimals the parameter is shown with. */
    decimals: number;
    timeZone: string;
    onupdated: (result: TestResult) => void;
    ondeleted: (id: number) => void;
  }

  let { result, decimals, timeZone, onupdated, ondeleted }: Props = $props();

  let editing = $state(false);
  // The draft is rebuilt from the saved result each time editing starts.
  // svelte-ignore state_referenced_locally
  let draft = $state(draftFromResult(result, decimals, timeZone));
  let errors = $state<{ value?: string; note?: string; time?: string }>({});
  let failure = $state('');
  let busy = $state(false);
  let form = $state<HTMLFormElement>();

  function startEditing() {
    draft = draftFromResult(result, decimals, timeZone);
    errors = {};
    failure = '';
    editing = true;
  }

  async function save(event: SubmitEvent) {
    event.preventDefault();
    failure = '';
    const edit = resultChanges(result, draft, timeZone);
    errors = {
      value: edit.problems.value ? m.value_invalid_number() : undefined,
      time: edit.problems.measuredAt ? m.tests_time_invalid() : undefined
    };
    if (Object.values(errors).some(Boolean)) return focusFirstInvalid(form);
    if (Object.keys(edit.changes).length === 0) {
      editing = false;
      return;
    }

    busy = true;
    try {
      const saved = await paramsApi.updateResult(result.id, edit.changes);
      editing = false;
      onupdated(saved);
    } catch (e) {
      errors = {
        value: fieldText(e, 'value'),
        note: fieldText(e, 'note'),
        time: fieldText(e, 'measured_at')
      };
      if (!Object.values(errors).some(Boolean)) failure = errorText(e);
      await focusFirstInvalid(form);
    } finally {
      busy = false;
    }
  }

  async function remove() {
    failure = '';
    try {
      await paramsApi.removeResult(result.id);
      ondeleted(result.id);
    } catch (e) {
      failure = errorText(e);
    }
  }

  const shownValue = $derived(formatValue(result.value, result.unit_label, decimals));
</script>

<li class="card item" class:out={!result.in_range}>
  {#if editing}
    <form class="stack" novalidate bind:this={form} onsubmit={save} use:focusFirstField>
      <h3>{result.parameter_name}</h3>
      {#if failure}
        <Alert kind="error">{failure}</Alert>
      {/if}
      <ValueInput
        label={m.tests_value_for({ name: result.parameter_name })}
        bind:value={draft.value}
        unit={result.unit_label}
        error={errors.value}
      />
      <TextField
        label={m.tests_time()}
        type="datetime-local"
        bind:value={draft.measuredAt}
        error={errors.time}
      />
      <TextField
        label={m.tests_note()}
        bind:value={draft.note}
        error={errors.note}
        autocomplete="off"
      />
      <div class="row">
        <button class="btn primary" type="submit" disabled={busy}>{m.form_save()}</button>
        <button class="btn" type="button" disabled={busy} onclick={() => (editing = false)}>
          {m.form_cancel()}
        </button>
      </div>
    </form>
  {:else}
    <div class="summary">
      <div class="what">
        <span class="name">{result.parameter_name}</span>
        <span class="value">{shownValue}</span>
        {#if !result.in_range}<span class="badge warning">{m.tests_out_of_range()}</span>{/if}
      </div>
      <p class="muted when">{formatDateTime(result.measured_at, timeZone)}</p>
      {#if result.note}<p class="note">{result.note}</p>{/if}
    </div>
    {#if failure}
      <Alert kind="error">{failure}</Alert>
    {/if}
    <div class="row">
      <button type="button" class="btn" onclick={startEditing}>
        {m.form_edit()}<span class="sr-only"> {result.parameter_name} {shownValue}</span>
      </button>
      <ConfirmButton
        label={m.form_delete()}
        question={m.tests_delete_question({ name: result.parameter_name, value: shownValue })}
        confirmLabel={m.form_delete()}
        onconfirm={remove}
      />
    </div>
  {/if}
</li>

<style>
  .item {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    margin: 0;
  }

  .item.out {
    border-left: 4px solid var(--warning);
  }

  .summary p {
    margin: 0;
  }

  .what {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: var(--space-2);
  }

  .name {
    font-weight: 600;
  }

  .value {
    font-variant-numeric: tabular-nums;
  }

  .item.out .value {
    color: var(--warning);
    font-weight: 600;
  }

  .note {
    overflow-wrap: anywhere;
  }
</style>

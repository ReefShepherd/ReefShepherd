<script lang="ts">
  import { untrack } from 'svelte';
  import { errorText } from '#lib/api/error-text.js';
  import { fieldText } from '#lib/api/field-errors.js';
  import { paramsApi, type TankParameter, type TestResult } from '#lib/api/params.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import TextField from '#lib/components/forms/TextField.svelte';
  import ValueInput from '#lib/components/forms/ValueInput.svelte';
  import { focusFirstInvalid } from '#lib/dom.js';
  import { formatValue } from '#lib/format/number.js';
  import { fromLocalInput, toLocalInput } from '#lib/format/local-time.js';
  import { relativeTime } from '#lib/format/datetime.js';
  import { m } from '#lib/paraglide/messages.js';
  import { buildEntry, rowErrors, type EntryRow } from '#lib/results/entry.js';

  interface Props {
    tankId: number;
    /** The parameters switched on for the tank, in display order. */
    parameters: readonly TankParameter[];
    /** The instance timezone the date and time are entered in. */
    timeZone: string;
    onsaved: (results: TestResult[]) => void;
  }

  let { tankId, parameters, timeZone, onsaved }: Props = $props();

  const emptyRows = (): EntryRow[] =>
    parameters.map((p) => ({ parameterId: p.parameter_id, value: '', note: '' }));

  let rows = $state<EntryRow[]>(emptyRows());
  const parameterKey = $derived(parameters.map((p) => p.parameter_id).join(','));
  // svelte-ignore state_referenced_locally
  let rowsFor = parameterKey;

  // The rows are rebuilt whenever the listed parameters change, so a row always belongs to a listed parameter.
  $effect.pre(() => {
    if (parameterKey === rowsFor) return;
    rowsFor = parameterKey;
    rows = untrack(emptyRows);
  });
  // svelte-ignore state_referenced_locally
  const opened = toLocalInput(new Date(), timeZone);
  let measuredAt = $state(opened);
  // Until the person changes the time it is left out of the request, so the server stamps the moment of saving.
  let shownAt = opened;
  // These three are keyed by parameter id.
  let notesOpen = $state<Record<number, boolean>>({});
  let valueErrors = $state<Record<number, string>>({});
  let noteErrors = $state<Record<number, string>>({});
  let timeError = $state<string>();
  let failure = $state('');
  let notice = $state('');
  let busy = $state(false);
  let form = $state<HTMLFormElement>();

  function clearErrors() {
    valueErrors = {};
    noteErrors = {};
    timeError = undefined;
    failure = '';
    notice = '';
  }

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    clearErrors();

    let instant: string | null = null;
    if (measuredAt !== shownAt) {
      instant = fromLocalInput(measuredAt, timeZone);
      if (instant === null) {
        timeError = m.tests_time_invalid();
        return focusFirstInvalid(form);
      }
    }

    const entry = buildEntry(rows, instant);
    const rowIds = rows.map((r) => r.parameterId);
    for (const [index, problem] of Object.entries(entry.problems)) {
      valueErrors[rowIds[Number(index)]] =
        problem === 'invalid_number' ? m.value_invalid_number() : m.tests_value_needed();
    }
    if (Object.keys(entry.problems).length > 0) return focusFirstInvalid(form);
    if (entry.body === null) {
      failure = m.tests_nothing_entered();
      return;
    }

    busy = true;
    try {
      const saved = await paramsApi.createResults(tankId, entry.body);
      rows = emptyRows();
      notesOpen = {};
      measuredAt = toLocalInput(new Date(), timeZone);
      shownAt = measuredAt;
      notice = m.tests_saved({ count: saved.length });
      onsaved(saved);
    } catch (e) {
      const mapped = rowErrors(e, entry.sent);
      for (const [index, errs] of Object.entries(mapped)) {
        if (errs.value) valueErrors[rowIds[Number(index)]] = errs.value;
        if (errs.note) noteErrors[rowIds[Number(index)]] = errs.note;
      }
      timeError = fieldText(e, 'measured_at');
      if (Object.keys(mapped).length === 0 && timeError === undefined) failure = errorText(e);
      await focusFirstInvalid(form);
    } finally {
      busy = false;
    }
  }
</script>

<form class="stack" novalidate bind:this={form} onsubmit={submit}>
  {#if failure}
    <Alert kind="error">{failure}</Alert>
  {/if}
  {#if notice}
    <Alert kind="success">{notice}</Alert>
  {/if}

  <TextField
    label={m.tests_time()}
    type="datetime-local"
    bind:value={measuredAt}
    error={timeError}
  />

  <ul class="rows">
    {#each parameters as parameter (parameter.parameter_id)}
      {@const row = rows.find((r) => r.parameterId === parameter.parameter_id)}
      {#if row}
        <li class="card row-card">
          <div class="head">
            <h3>{parameter.name}</h3>
            {#if parameter.latest}
              <p class="hint">
                {m.tests_last({
                  value: formatValue(
                    parameter.latest.value,
                    parameter.unit_label,
                    parameter.decimals
                  ),
                  time: relativeTime(parameter.latest.measured_at)
                })}
                {#if !parameter.latest.in_range}
                  <span class="badge warning">{m.tests_out_of_range()}</span>
                {/if}
              </p>
            {/if}
          </div>
          <ValueInput
            label={m.tests_value_for({ name: parameter.name })}
            hideLabel
            bind:value={row.value}
            unit={parameter.unit_label}
            error={valueErrors[parameter.parameter_id]}
          />
          {#if notesOpen[parameter.parameter_id] || row.note !== '' || noteErrors[parameter.parameter_id]}
            <TextField
              label={m.tests_note_for({ name: parameter.name })}
              bind:value={row.note}
              error={noteErrors[parameter.parameter_id]}
              autocomplete="off"
            />
          {:else}
            <button
              type="button"
              class="btn note"
              onclick={() => (notesOpen[parameter.parameter_id] = true)}
            >
              {m.tests_add_note()}<span class="sr-only"> {parameter.name}</span>
            </button>
          {/if}
        </li>
      {/if}
    {/each}
  </ul>

  <div>
    <button class="btn primary" type="submit" disabled={busy}>{m.tests_save()}</button>
  </div>
</form>

<style>
  .rows {
    display: grid;
    gap: var(--space-3);
    margin: 0;
    padding: 0;
    list-style: none;
    grid-template-columns: minmax(0, 1fr);
  }

  .row-card {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    margin: 0;
  }

  .head h3 {
    margin: 0;
  }

  .head p {
    margin: 0;
  }

  .note {
    align-self: flex-start;
    font-weight: 400;
  }

  @media (min-width: 768px) {
    .rows {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }
</style>

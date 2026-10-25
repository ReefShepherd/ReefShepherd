<script lang="ts">
  import { untrack } from 'svelte';
  import { paramsApi, type TankParameter, type TestResult } from '#lib/api/params.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import TextField from '#lib/components/forms/TextField.svelte';
  import ValueInput from '#lib/components/forms/ValueInput.svelte';
  import { focusFirstInvalid } from '#lib/dom.js';
  import { formatValue } from '#lib/format/number.js';
  import { fromLocalInput, toLocalInput } from '#lib/format/local-time.js';
  import { relativeTime } from '#lib/format/datetime.js';
  import { hasErrors, type FieldErrors } from '#lib/forms/codes.js';
  import { guarded } from '#lib/forms/guarded.js';
  import { fieldTexts } from '#lib/forms/serverErrors.js';
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
  // These are keyed by parameter id.
  let notesOpen = $state<Record<number, boolean>>({});
  let rowErrs = $state<Record<number, FieldErrors>>({});
  const rowTexts = $derived(
    Object.fromEntries(Object.entries(rowErrs).map(([id, errs]) => [id, fieldTexts(errs)]))
  );
  let timeErrors = $state<FieldErrors>({});
  const timeText = $derived(fieldTexts(timeErrors).measured_at);
  let failure = $state('');
  let notice = $state('');
  let busy = $state(false);
  let form = $state<HTMLFormElement>();

  function clearErrors() {
    rowErrs = {};
    timeErrors = {};
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
        timeErrors = { measured_at: 'invalid' };
        return focusFirstInvalid(form);
      }
    }

    const entry = buildEntry(rows, instant);
    const rowIds = rows.map((r) => r.parameterId);
    for (const [index, errs] of Object.entries(entry.problems)) {
      rowErrs[rowIds[Number(index)]] = errs;
    }
    if (hasErrors(entry.problems)) return focusFirstInvalid(form);
    if (entry.body === null) {
      failure = m.tests_nothing_entered();
      return;
    }
    const body = entry.body;

    await guarded(
      async () => {
        const saved = await paramsApi.createResults(tankId, body);
        rows = emptyRows();
        notesOpen = {};
        measuredAt = toLocalInput(new Date(), timeZone);
        shownAt = measuredAt;
        notice = m.tests_saved({ count: saved.length });
        onsaved(saved);
      },
      {
        failure: (text) => (failure = text),
        busy: (b) => (busy = b),
        fields: (fields) => {
          for (const [index, errs] of Object.entries(rowErrors(fields, entry.sent))) {
            rowErrs[rowIds[Number(index)]] = errs;
          }
          timeErrors = { measured_at: fields.measured_at };
        },
        shown: /^(measured_at|results\.\d+\.(value|note|parameter_id))$/,
        form: () => form
      }
    );
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
    error={timeText}
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
            error={rowTexts[parameter.parameter_id]?.value ??
              rowTexts[parameter.parameter_id]?.value_missing ??
              rowTexts[parameter.parameter_id]?.parameter_id}
          />
          {#if notesOpen[parameter.parameter_id] || row.note !== '' || rowTexts[parameter.parameter_id]?.note}
            <TextField
              label={m.tests_note_for({ name: parameter.name })}
              bind:value={row.note}
              error={rowTexts[parameter.parameter_id]?.note}
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

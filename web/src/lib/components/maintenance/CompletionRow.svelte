<script lang="ts">
  import { maintenanceApi, type Completion } from '#lib/api/maintenance.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import ConfirmButton from '#lib/components/forms/ConfirmButton.svelte';
  import SelectField from '#lib/components/forms/SelectField.svelte';
  import TextField from '#lib/components/forms/TextField.svelte';
  import { focusFirstField, focusFirstInvalid } from '#lib/dom.js';
  import { formatDateTime } from '#lib/format/datetime.js';
  import { hasErrors, type FieldErrors } from '#lib/forms/codes.js';
  import { guarded, type GuardHooks } from '#lib/forms/guarded.js';
  import { fieldTexts } from '#lib/forms/serverErrors.js';
  import { completionChanges, draftFromCompletion, outcomes } from '#lib/maintenance/completion.js';
  import { m } from '#lib/paraglide/messages.js';

  interface Props {
    completion: Completion;
    timeZone: string;
    onupdated: (completion: Completion) => void;
    ondeleted: (id: number) => void;
  }

  let { completion, timeZone, onupdated, ondeleted }: Props = $props();

  let editing = $state(false);
  // The draft is rebuilt from the saved entry each time editing starts.
  // svelte-ignore state_referenced_locally
  let draft = $state(draftFromCompletion(completion, timeZone));
  let errors = $state<FieldErrors>({});
  const texts = $derived(fieldTexts(errors));
  let failure = $state('');
  let busy = $state(false);
  let form = $state<HTMLFormElement>();

  const outcomeLabels = { done: m.maintenance_done, skipped: m.maintenance_skipped };
  const when = $derived(formatDateTime(completion.done_at, timeZone));

  function startEditing() {
    draft = draftFromCompletion(completion, timeZone);
    errors = {};
    failure = '';
    editing = true;
  }

  const hooks: GuardHooks = {
    failure: (text) => (failure = text),
    busy: (b) => (busy = b),
    fields: (f) => (errors = f),
    shown: ['done_at', 'outcome', 'note'],
    form: () => form
  };

  async function save(event: SubmitEvent) {
    event.preventDefault();
    failure = '';
    const edit = completionChanges(completion, draft, timeZone, new Date());
    errors = edit.errors;
    if (hasErrors(errors)) return focusFirstInvalid(form);
    if (Object.keys(edit.changes).length === 0) {
      editing = false;
      return;
    }
    await guarded(async () => {
      const saved = await maintenanceApi.updateCompletion(completion.id, edit.changes);
      editing = false;
      onupdated(saved);
    }, hooks);
  }

  const remove = () =>
    guarded(
      async () => {
        await maintenanceApi.removeCompletion(completion.id);
        ondeleted(completion.id);
      },
      { failure: (text) => (failure = text) }
    );
</script>

<li class="card item" class:skipped={completion.outcome === 'skipped'}>
  {#if editing}
    <form class="stack" novalidate bind:this={form} onsubmit={save} use:focusFirstField>
      {#if failure}
        <Alert kind="error">{failure}</Alert>
      {/if}
      <TextField
        label={m.maintenance_when()}
        type="datetime-local"
        bind:value={draft.doneAt}
        error={texts.done_at}
      />
      <SelectField
        label={m.maintenance_outcome()}
        bind:value={draft.outcome}
        options={outcomes.map((o) => ({ value: o, label: outcomeLabels[o]() }))}
        error={texts.outcome}
      />
      <TextField
        label={m.maintenance_note()}
        bind:value={draft.note}
        error={texts.note}
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
        <span class="badge" class:accent={completion.outcome === 'done'}>
          {outcomeLabels[completion.outcome]()}
        </span>
        <span class="when">{when}</span>
        {#if completion.done_by_name}
          <span class="muted">{m.maintenance_by({ name: completion.done_by_name })}</span>
        {/if}
      </div>
      {#if completion.note}<p class="note">{completion.note}</p>{/if}
    </div>
    {#if failure}
      <Alert kind="error">{failure}</Alert>
    {/if}
    <div class="row">
      <button type="button" class="btn" onclick={startEditing}>
        {m.form_edit()}<span class="sr-only"> {when}</span>
      </button>
      <ConfirmButton
        label={m.form_delete()}
        ariaLabel={m.maintenance_completion_delete_named({ when })}
        question={m.maintenance_completion_delete_question({ when })}
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

  .summary p {
    margin: 0;
  }

  .what {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: var(--space-2);
  }

  .when {
    font-variant-numeric: tabular-nums;
  }

  .note {
    overflow-wrap: anywhere;
  }
</style>

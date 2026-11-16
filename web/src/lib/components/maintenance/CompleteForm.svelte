<script lang="ts">
  import {
    maintenanceApi,
    type CompletionOutcome,
    type CompletionResult,
    type Task
  } from '#lib/api/maintenance.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import TextField from '#lib/components/forms/TextField.svelte';
  import { focusFirstField, focusFirstInvalid } from '#lib/dom.js';
  import { hasErrors, type FieldErrors } from '#lib/forms/codes.js';
  import { guarded, type GuardHooks } from '#lib/forms/guarded.js';
  import { fieldTexts } from '#lib/forms/serverErrors.js';
  import { completionBody, newCompletionDraft } from '#lib/maintenance/completion.js';
  import { m } from '#lib/paraglide/messages.js';

  interface Props {
    task: Task;
    /** Whether the entry marks the task done or skips it this time. */
    outcome: CompletionOutcome;
    timeZone: string;
    onsaved: (result: CompletionResult) => void;
    oncancel: () => void;
  }

  let { task, outcome, timeZone, onsaved, oncancel }: Props = $props();

  // svelte-ignore state_referenced_locally
  let draft = $state(newCompletionDraft(new Date(), timeZone));
  // Until the person changes the time it is left out of the request, so the server stamps the moment of saving.
  const opened = draft.doneAt;
  let errors = $state<FieldErrors>({});
  const texts = $derived(fieldTexts(errors));
  let failure = $state('');
  let busy = $state(false);
  let form = $state<HTMLFormElement>();

  const hooks: GuardHooks = {
    failure: (text) => (failure = text),
    busy: (b) => (busy = b),
    fields: (f) => (errors = f),
    shown: ['done_at', 'note', 'outcome'],
    form: () => form
  };

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    failure = '';
    const entry = completionBody(draft, opened, outcome, timeZone, new Date());
    errors = entry.errors;
    const body = entry.body;
    if (hasErrors(errors) || body === null) return focusFirstInvalid(form);
    await guarded(async () => onsaved(await maintenanceApi.complete(task.id, body)), hooks);
  }
</script>

<form class="stack" novalidate bind:this={form} onsubmit={submit} use:focusFirstField>
  <h3>
    {outcome === 'done'
      ? m.maintenance_complete_title_done({ name: task.name })
      : m.maintenance_complete_title_skip({ name: task.name })}
  </h3>
  {#if failure}
    <Alert kind="error">{failure}</Alert>
  {/if}
  <TextField
    label={m.maintenance_when()}
    type="datetime-local"
    bind:value={draft.doneAt}
    error={texts.done_at}
  />
  <TextField
    label={m.maintenance_note()}
    bind:value={draft.note}
    error={texts.note}
    autocomplete="off"
  />
  <div class="row">
    <button class="btn primary" type="submit" disabled={busy}>
      {outcome === 'done' ? m.maintenance_confirm_done() : m.maintenance_confirm_skip()}
    </button>
    <button class="btn" type="button" disabled={busy} onclick={oncancel}>
      {m.form_cancel()}
    </button>
  </div>
</form>

<style>
  h3 {
    margin: 0;
  }
</style>

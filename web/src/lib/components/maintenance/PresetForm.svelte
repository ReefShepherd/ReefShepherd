<script lang="ts">
  import { maintenanceApi, type Preset } from '#lib/api/maintenance.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import ScheduleEditor from '#lib/components/forms/ScheduleEditor.svelte';
  import TextAreaField from '#lib/components/forms/TextAreaField.svelte';
  import TextField from '#lib/components/forms/TextField.svelte';
  import { focusFirstField, focusFirstInvalid } from '#lib/dom.js';
  import { hasErrors, type FieldErrors } from '#lib/forms/codes.js';
  import { guarded, type GuardHooks } from '#lib/forms/guarded.js';
  import { fieldTexts } from '#lib/forms/serverErrors.js';
  import {
    blankPresetDraft,
    descriptionMax,
    draftFromPresetRecord,
    nameMax,
    newPreset,
    presetChanges
  } from '#lib/maintenance/draft.js';
  import { m } from '#lib/paraglide/messages.js';
  import { scheduleKinds } from '#lib/schedule/schedule.js';

  interface Props {
    /** The template being edited; without it the form adds a new one. */
    preset?: Preset;
    /** The instance reminder time, which a new template's schedule starts at. */
    dueTime: string;
    onsaved: (preset: Preset) => void;
    oncancel: () => void;
  }

  let { preset, dueTime, onsaved, oncancel }: Props = $props();

  // svelte-ignore state_referenced_locally
  let draft = $state(preset ? draftFromPresetRecord(preset) : blankPresetDraft(dueTime));
  let errors = $state<FieldErrors>({});
  const texts = $derived(fieldTexts(errors, { name: nameMax, description: descriptionMax }));
  let showScheduleErrors = $state(false);
  let failure = $state('');
  let busy = $state(false);
  let form = $state<HTMLFormElement>();

  const hooks: GuardHooks = {
    failure: (text) => (failure = text),
    busy: (b) => (busy = b),
    fields: (f) => (errors = f),
    shown: ['name', 'description', 'tankless', 'schedule', 'schedule.kind'],
    form: () => form
  };

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    failure = '';
    showScheduleErrors = true;

    if (preset) {
      const current = preset;
      const edit = presetChanges(current, draft);
      errors = edit.errors;
      if (hasErrors(errors) || edit.schedule.length > 0) return focusFirstInvalid(form);
      if (Object.keys(edit.changes).length === 0) return oncancel();
      await guarded(
        async () => onsaved(await maintenanceApi.updatePreset(current.id, edit.changes)),
        hooks
      );
      return;
    }

    const entry = newPreset(draft);
    errors = entry.errors;
    const body = entry.body;
    if (hasErrors(errors) || entry.schedule.length > 0 || body === null) {
      return focusFirstInvalid(form);
    }
    await guarded(async () => onsaved(await maintenanceApi.createPreset(body)), hooks);
  }
</script>

<form class="card stack" novalidate bind:this={form} onsubmit={submit} use:focusFirstField>
  <h2>
    {preset ? m.maintenance_template_edit({ name: preset.name }) : m.maintenance_template_new()}
  </h2>
  {#if failure}
    <Alert kind="error">{failure}</Alert>
  {/if}
  <TextField
    label={m.maintenance_name()}
    bind:value={draft.name}
    error={texts.name}
    autocomplete="off"
  />
  <TextAreaField
    label={m.maintenance_description()}
    bind:value={draft.description}
    error={texts.description}
  />
  <label class="choice">
    <input type="checkbox" bind:checked={draft.tankless} />
    <span class="choice-text">
      <span>{m.maintenance_template_tankless()}</span>
      <span class="hint">{m.maintenance_template_tankless_hint()}</span>
    </span>
  </label>
  <ScheduleEditor
    bind:value={draft.schedule}
    allowedKinds={scheduleKinds}
    showTime={false}
    showErrors={showScheduleErrors}
    error={texts.schedule ?? texts['schedule.kind']}
  />
  <div class="row">
    <button class="btn primary" type="submit" disabled={busy}>
      {preset ? m.form_save() : m.maintenance_template_add()}
    </button>
    <button class="btn" type="button" disabled={busy} onclick={oncancel}>
      {m.form_cancel()}
    </button>
  </div>
</form>

<style>
  h2 {
    margin: 0;
  }
</style>

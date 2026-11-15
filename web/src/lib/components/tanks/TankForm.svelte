<script lang="ts">
  import type { ParameterDefinition } from '#lib/api/params.js';
  import { tanksApi, tankTypes, type Tank } from '#lib/api/tanks.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import SelectField from '#lib/components/forms/SelectField.svelte';
  import TextAreaField from '#lib/components/forms/TextAreaField.svelte';
  import TextField from '#lib/components/forms/TextField.svelte';
  import ValueInput from '#lib/components/forms/ValueInput.svelte';
  import { focusFirstField, focusFirstInvalid } from '#lib/dom.js';
  import { hasErrors, type FieldErrors } from '#lib/forms/codes.js';
  import { guarded, type GuardHooks } from '#lib/forms/guarded.js';
  import { fieldTexts } from '#lib/forms/serverErrors.js';
  import { m } from '#lib/paraglide/messages.js';
  import {
    draftFromTank,
    newTankBody,
    tankChanges,
    tankNameMax,
    tankNoteMax,
    validateTank
  } from '#lib/tank/form.js';
  import PresetChecklist from './PresetChecklist.svelte';
  import { tankTypeLabel } from './labels.js';

  interface Props {
    /** The tank being edited; without one the form creates a tank. */
    tank?: Tank;
    /** The built-in parameters offered when creating a tank. */
    presets?: readonly ParameterDefinition[];
    /** The label of the volume unit the instance uses. */
    volumeUnit: string;
    onsaved: (tank: Tank) => void;
    oncancel?: () => void;
  }

  let { tank, presets = [], volumeUnit, onsaved, oncancel }: Props = $props();

  // The draft starts from the tank as it was when the form opened.
  // svelte-ignore state_referenced_locally
  let draft = $state(draftFromTank(tank));
  // Every preset starts switched on, since most people track the basics.
  // svelte-ignore state_referenced_locally
  let chosen = $state<string[]>(presets.flatMap((p) => (p.preset_key ? [p.preset_key] : [])));
  let errors = $state<FieldErrors>({});
  const texts = $derived(fieldTexts(errors, { name: tankNameMax, note: tankNoteMax }));
  let failure = $state('');
  let busy = $state(false);
  let form = $state<HTMLFormElement>();

  const hooks: GuardHooks = {
    failure: (text) => (failure = text),
    busy: (b) => (busy = b),
    fields: (f) => (errors = f),
    shown: ['name', 'volume', 'note', 'started_on', 'presets'],
    form: () => form
  };

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    failure = '';
    errors = validateTank(draft);
    if (hasErrors(errors)) return focusFirstInvalid(form);

    await guarded(async () => {
      let saved: Tank;
      if (tank) {
        const changes = tankChanges(tank, draft);
        saved = Object.keys(changes).length === 0 ? tank : await tanksApi.update(tank.id, changes);
      } else {
        saved = await tanksApi.create(newTankBody(draft, chosen));
      }
      onsaved(saved);
    }, hooks);
  }
</script>

<form class="card stack" novalidate bind:this={form} onsubmit={submit} use:focusFirstField>
  {#if failure}
    <Alert kind="error">{failure}</Alert>
  {/if}
  <TextField
    label={m.tanks_name()}
    bind:value={draft.name}
    error={texts.name}
    autocomplete="off"
    maxlength={tankNameMax}
  />
  <SelectField
    label={m.tanks_type()}
    bind:value={draft.tankType}
    options={tankTypes.map((t) => ({ value: t, label: tankTypeLabel(t) }))}
  />
  <ValueInput
    label={m.tanks_volume()}
    bind:value={draft.volume}
    unit={volumeUnit}
    error={texts.volume}
    hint={m.tanks_volume_hint()}
  />
  <TextField
    label={m.tanks_started_on()}
    type="date"
    bind:value={draft.startedOn}
    error={texts.started_on}
  />
  <TextAreaField label={m.tanks_note()} bind:value={draft.note} error={texts.note} />

  {#if !tank}
    <PresetChecklist {presets} bind:chosen error={texts.presets} />
  {/if}

  <div class="row">
    <button class="btn primary" type="submit" disabled={busy}>
      {tank ? m.form_save() : m.tanks_create()}
    </button>
    {#if oncancel}
      <button class="btn" type="button" disabled={busy} onclick={oncancel}>{m.form_cancel()}</button
      >
    {/if}
  </div>
</form>

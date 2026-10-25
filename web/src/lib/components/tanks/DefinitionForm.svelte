<script lang="ts">
  import { paramsApi, type ParameterDefinition } from '#lib/api/params.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import SelectField from '#lib/components/forms/SelectField.svelte';
  import TextField from '#lib/components/forms/TextField.svelte';
  import ValueInput from '#lib/components/forms/ValueInput.svelte';
  import { focusFirstField, focusFirstInvalid } from '#lib/dom.js';
  import { hasErrors, type FieldErrors } from '#lib/forms/codes.js';
  import { guarded, type GuardHooks } from '#lib/forms/guarded.js';
  import { fieldTexts } from '#lib/forms/serverErrors.js';
  import { m } from '#lib/paraglide/messages.js';
  import {
    definitionChanges,
    draftFromDefinition,
    maxTestInterval,
    newParameterBody,
    parameterNameMax,
    unitLabelMax,
    validateDefinition
  } from '#lib/tank/definition-form.js';

  interface Props {
    /** The parameter being edited; without one the form creates a custom parameter. */
    definition?: ParameterDefinition;
    onsaved: (definition: ParameterDefinition) => void;
    oncancel: () => void;
  }

  let { definition, onsaved, oncancel }: Props = $props();

  // svelte-ignore state_referenced_locally
  let draft = $state(draftFromDefinition(definition));
  let decimals = $state(String(draft.decimals));
  let errors = $state<FieldErrors>({});
  const texts = $derived(
    fieldTexts(errors, {
      name: parameterNameMax,
      unit_label: unitLabelMax,
      default_test_interval_days: maxTestInterval
    })
  );
  let failure = $state('');
  let busy = $state(false);
  let form = $state<HTMLFormElement>();

  const decimalOptions = ['0', '1', '2', '3', '4'].map((d) => ({ value: d, label: d }));

  const hooks: GuardHooks = {
    failure: (text) => (failure = text),
    busy: (b) => (busy = b),
    fields: (f) => (errors = f),
    shown: ['name', 'unit_label', 'default_low', 'default_high', 'default_test_interval_days'],
    form: () => form
  };

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    failure = '';
    draft.decimals = Number(decimals);
    errors = validateDefinition(draft);
    if (hasErrors(errors)) return focusFirstInvalid(form);

    await guarded(async () => {
      let saved: ParameterDefinition;
      if (definition) {
        const changes = definitionChanges(definition, draft);
        saved =
          Object.keys(changes).length === 0
            ? definition
            : await paramsApi.updateDefinition(definition.id, changes);
      } else {
        saved = await paramsApi.createDefinition(newParameterBody(draft));
      }
      onsaved(saved);
    }, hooks);
  }
</script>

<form class="card stack" novalidate bind:this={form} onsubmit={submit} use:focusFirstField>
  <h2>{definition ? m.params_def_edit_title() : m.params_def_add_title()}</h2>
  {#if failure}
    <Alert kind="error">{failure}</Alert>
  {/if}
  <TextField
    label={m.params_def_name()}
    bind:value={draft.name}
    error={texts.name}
    autocomplete="off"
    maxlength={parameterNameMax}
  />
  <TextField
    label={m.params_def_unit()}
    hint={m.params_def_unit_hint()}
    bind:value={draft.unitLabel}
    error={texts.unit_label}
    autocomplete="off"
    maxlength={unitLabelMax}
  />
  <SelectField
    label={m.params_def_decimals()}
    hint={m.params_def_decimals_hint()}
    bind:value={decimals}
    options={decimalOptions}
  />
  <div class="grid">
    <ValueInput
      label={m.params_def_low()}
      bind:value={draft.low}
      unit={draft.unitLabel.trim()}
      error={texts.default_low}
    />
    <ValueInput
      label={m.params_def_high()}
      bind:value={draft.high}
      unit={draft.unitLabel.trim()}
      error={texts.default_high ?? texts.range}
    />
  </div>
  <TextField
    label={m.params_def_interval()}
    hint={m.params_def_interval_hint()}
    bind:value={draft.intervalDays}
    error={texts.default_test_interval_days}
    inputmode="numeric"
    autocomplete="off"
  />
  <div class="row">
    <button class="btn primary" type="submit" disabled={busy}>
      {definition ? m.form_save() : m.params_def_create()}
    </button>
    <button class="btn" type="button" disabled={busy} onclick={oncancel}>{m.form_cancel()}</button>
  </div>
</form>

<style>
  .grid {
    display: grid;
    gap: var(--space-3);
    grid-template-columns: repeat(auto-fit, minmax(9rem, 1fr));
  }
</style>

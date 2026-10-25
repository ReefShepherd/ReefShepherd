<script lang="ts">
  import { errorText } from '#lib/api/error-text.js';
  import { fieldText } from '#lib/api/field-errors.js';
  import { paramsApi, type ParameterDefinition } from '#lib/api/params.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import SelectField from '#lib/components/forms/SelectField.svelte';
  import TextField from '#lib/components/forms/TextField.svelte';
  import ValueInput from '#lib/components/forms/ValueInput.svelte';
  import { focusFirstField, focusFirstInvalid } from '#lib/dom.js';
  import { m } from '#lib/paraglide/messages.js';
  import {
    definitionChanges,
    definitionProblems,
    draftFromDefinition,
    maxTestInterval,
    newParameterBody,
    parameterNameMax,
    unitLabelMax
  } from '#lib/tank/definition-form.js';

  interface Props {
    /** The parameter being edited; without one the form creates a custom parameter. */
    definition?: ParameterDefinition;
    onsaved: (definition: ParameterDefinition) => void;
    oncancel: () => void;
  }

  let { definition, onsaved, oncancel }: Props = $props();

  type Field = 'name' | 'unitLabel' | 'low' | 'high' | 'intervalDays';

  // svelte-ignore state_referenced_locally
  let draft = $state(draftFromDefinition(definition));
  let decimals = $state(String(draft.decimals));
  let errors = $state<Partial<Record<Field, string>>>({});
  let failure = $state('');
  let busy = $state(false);
  let form = $state<HTMLFormElement>();

  const decimalOptions = ['0', '1', '2', '3', '4'].map((d) => ({ value: d, label: d }));

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    failure = '';
    draft.decimals = Number(decimals);
    const p = definitionProblems(draft);
    errors = {
      name:
        p.name === 'required'
          ? m.validation_required()
          : p.name === 'too_long'
            ? m.validation_name_long({ max: parameterNameMax })
            : undefined,
      unitLabel: p.unitLabel ? m.validation_name_long({ max: unitLabelMax }) : undefined,
      low: p.low ? m.value_invalid_number() : undefined,
      high: p.high ? m.value_invalid_number() : p.range ? m.params_range_order() : undefined,
      intervalDays: p.intervalDays
        ? m.params_def_interval_invalid({ max: maxTestInterval })
        : undefined
    };
    if (Object.values(errors).some(Boolean)) return focusFirstInvalid(form);

    busy = true;
    try {
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
    } catch (e) {
      errors = {
        name: fieldText(e, 'name'),
        unitLabel: fieldText(e, 'unit_label'),
        low: fieldText(e, 'default_low'),
        high: fieldText(e, 'default_high'),
        intervalDays: fieldText(e, 'default_test_interval_days')
      };
      if (!Object.values(errors).some(Boolean)) failure = errorText(e);
      await focusFirstInvalid(form);
    } finally {
      busy = false;
    }
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
    error={errors.name}
    autocomplete="off"
    maxlength={parameterNameMax}
  />
  <TextField
    label={m.params_def_unit()}
    hint={m.params_def_unit_hint()}
    bind:value={draft.unitLabel}
    error={errors.unitLabel}
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
      error={errors.low}
    />
    <ValueInput
      label={m.params_def_high()}
      bind:value={draft.high}
      unit={draft.unitLabel.trim()}
      error={errors.high}
    />
  </div>
  <TextField
    label={m.params_def_interval()}
    hint={m.params_def_interval_hint()}
    bind:value={draft.intervalDays}
    error={errors.intervalDays}
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

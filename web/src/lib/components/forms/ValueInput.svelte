<script lang="ts">
  import { parseDecimal } from '#lib/format/decimal.js';
  import { m } from '#lib/paraglide/messages.js';

  interface Props {
    label: string;
    /** The text as typed; read it with parseDecimal, which accepts a decimal point or comma. */
    value?: string;
    /** Shown after the input, such as a unit. */
    unit?: string;
    error?: string;
    hint?: string;
    /** Keeps the label for screen readers only, for rows that already have a visible heading. */
    hideLabel?: boolean;
    disabled?: boolean;
    placeholder?: string;
  }

  let {
    label,
    value = $bindable(''),
    unit,
    error,
    hint,
    hideLabel = false,
    disabled = false,
    placeholder
  }: Props = $props();

  const uid = $props.id();
  let touched = $state(false);

  // A number that cannot be read is flagged once the person leaves the field.
  const typed = $derived(
    touched && value.trim() !== '' && parseDecimal(value) === null
      ? m.value_invalid_number()
      : undefined
  );
  const shownError = $derived(error ?? typed);
  const describedBy = $derived(
    [hint ? `${uid}-hint` : '', shownError ? `${uid}-error` : ''].filter(Boolean).join(' ') ||
      undefined
  );
</script>

<div class="field">
  <label for={uid} class:sr-only={hideLabel}>{label}</label>
  <div class="control">
    <input
      id={uid}
      type="text"
      inputmode="decimal"
      autocomplete="off"
      {disabled}
      {placeholder}
      bind:value
      onblur={() => (touched = true)}
      aria-invalid={shownError ? 'true' : undefined}
      aria-describedby={describedBy}
    />
    {#if unit}<span class="unit">{unit}</span>{/if}
  </div>
  {#if hint}<p class="hint" id="{uid}-hint">{hint}</p>{/if}
  {#if shownError}<p class="field-error" id="{uid}-error">{shownError}</p>{/if}
</div>

<style>
  .control {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .control input {
    flex: 1;
    min-width: 0;
  }

  .unit {
    flex: none;
    color: var(--text-muted);
  }
</style>

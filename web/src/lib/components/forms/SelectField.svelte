<script lang="ts" generics="T extends string">
  interface Option {
    value: T;
    label: string;
  }

  interface Props {
    label: string;
    value: T;
    options: readonly Option[];
    disabled?: boolean;
    hint?: string;
    error?: string;
  }

  let { label, value = $bindable(), options, disabled = false, hint, error }: Props = $props();

  const uid = $props.id();
  const describedBy = $derived(
    [hint ? `${uid}-hint` : '', error ? `${uid}-error` : ''].filter(Boolean).join(' ') || undefined
  );
</script>

<div class="field">
  <label for={uid}>{label}</label>
  <select
    id={uid}
    bind:value
    {disabled}
    aria-invalid={error ? 'true' : undefined}
    aria-describedby={describedBy}
  >
    {#each options as option (option.value)}
      <option value={option.value}>{option.label}</option>
    {/each}
  </select>
  {#if hint}<p class="hint" id="{uid}-hint">{hint}</p>{/if}
  {#if error}<p class="field-error" id="{uid}-error">{error}</p>{/if}
</div>

<script lang="ts">
  import type { HTMLInputAttributes } from 'svelte/elements';

  interface Props extends Omit<HTMLInputAttributes, 'value' | 'id' | 'class'> {
    label: string;
    value?: string;
    error?: string;
    hint?: string;
  }

  let { label, value = $bindable(''), error, hint, ...rest }: Props = $props();

  const uid = $props.id();
  const describedBy = $derived(
    [hint ? `${uid}-hint` : '', error ? `${uid}-error` : ''].filter(Boolean).join(' ') || undefined
  );
</script>

<div class="field">
  <label for={uid}>{label}</label>
  <input
    id={uid}
    {...rest}
    bind:value
    aria-invalid={error ? 'true' : undefined}
    aria-describedby={describedBy}
  />
  {#if hint}<p class="hint" id="{uid}-hint">{hint}</p>{/if}
  {#if error}<p class="field-error" id="{uid}-error">{error}</p>{/if}
</div>

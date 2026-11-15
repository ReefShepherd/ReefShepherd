<script lang="ts">
  import type { HTMLTextareaAttributes } from 'svelte/elements';

  interface Props extends Omit<HTMLTextareaAttributes, 'value' | 'id' | 'class'> {
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
  <textarea
    id={uid}
    rows="3"
    {...rest}
    bind:value
    aria-invalid={error ? 'true' : undefined}
    aria-describedby={describedBy}></textarea>
  {#if hint}<p class="hint" id="{uid}-hint">{hint}</p>{/if}
  {#if error}<p class="field-error" id="{uid}-error">{error}</p>{/if}
</div>

<style>
  textarea {
    width: 100%;
    padding: var(--space-2);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg);
    color: var(--text);
    font: inherit;
    resize: vertical;
  }

  textarea[aria-invalid='true'] {
    border-color: var(--danger);
  }
</style>

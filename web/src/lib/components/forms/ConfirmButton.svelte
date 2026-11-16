<script lang="ts">
  import { tick } from 'svelte';
  import { m } from '#lib/paraglide/messages.js';

  interface Props {
    label: string;
    /** Question shown while waiting for the second click. */
    question: string;
    confirmLabel: string;
    onconfirm: () => void | Promise<void>;
    disabled?: boolean;
    /** A longer accessible name for the first button, such as one that says what is deleted. */
    ariaLabel?: string;
    /** Moves focus to the confirm button when the question opens and back to the first button when it closes, so keyboard users keep their place. */
    manageFocus?: boolean;
  }

  let {
    label,
    question,
    confirmLabel,
    onconfirm,
    disabled = false,
    ariaLabel,
    manageFocus = false
  }: Props = $props();

  let asking = $state(false);
  let busy = $state(false);
  let openButton = $state<HTMLButtonElement | null>(null);
  let confirmButton = $state<HTMLButtonElement | null>(null);

  async function setAsking(value: boolean) {
    asking = value;
    if (!manageFocus) return;
    await tick();
    (value ? confirmButton : openButton)?.focus();
  }

  async function confirm() {
    busy = true;
    try {
      await onconfirm();
    } finally {
      busy = false;
      void setAsking(false);
    }
  }
</script>

{#if asking}
  <div class="row" role="group" aria-label={question}>
    <span>{question}</span>
    <button
      type="button"
      class="btn danger"
      disabled={busy}
      bind:this={confirmButton}
      onclick={confirm}
    >
      {confirmLabel}
    </button>
    <button type="button" class="btn" disabled={busy} onclick={() => setAsking(false)}>
      {m.form_cancel()}
    </button>
  </div>
{:else}
  <button
    type="button"
    class="btn danger"
    {disabled}
    aria-label={ariaLabel}
    bind:this={openButton}
    onclick={() => setAsking(true)}
  >
    {label}
  </button>
{/if}

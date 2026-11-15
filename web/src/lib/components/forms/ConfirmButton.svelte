<script lang="ts">
  import { m } from '#lib/paraglide/messages.js';

  interface Props {
    label: string;
    /** Question shown while waiting for the second click. */
    question: string;
    confirmLabel: string;
    onconfirm: () => void | Promise<void>;
    disabled?: boolean;
  }

  let { label, question, confirmLabel, onconfirm, disabled = false }: Props = $props();

  let asking = $state(false);
  let busy = $state(false);

  async function confirm() {
    busy = true;
    try {
      await onconfirm();
    } finally {
      busy = false;
      asking = false;
    }
  }
</script>

{#if asking}
  <div class="row" role="group" aria-label={question}>
    <span>{question}</span>
    <button type="button" class="btn danger" disabled={busy} onclick={confirm}>
      {confirmLabel}
    </button>
    <button type="button" class="btn" disabled={busy} onclick={() => (asking = false)}>
      {m.form_cancel()}
    </button>
  </div>
{:else}
  <button type="button" class="btn danger" {disabled} onclick={() => (asking = true)}>
    {label}
  </button>
{/if}

<script lang="ts">
  import TextField from '#lib/components/forms/TextField.svelte';
  import type { SecretDraft } from '#lib/notifications/channel-draft.js';
  import { m } from '#lib/paraglide/messages.js';

  interface Props {
    label: string;
    secret: SecretDraft;
    hint?: string;
    error?: string;
    /** Offers to remove a saved value, for a setting that may be left blank. */
    removeLabel?: string;
  }

  let { label, secret = $bindable(), hint, error, removeLabel }: Props = $props();

  // The saved value is never shown; an empty box means it stays as it is.
  const shownHint = $derived(
    secret.stored ? [m.channel_secret_saved(), hint].filter(Boolean).join(' ') : hint
  );
</script>

<div class="stack">
  <TextField
    {label}
    type="password"
    bind:value={secret.text}
    {error}
    hint={shownHint}
    placeholder={secret.stored && !secret.remove ? m.channel_secret_unchanged() : undefined}
    disabled={secret.remove && secret.text === ''}
    autocomplete="new-password"
  />
  {#if removeLabel && secret.stored}
    <label class="choice">
      <input type="checkbox" bind:checked={secret.remove} />
      <span class="choice-text">{removeLabel}</span>
    </label>
  {/if}
</div>

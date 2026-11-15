<script lang="ts">
  import { recoveryCodesFilename, recoveryCodesText } from '#lib/auth/recovery-codes.js';
  import { downloadTextFile } from '#lib/dom.js';
  import { m } from '#lib/paraglide/messages.js';
  import { session } from '#lib/session/session.svelte.js';

  interface Props {
    codes: readonly string[];
    ondone: () => void;
  }

  let { codes, ondone }: Props = $props();

  function download() {
    const text = recoveryCodesText(codes, {
      username: session.me?.username ?? '',
      instanceName: session.me?.instance.name ?? '',
      createdAt: new Date(),
      timeZone: session.timeZone
    });
    downloadTextFile(recoveryCodesFilename, text);
  }
</script>

<div class="stack">
  <div>
    <h3>{m.totp_recovery_title()}</h3>
    <p class="hint">{m.totp_recovery_intro()}</p>
  </div>
  <ul class="codes mono" aria-label={m.totp_recovery_list_label()}>
    {#each codes as code (code)}
      <li>{code}</li>
    {/each}
  </ul>
  <div class="row">
    <button type="button" class="btn" onclick={download}>{m.totp_recovery_download()}</button>
    <button type="button" class="btn primary" onclick={ondone}>{m.totp_recovery_saved()}</button>
  </div>
</div>

<style>
  .codes {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(9rem, 1fr));
    gap: var(--space-2);
    margin: 0;
    padding: var(--space-3);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg);
    list-style: none;
    font-size: 1.1rem;
  }
</style>

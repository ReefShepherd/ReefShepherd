<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { backupsApi, type Backup } from '#lib/api/backups.js';
  import { errorText } from '#lib/api/error-text.js';
  import {
    addBackup,
    createFailureText,
    kindLabel,
    nextAfterRemoval,
    removeBackup
  } from '#lib/backups/list.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import ConfirmButton from '#lib/components/forms/ConfirmButton.svelte';
  import { formatBytes } from '#lib/format/bytes.js';
  import { formatDateTime } from '#lib/format/datetime.js';
  import { m } from '#lib/paraglide/messages.js';
  import { session } from '#lib/session/session.svelte.js';

  let backups = $state<Backup[] | null>(null);
  let failure = $state('');
  let notice = $state('');
  let creating = $state(false);

  async function load() {
    try {
      backups = await backupsApi.list();
      failure = '';
    } catch (e) {
      failure = errorText(e);
    }
  }

  onMount(load);

  async function focusId(id: string) {
    await tick();
    document.getElementById(id)?.focus();
  }

  async function create() {
    creating = true;
    failure = '';
    notice = '';
    try {
      const made = await backupsApi.create();
      // A list that never loaded is read again so the new backup does not stand in for all of them.
      if (backups === null) await load();
      else backups = addBackup(backups, made);
      notice = m.backups_created({ name: made.name });
    } catch (e) {
      failure = createFailureText(e);
    } finally {
      creating = false;
    }
  }

  async function remove(item: Backup) {
    failure = '';
    notice = '';
    const next = nextAfterRemoval(backups ?? [], item.name);
    try {
      await backupsApi.remove(item.name);
      backups = removeBackup(backups ?? [], item.name);
      notice = m.backups_deleted({ name: item.name });
      void focusId(next === null ? 'backups-list' : `backup-download-${next}`);
    } catch (e) {
      failure = errorText(e);
    }
  }
</script>

<section class="card stack" aria-labelledby="backups-list">
  <div class="head">
    <h2 id="backups-list" tabindex="-1">{m.backups_list_heading()}</h2>
    <button type="button" class="btn primary" disabled={creating} onclick={create}>
      {m.backups_now()}
    </button>
  </div>

  {#if failure}
    <Alert kind="error">{failure}</Alert>
  {/if}
  {#if notice}
    <Alert kind="success">{notice}</Alert>
  {/if}

  {#if backups === null}
    {#if !failure}<p class="muted" role="status">{m.loading()}</p>{/if}
  {:else if backups.length === 0}
    <p class="muted">{m.backups_empty()}</p>
  {:else}
    <ul class="backups">
      {#each backups as item (item.name)}
        <li>
          <div class="info">
            <p class="name">
              <strong class="mono">{item.name}</strong>
              <span class="badge" class:accent={item.kind === 'scheduled'}
                >{kindLabel(item.kind)}</span
              >
            </p>
            <p class="hint">
              {m.detail_pair({
                first: formatDateTime(item.created_at, session.timeZone),
                second: formatBytes(item.size_bytes)
              })}
            </p>
          </div>
          <div class="actions">
            <a
              id="backup-download-{item.name}"
              class="btn"
              href={backupsApi.downloadUrl(item.name)}
              download
              aria-label={m.backups_download_label({ name: item.name })}
            >
              {m.backups_download()}
            </a>
            <ConfirmButton
              label={m.backups_delete()}
              ariaLabel={m.backups_delete_label({ name: item.name })}
              question={m.backups_delete_question({ name: item.name })}
              confirmLabel={m.backups_delete_confirm()}
              manageFocus
              onconfirm={() => remove(item)}
            />
          </div>
        </li>
      {/each}
    </ul>
  {/if}
</section>

<style>
  .head {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
  }

  .head h2 {
    margin: 0;
  }

  .backups {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  li {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg);
  }

  .info {
    flex: 1 1 14rem;
    min-width: 0;
  }

  .name,
  .info .hint {
    margin: 0;
    overflow-wrap: anywhere;
  }

  .actions {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
  }
</style>

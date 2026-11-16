<script lang="ts">
  import { untrack } from 'svelte';
  import { errorText } from '#lib/api/error-text.js';
  import { maintenanceApi, type Completion } from '#lib/api/maintenance.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import {
    addCompletion,
    appendPage,
    completionQuery,
    removeCompletion,
    replaceCompletion
  } from '#lib/maintenance/paging.js';
  import { m } from '#lib/paraglide/messages.js';
  import CompletionRow from './CompletionRow.svelte';

  interface Props {
    taskId: number;
    timeZone: string;
    /** Called after an entry was edited or deleted, because the task's next due date may have moved. */
    onchanged: () => void;
  }

  let { taskId, timeZone, onchanged }: Props = $props();

  let entries = $state<Completion[] | null>(null);
  let nextCursor = $state<string | null>(null);
  let failure = $state('');
  let loadingMore = $state(false);
  // A reply for an older task is dropped, so a slow request cannot overwrite a newer list.
  let generation = 0;

  async function load(id: number) {
    const mine = ++generation;
    failure = '';
    entries = null;
    nextCursor = null;
    try {
      const page = await maintenanceApi.completions(id, completionQuery(null));
      if (mine !== generation) return;
      entries = page.items;
      nextCursor = page.next_cursor;
    } catch (e) {
      if (mine !== generation) return;
      failure = errorText(e);
      entries = [];
    }
  }

  async function loadMore() {
    if (nextCursor === null || entries === null) return;
    const mine = generation;
    loadingMore = true;
    failure = '';
    try {
      const page = await maintenanceApi.completions(taskId, completionQuery(nextCursor));
      if (mine !== generation) return;
      entries = appendPage(entries, page);
      nextCursor = page.next_cursor;
    } catch (e) {
      if (mine === generation) failure = errorText(e);
    } finally {
      loadingMore = false;
    }
  }

  $effect(() => {
    const id = taskId;
    untrack(() => void load(id));
  });

  /** Shows an entry that was just recorded at the top of the history. */
  export function add(completion: Completion): void {
    if (entries !== null) entries = addCompletion(entries, completion);
  }
</script>

<section class="stack" aria-labelledby="history-heading">
  <h2 id="history-heading">{m.maintenance_history_title()}</h2>

  {#if failure}
    <Alert kind="error">{failure}</Alert>
  {/if}

  {#if entries === null}
    <p class="muted" role="status">{m.loading()}</p>
  {:else if entries.length === 0}
    {#if !failure}<p class="muted">{m.maintenance_history_empty()}</p>{/if}
  {:else}
    <ul class="list">
      {#each entries as completion (completion.id)}
        <CompletionRow
          {completion}
          {timeZone}
          onupdated={(saved) => {
            entries = replaceCompletion(entries ?? [], saved);
            onchanged();
          }}
          ondeleted={(id) => {
            entries = removeCompletion(entries ?? [], id);
            onchanged();
          }}
        />
      {/each}
    </ul>
    {#if nextCursor !== null}
      <div>
        <button type="button" class="btn" disabled={loadingMore} onclick={loadMore}>
          {m.maintenance_load_more()}
        </button>
      </div>
    {/if}
  {/if}
</section>

<style>
  .list {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  h2 {
    margin: 0;
  }
</style>

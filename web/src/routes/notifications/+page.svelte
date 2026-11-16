<script lang="ts">
  import { untrack } from 'svelte';
  import { errorText } from '#lib/api/error-text.js';
  import { notifyApi, type Notification } from '#lib/api/notify.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import InboxItem from '#lib/components/notifications/InboxItem.svelte';
  import PageTitle from '#lib/components/shell/PageTitle.svelte';
  import {
    appendPage,
    inboxQuery,
    markAllRead,
    markedRead,
    markOneRead,
    type MarkDeps
  } from '#lib/notifications/inbox.js';
  import { unread } from '#lib/notifications/unread.svelte.js';
  import { m } from '#lib/paraglide/messages.js';
  import { session } from '#lib/session/session.svelte.js';

  const deps: MarkDeps = { markRead: (which) => notifyApi.markRead(which), unread };

  let unreadOnly = $state(false);
  let items = $state<Notification[] | null>(null);
  let nextCursor = $state<string | null>(null);
  let failure = $state('');
  let busy = $state(false);
  let loadingMore = $state(false);
  // A reply for an older filter is dropped, so a slow request cannot overwrite a newer list.
  let generation = 0;

  async function load(only: boolean) {
    const mine = ++generation;
    failure = '';
    items = null;
    nextCursor = null;
    try {
      const page = await notifyApi.notifications(inboxQuery(only, null));
      if (mine !== generation) return;
      items = page.items;
      nextCursor = page.next_cursor;
      // Opening the inbox is a good moment to bring the bell up to date.
      unread.refresh().catch(() => {});
    } catch (e) {
      if (mine !== generation) return;
      failure = errorText(e);
      items = [];
    }
  }

  async function loadMore() {
    if (nextCursor === null || items === null) return;
    const mine = generation;
    loadingMore = true;
    failure = '';
    try {
      const page = await notifyApi.notifications(inboxQuery(unreadOnly, nextCursor));
      if (mine !== generation) return;
      items = appendPage(items, page);
      nextCursor = page.next_cursor;
    } catch (e) {
      if (mine === generation) failure = errorText(e);
    } finally {
      loadingMore = false;
    }
  }

  // Changing the filter starts a new list from the newest notification.
  $effect(() => {
    const only = unreadOnly;
    untrack(() => void load(only));
  });

  async function markOne(item: Notification) {
    if (items === null) return;
    failure = '';
    busy = true;
    try {
      items = await markOneRead(deps, items, item.id);
    } catch (e) {
      failure = errorText(e);
    } finally {
      busy = false;
    }
  }

  async function markAll() {
    failure = '';
    busy = true;
    try {
      await markAllRead(deps);
      await load(unreadOnly);
    } catch (e) {
      failure = errorText(e);
    } finally {
      busy = false;
    }
  }

  // Following a link marks the notification read; the page changes either way, so a failure is not shown.
  function open(item: Notification) {
    if (item.read) return;
    if (items !== null) items = markedRead(items, [item.id]);
    markOneRead(deps, [item], item.id).catch(() => {});
  }

  const nothingUnread = $derived(unread.count === 0 && !(items ?? []).some((n) => !n.read));
</script>

<PageTitle title={m.notifications_title()} />
<h1>{m.notifications_title()}</h1>

<div class="stack">
  <div class="row bar">
    <label class="choice">
      <input type="checkbox" bind:checked={unreadOnly} />
      <span class="choice-text">{m.inbox_filter_unread()}</span>
    </label>
    <button type="button" class="btn" disabled={busy || nothingUnread} onclick={markAll}>
      {m.inbox_mark_all()}
    </button>
  </div>

  {#if failure}
    <Alert kind="error">{failure}</Alert>
  {/if}

  {#if items === null}
    <p class="muted" role="status">{m.loading()}</p>
  {:else if items.length === 0}
    {#if !failure}
      <p class="muted">{unreadOnly ? m.inbox_empty_unread() : m.inbox_empty()}</p>
    {/if}
  {:else}
    <ul class="list">
      {#each items as item (item.id)}
        <InboxItem
          {item}
          timeZone={session.timeZone}
          disabled={busy}
          onopen={open}
          onmarkread={markOne}
        />
      {/each}
    </ul>
    {#if nextCursor !== null}
      <div>
        <button type="button" class="btn" disabled={loadingMore} onclick={loadMore}>
          {m.inbox_load_more()}
        </button>
      </div>
    {/if}
  {/if}
</div>

<style>
  .bar {
    justify-content: space-between;
  }

  .list {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    margin: 0;
    padding: 0;
    list-style: none;
  }
</style>

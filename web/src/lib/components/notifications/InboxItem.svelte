<script lang="ts">
  import type { Notification } from '#lib/api/notify.js';
  import { formatDateTime } from '#lib/format/datetime.js';
  import { inboxLink } from '#lib/notifications/inbox.js';
  import { severityLabel } from '#lib/notifications/labels.js';
  import { m } from '#lib/paraglide/messages.js';

  interface Props {
    item: Notification;
    timeZone: string;
    /** Called when the link is followed, so the notification can be marked read on the way. */
    onopen: (item: Notification) => void;
    onmarkread: (item: Notification) => void;
    disabled?: boolean;
  }

  let { item, timeZone, onopen, onmarkread, disabled = false }: Props = $props();

  const link = $derived(inboxLink(item.link));
</script>

<li class="card stack" class:unread={!item.read}>
  <div class="meta">
    <span class="badge {item.severity}">{severityLabel(item.severity)}</span>
    {#if !item.read}<span class="badge accent">{m.inbox_unread()}</span>{/if}
    <time class="muted" datetime={item.created_at}>{formatDateTime(item.created_at, timeZone)}</time
    >
  </div>

  <h3>
    {#if link}
      <a href={link} onclick={() => onopen(item)}>{item.title}</a>
    {:else}
      {item.title}
    {/if}
  </h3>

  {#if item.body}<p class="body">{item.body}</p>{/if}

  {#if !item.read}
    <div>
      <button type="button" class="btn" {disabled} onclick={() => onmarkread(item)}>
        {m.inbox_mark_one()}
      </button>
    </div>
  {/if}
</li>

<style>
  li {
    margin: 0;
  }

  .unread {
    border-color: var(--accent);
  }

  .meta {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
  }

  h3 {
    margin: 0;
    overflow-wrap: anywhere;
  }

  .body {
    margin: 0;
    overflow-wrap: anywhere;
    white-space: pre-line;
  }

  .badge.warning {
    border-color: var(--warning);
    color: var(--warning);
  }

  .badge.critical {
    border-color: var(--danger);
    color: var(--danger);
  }
</style>

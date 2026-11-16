<script lang="ts">
  import { onMount } from 'svelte';
  import { m } from '#lib/paraglide/messages.js';
  import { badgeText } from '#lib/notifications/poll.js';
  import { followSession, unread } from '#lib/notifications/unread.svelte.js';
  import { session } from '#lib/session/session.svelte.js';

  const follow = followSession();
  // A derived boolean, so the effect reruns when the user signs in or out and not when the profile is refreshed.
  const signedIn = $derived(session.me !== null);
  const badge = $derived(badgeText(unread.count));

  // Counting starts once the profile is known, so a signed-out visitor is not sent to sign in by the bell.
  onMount(() => {
    session.ready().catch(() => {});
  });

  $effect(() => {
    follow.update(signedIn);
    return follow.stop;
  });
</script>

<a
  class="bell"
  href="/notifications"
  aria-label={badge ? m.bell_label_unread({ count: badge }) : m.bell_label()}
>
  <svg
    viewBox="0 0 24 24"
    width="24"
    height="24"
    fill="none"
    stroke="currentColor"
    stroke-width="1.8"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
    focusable="false"
  >
    <path d="M6 9a6 6 0 0 1 12 0c0 6 2.5 7.5 2.5 7.5h-17S6 15 6 9z" />
    <path d="M10 20a2 2 0 0 0 4 0" />
  </svg>
  {#if badge}<span class="count" aria-hidden="true">{badge}</span>{/if}
</a>

<style>
  .bell {
    position: relative;
    display: inline-flex;
    flex: none;
    align-items: center;
    justify-content: center;
    min-width: var(--touch);
    min-height: var(--touch);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg);
    color: var(--text);
  }

  .count {
    position: absolute;
    top: -0.25rem;
    right: -0.25rem;
    min-width: 1.25rem;
    padding: 0 0.25rem;
    border-radius: 999px;
    background: var(--danger);
    color: var(--accent-text);
    font-size: 0.75rem;
    font-weight: 700;
    line-height: 1.25rem;
    text-align: center;
  }
</style>

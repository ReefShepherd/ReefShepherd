<script lang="ts">
  import { goto } from '$app/navigation';
  import { authApi, type Session } from '#lib/api/auth.js';
  import { errorText } from '#lib/api/error-text.js';
  import { describeUserAgent } from '#lib/auth/user-agent.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import { formatDateTime, relativeTime } from '#lib/format/datetime.js';
  import { m } from '#lib/paraglide/messages.js';
  import { session } from '#lib/session/session.svelte.js';

  /** Changes whenever the list should be read again, such as after a password change ends other sessions. */
  let { version = 0 }: { version?: number } = $props();

  let sessions = $state<Session[] | null>(null);
  let failure = $state('');
  let busyId = $state<number | null>(null);

  async function load() {
    try {
      sessions = await authApi.sessions();
      failure = '';
    } catch (e) {
      failure = errorText(e);
    }
  }

  $effect(() => {
    void version;
    void load();
  });

  async function revoke(item: Session) {
    busyId = item.id;
    try {
      await authApi.revokeSession(item.id);
      if (item.current) {
        session.clear();
        await goto('/login');
        return;
      }
      await load();
    } catch (e) {
      failure = errorText(e);
    } finally {
      busyId = null;
    }
  }
</script>

<section class="card stack" aria-labelledby="sessions-heading">
  <div>
    <h2 id="sessions-heading">{m.sessions_title()}</h2>
    <p class="hint">{m.sessions_intro()}</p>
  </div>

  {#if failure}
    <Alert kind="error">{failure}</Alert>
  {/if}

  {#if sessions === null}
    {#if !failure}<p class="muted" role="status">{m.loading()}</p>{/if}
  {:else}
    <ul class="sessions">
      {#each sessions as item (item.id)}
        {@const device = describeUserAgent(item.user_agent)}
        <li>
          <div class="info">
            <p class="name">
              <strong>{device}</strong>
              {#if item.current}<span class="badge accent">{m.sessions_current()}</span>{/if}
            </p>
            <p class="hint">
              {#if item.ip_address}
                {m.sessions_detail_ip({
                  lastSeen: relativeTime(item.last_seen_at),
                  signedIn: formatDateTime(item.created_at, session.timeZone),
                  ip: item.ip_address
                })}
              {:else}
                {m.sessions_detail({
                  lastSeen: relativeTime(item.last_seen_at),
                  signedIn: formatDateTime(item.created_at, session.timeZone)
                })}
              {/if}
            </p>
          </div>
          <button
            type="button"
            class="btn"
            disabled={busyId !== null}
            aria-label={m.sessions_revoke_label({ device })}
            onclick={() => revoke(item)}
          >
            {m.sessions_revoke()}
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</section>

<style>
  .sessions {
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
</style>

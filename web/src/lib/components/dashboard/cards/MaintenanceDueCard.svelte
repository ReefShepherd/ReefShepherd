<script module lang="ts">
  import { m } from '#lib/paraglide/messages.js';

  export const cardType = 'maintenance_due';
  export const needsParameter = false;
  export const cardLabel = () => m.maintenance_card_title();
</script>

<script lang="ts">
  import { errorText } from '#lib/api/error-text.js';
  import { maintenanceApi, type Task } from '#lib/api/maintenance.js';
  import { tankPath } from '#lib/api/tank-id.js';
  import { formatDateTime, relativeTime } from '#lib/format/datetime.js';
  import { cardRows, dueState, replaceTask } from '#lib/maintenance/grouping.js';
  import { useDashboard } from '../context.js';

  const dash = useDashboard();

  let tasks = $state.raw<Task[] | null>(null);
  let failure = $state('');
  let busyId = $state<number | null>(null);
  let attempt = $state(0);

  $effect(() => {
    void attempt;
    const id = dash.tankId;
    const controller = new AbortController();
    tasks = null;
    failure = '';
    maintenanceApi
      .tasks({ tank_id: id, include_tankless: true, status: 'due' }, controller.signal)
      .then((list) => {
        if (!controller.signal.aborted) tasks = list;
      })
      .catch((e: unknown) => {
        if (!controller.signal.aborted) failure = errorText(e);
      });
    return () => controller.abort();
  });

  // The clock is read again whenever the list changes.
  const now = $derived.by(() => {
    void tasks;
    return new Date();
  });
  const rows = $derived(cardRows(tasks ?? [], now, dash.timeZone));

  async function markDone(task: Task) {
    busyId = task.id;
    failure = '';
    try {
      const result = await maintenanceApi.complete(task.id);
      tasks = replaceTask(tasks ?? [], result.task);
    } catch (e) {
      failure = errorText(e);
    } finally {
      busyId = null;
    }
  }
</script>

<div class="due">
  <h3>{m.maintenance_card_title()}</h3>
  {#if failure}
    <p class="problem" role="alert">{failure}</p>
  {/if}
  {#if tasks === null && !failure}
    <p class="muted" role="status">{m.loading()}</p>
  {:else if tasks === null}
    <button type="button" class="btn" onclick={() => attempt++}>{m.maintenance_retry()}</button>
  {:else}
    {#if rows.shown.length === 0}
      <p class="muted">{m.maintenance_card_none()}</p>
    {:else}
      <ul>
        {#each rows.shown as task (task.id)}
          {@const state = dueState(task, now, dash.timeZone)}
          <li>
            <div class="info">
              <span class="name">{task.name}</span>
              {#if state === 'overdue'}
                <span class="badge warning">{m.dashboard_overdue()}</span>
                <span class="muted">
                  {m.dashboard_due({ when: relativeTime(task.next_due_at ?? '') })}
                </span>
              {:else}
                <span class="badge accent">{m.dashboard_due_today()}</span>
                <span class="muted">
                  {m.dashboard_due({ when: formatDateTime(task.next_due_at ?? '', dash.timeZone) })}
                </span>
              {/if}
            </div>
            <button
              type="button"
              class="btn"
              disabled={busyId !== null}
              onclick={() => markDone(task)}
            >
              {m.maintenance_done()}<span class="sr-only"> {task.name}</span>
            </button>
          </li>
        {/each}
      </ul>
    {/if}
    <a class="more" href={tankPath(dash.tankId, 'maintenance')}>
      {rows.more > 0 ? m.maintenance_card_more({ count: rows.more }) : m.maintenance_card_open()}
    </a>
  {/if}
</div>

<style>
  .due {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    align-items: flex-start;
  }

  h3,
  p {
    margin: 0;
  }

  .problem {
    color: var(--danger);
  }

  ul {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    width: 100%;
    margin: 0;
    padding: 0;
    list-style: none;
  }

  li {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-1) var(--space-2);
  }

  .info {
    display: flex;
    flex: 1 1 10rem;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-1) var(--space-2);
    min-width: 0;
  }

  .name {
    font-weight: 600;
    overflow-wrap: anywhere;
  }

  .more {
    display: inline-flex;
    align-items: center;
    min-height: var(--touch);
  }
</style>

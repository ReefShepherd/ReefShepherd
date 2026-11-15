<script module lang="ts">
  import { m } from '#lib/paraglide/messages.js';

  export const cardType = 'tests_due';
  export const needsParameter = false;
  export const cardLabel = () => m.dashboard_tests_due_title();
</script>

<script lang="ts">
  import { tankPath } from '#lib/api/tank-id.js';
  import { formatDateTime, relativeTime } from '#lib/format/datetime.js';
  import { useDashboard } from '../context.js';
  import { classifyDue } from '../status.js';

  const dash = useDashboard();
  const due = $derived(classifyDue(dash.parameters, new Date(), dash.timeZone));
  const nothing = $derived(due.overdue.length === 0 && due.today.length === 0);
</script>

<div class="due">
  <h3>{m.dashboard_tests_due_title()}</h3>
  {#if nothing}
    <p class="muted">{m.dashboard_tests_due_none()}</p>
  {:else}
    <ul>
      {#each due.overdue as p (p.parameter_id)}
        <li>
          <span class="name">{p.name}</span>
          <span class="badge warning">{m.dashboard_overdue()}</span>
          <span class="muted">{m.dashboard_due({ when: relativeTime(p.next_due ?? '') })}</span>
        </li>
      {/each}
      {#each due.today as p (p.parameter_id)}
        <li>
          <span class="name">{p.name}</span>
          <span class="badge accent">{m.dashboard_due_today()}</span>
          <span class="muted"
            >{m.dashboard_due({ when: formatDateTime(p.next_due ?? '', dash.timeZone) })}</span
          >
        </li>
      {/each}
    </ul>
    <a class="btn" href={tankPath(dash.tankId, 'tests')}>{m.dashboard_log_now()}</a>
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
    gap: var(--space-1) var(--space-2);
  }

  .name {
    font-weight: 600;
    overflow-wrap: anywhere;
  }
</style>

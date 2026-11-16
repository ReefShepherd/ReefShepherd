<script lang="ts">
  import {
    maintenanceApi,
    type Completion,
    type CompletionOutcome,
    type Task
  } from '#lib/api/maintenance.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import ConfirmButton from '#lib/components/forms/ConfirmButton.svelte';
  import { formatDateTime } from '#lib/format/datetime.js';
  import { guarded } from '#lib/forms/guarded.js';
  import { dueState } from '#lib/maintenance/grouping.js';
  import { m } from '#lib/paraglide/messages.js';
  import { describeSchedule } from '#lib/schedule/describe.js';
  import CompleteForm from './CompleteForm.svelte';
  import TaskForm from './TaskForm.svelte';

  interface Props {
    task: Task;
    timeZone: string;
    /** On the task's own page: no link to it, and the description is shown. */
    detail?: boolean;
    /** Called with the task as the server has it after a change or after it was marked. */
    onchanged: (task: Task, completion?: Completion) => void;
    onremoved: (id: number) => void;
  }

  let { task, timeZone, detail = false, onchanged, onremoved }: Props = $props();

  type Mode = 'view' | 'edit' | CompletionOutcome;
  let mode = $state<Mode>('view');
  let failure = $state('');

  const due = $derived(dueState(task, new Date(), timeZone));

  const remove = () =>
    guarded(
      async () => {
        await maintenanceApi.removeTask(task.id);
        onremoved(task.id);
      },
      { failure: (text) => (failure = text) }
    );
</script>

<li class="item" class:overdue={due === 'overdue'} class:paused={due === 'paused'}>
  {#if mode === 'edit'}
    <TaskForm
      {task}
      {timeZone}
      onsaved={(saved) => {
        mode = 'view';
        onchanged(saved);
      }}
      oncancel={() => (mode = 'view')}
    />
  {:else}
    <div class="card body">
      <div class="summary">
        <div class="what">
          {#if detail}
            <h1 class="name">{task.name}</h1>
          {:else}
            <a class="name" href="/maintenance/{task.id}">{task.name}</a>
          {/if}
          {#if due === 'overdue'}
            <span class="badge warning">{m.dashboard_overdue()}</span>
          {:else if due === 'today'}
            <span class="badge accent">{m.dashboard_due_today()}</span>
          {:else if due === 'paused'}
            <span class="badge">{m.maintenance_paused()}</span>
          {/if}
        </div>
        {#if detail && task.tank_name !== null}
          <p class="muted">{task.tank_name}</p>
        {:else if detail}
          <p class="muted">{m.maintenance_whole_system()}</p>
        {/if}
        {#if detail && task.description}<p class="description">{task.description}</p>{/if}
        <p class="muted">{describeSchedule(task.schedule)}</p>
        <p class="muted">
          {task.next_due_at === null
            ? m.maintenance_no_due()
            : m.maintenance_next_due({ when: formatDateTime(task.next_due_at, timeZone) })}
        </p>
        <p class="muted">
          {#if task.last_done_at === null}
            {m.maintenance_never_done()}
          {:else if task.last_outcome === 'skipped'}
            {m.maintenance_last_skipped({ when: formatDateTime(task.last_done_at, timeZone) })}
          {:else}
            {m.maintenance_last_done({ when: formatDateTime(task.last_done_at, timeZone) })}
          {/if}
        </p>
      </div>

      {#if failure}
        <Alert kind="error">{failure}</Alert>
      {/if}

      {#if mode === 'done' || mode === 'skipped'}
        <CompleteForm
          {task}
          outcome={mode}
          {timeZone}
          onsaved={(result) => {
            mode = 'view';
            onchanged(result.task, result.completion);
          }}
          oncancel={() => (mode = 'view')}
        />
      {:else}
        <div class="row">
          <button type="button" class="btn primary" onclick={() => (mode = 'done')}>
            {m.maintenance_done()}<span class="sr-only"> {task.name}</span>
          </button>
          <button type="button" class="btn" onclick={() => (mode = 'skipped')}>
            {m.maintenance_skip()}<span class="sr-only"> {task.name}</span>
          </button>
          <button type="button" class="btn" onclick={() => (mode = 'edit')}>
            {m.form_edit()}<span class="sr-only"> {task.name}</span>
          </button>
          {#if !detail}
            <a class="btn" href="/maintenance/{task.id}">
              {m.maintenance_history()}<span class="sr-only"> {task.name}</span>
            </a>
          {/if}
          <ConfirmButton
            label={m.form_delete()}
            ariaLabel={m.maintenance_delete_named({ name: task.name })}
            question={m.maintenance_delete_question({ name: task.name })}
            confirmLabel={m.form_delete()}
            onconfirm={remove}
          />
        </div>
      {/if}
    </div>
  {/if}
</li>

<style>
  .item {
    margin: 0;
    list-style: none;
  }

  .body {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    margin: 0;
  }

  .item.overdue .body {
    border-left: 4px solid var(--warning);
  }

  .item.paused .summary {
    opacity: 0.75;
  }

  .summary p {
    margin: 0;
  }

  .what {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: var(--space-2);
  }

  h1.name {
    font-size: 1.5rem;
  }

  .name {
    margin: 0;
    font-size: 1.1rem;
    font-weight: 600;
    overflow-wrap: anywhere;
  }

  .description {
    overflow-wrap: anywhere;
  }
</style>

<script lang="ts">
  import { errorText } from '#lib/api/error-text.js';
  import { maintenanceApi, type Completion, type Task } from '#lib/api/maintenance.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import { currentTank } from '#lib/tank/current-tank.svelte.js';
  import {
    groupByTank,
    removeTask,
    replaceTask,
    segmentCounts,
    segments,
    segmentTasks,
    type TaskSegment
  } from '#lib/maintenance/grouping.js';
  import { useOptions } from '#lib/maintenance/options.js';
  import { m } from '#lib/paraglide/messages.js';
  import { session } from '#lib/session/session.svelte.js';
  import TaskForm from './TaskForm.svelte';
  import TaskList from './TaskList.svelte';

  interface Props {
    /** The tank to list tasks for, with those of the whole system; null lists every tank. */
    tankId: number | null;
  }

  let { tankId }: Props = $props();

  const options = useOptions();
  const timeZone = $derived(session.timeZone);

  let tasks = $state.raw<Task[] | null>(null);
  let failure = $state('');
  let notice = $state('');
  let segment = $state<TaskSegment>('due');
  let adding = $state(false);
  let attempt = $state(0);

  $effect(() => {
    void attempt;
    const id = tankId;
    const controller = new AbortController();
    tasks = null;
    failure = '';
    const query =
      id === null
        ? { status: 'all' as const }
        : { tank_id: id, include_tankless: true, status: 'all' as const };
    maintenanceApi
      .tasks(query, controller.signal)
      .then((list) => {
        if (!controller.signal.aborted) tasks = list;
      })
      .catch((e: unknown) => {
        if (!controller.signal.aborted) failure = errorText(e);
      });
    return () => controller.abort();
  });

  // The tank a new task starts in: the page's tank, else the tank chosen in the header, else the first one.
  const defaultTankId = $derived(
    tankId ??
      (options.tanks.some((t) => t.id === currentTank.id) ? currentTank.id : null) ??
      options.tanks[0]?.id ??
      null
  );

  // The clock is read again whenever the list changes, so a change near midnight moves tasks between the segments.
  const now = $derived.by(() => {
    void tasks;
    return new Date();
  });
  const counts = $derived(segmentCounts(tasks ?? [], now, timeZone));
  const sections = $derived(groupByTank(segmentTasks(tasks ?? [], segment, now, timeZone)));

  const segmentLabels: Record<TaskSegment, (count: number) => string> = {
    due: (count) => m.maintenance_segment_due({ count }),
    upcoming: (count) => m.maintenance_segment_upcoming({ count }),
    all: (count) => m.maintenance_segment_all({ count })
  };
  const emptyTexts: Record<TaskSegment, () => string> = {
    due: m.maintenance_empty_due,
    upcoming: m.maintenance_empty_upcoming,
    all: m.maintenance_empty_all
  };

  /** A task that no longer belongs on this page, such as one moved to another tank, is dropped from the list. */
  const belongs = (task: Task) =>
    tankId === null || task.tank_id === null || task.tank_id === tankId;

  function changed(task: Task, completion?: Completion) {
    tasks = belongs(task) ? replaceTask(tasks ?? [], task) : removeTask(tasks ?? [], task.id);
    if (completion) {
      notice =
        completion.outcome === 'skipped'
          ? m.maintenance_notice_skipped({ name: task.name })
          : m.maintenance_notice_done({ name: task.name });
    } else {
      notice = '';
    }
  }

  function added(task: Task) {
    adding = false;
    tasks = belongs(task) ? replaceTask(tasks ?? [], task) : (tasks ?? []);
    notice = m.maintenance_notice_added({ name: task.name });
  }

  function removed(id: number) {
    tasks = removeTask(tasks ?? [], id);
    notice = '';
  }
</script>

<div class="stack">
  {#if failure}
    <Alert kind="error">{failure}</Alert>
    <div>
      <button type="button" class="btn" onclick={() => attempt++}>{m.maintenance_retry()}</button>
    </div>
  {:else if tasks === null}
    <p class="muted" role="status">{m.loading()}</p>
  {:else}
    {#if notice}
      <Alert kind="success">{notice}</Alert>
    {/if}

    {#if adding}
      <TaskForm {defaultTankId} {timeZone} onsaved={added} oncancel={() => (adding = false)} />
    {:else}
      <div class="row">
        <button type="button" class="btn primary" onclick={() => (adding = true)}>
          {m.maintenance_add()}
        </button>
        <a class="btn" href="/maintenance/templates">{m.maintenance_templates_title()}</a>
      </div>
    {/if}

    <div class="row" role="group" aria-label={m.maintenance_segments_label()}>
      {#each segments as s (s)}
        <button
          type="button"
          class="btn"
          class:primary={segment === s}
          aria-pressed={segment === s}
          onclick={() => (segment = s)}
        >
          {segmentLabels[s](counts[s])}
        </button>
      {/each}
    </div>

    <TaskList
      {sections}
      thisTankId={tankId}
      {timeZone}
      empty={emptyTexts[segment]()}
      onchanged={changed}
      onremoved={removed}
    />
  {/if}
</div>

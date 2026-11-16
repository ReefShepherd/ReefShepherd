<script lang="ts">
  import { goto } from '$app/navigation';
  import { errorText } from '#lib/api/error-text.js';
  import { maintenanceApi, type Completion, type Task } from '#lib/api/maintenance.js';
  import { tankPath } from '#lib/api/tank-id.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import CompletionHistory from '#lib/components/maintenance/CompletionHistory.svelte';
  import OptionsProvider from '#lib/components/maintenance/OptionsProvider.svelte';
  import TaskRow from '#lib/components/maintenance/TaskRow.svelte';
  import PageTitle from '#lib/components/shell/PageTitle.svelte';
  import { m } from '#lib/paraglide/messages.js';
  import { session } from '#lib/session/session.svelte.js';
  import type { PageProps } from './$types';

  let { data }: PageProps = $props();
  const id = $derived(data.taskId);

  let task = $state.raw<Task | null>(null);
  let failure = $state('');
  let history = $state<CompletionHistory>();

  $effect(() => {
    const wanted = id;
    const controller = new AbortController();
    task = null;
    failure = '';
    maintenanceApi
      .task(wanted, controller.signal)
      .then((t) => {
        if (!controller.signal.aborted) task = t;
      })
      .catch((e: unknown) => {
        if (!controller.signal.aborted) failure = errorText(e);
      });
    return () => controller.abort();
  });

  // An edited or deleted history entry can move the next due date, so the task is read again.
  function reload() {
    const wanted = id;
    maintenanceApi
      .task(wanted)
      .then((t) => {
        if (wanted === id) task = t;
      })
      .catch((e: unknown) => (failure = errorText(e)));
  }

  function changed(updated: Task, completion?: Completion) {
    task = updated;
    if (completion) history?.add(completion);
  }

  const backHref = $derived(
    task?.tank_id != null ? tankPath(task.tank_id, 'maintenance') : '/maintenance'
  );
</script>

<PageTitle title={task?.name ?? m.maintenance_title()} />

<div class="stack">
  <p><a href={backHref}>{m.maintenance_back()}</a></p>

  {#if !task}
    <h1>{m.maintenance_title()}</h1>
  {/if}

  {#if failure}
    <Alert kind="error">{failure}</Alert>
  {/if}

  {#if task}
    <OptionsProvider>
      <ul class="single">
        {#key task.id}
          <TaskRow
            {task}
            detail
            timeZone={session.timeZone}
            onchanged={changed}
            onremoved={() => void goto(backHref)}
          />
        {/key}
      </ul>
      <CompletionHistory
        bind:this={history}
        taskId={task.id}
        timeZone={session.timeZone}
        onchanged={reload}
      />
    </OptionsProvider>
  {:else if !failure}
    <p class="muted" role="status">{m.loading()}</p>
  {/if}
</div>

<style>
  .single {
    margin: 0;
    padding: 0;
    list-style: none;
  }
</style>

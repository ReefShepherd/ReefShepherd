<script lang="ts">
  import type { Completion, Task } from '#lib/api/maintenance.js';
  import type { TaskSection } from '#lib/maintenance/grouping.js';
  import { m } from '#lib/paraglide/messages.js';
  import TaskRow from './TaskRow.svelte';

  interface Props {
    sections: readonly TaskSection[];
    /** The tank whose page this is, or null on the page for every tank. */
    thisTankId: number | null;
    timeZone: string;
    /** Shown when no section has a task. */
    empty: string;
    onchanged: (task: Task, completion?: Completion) => void;
    onremoved: (id: number) => void;
  }

  let { sections, thisTankId, timeZone, empty, onchanged, onremoved }: Props = $props();

  function heading(section: TaskSection): string {
    if (section.tankId === null) return m.maintenance_whole_system();
    if (section.tankId === thisTankId) return m.maintenance_section_tank();
    return section.tankName ?? String(section.tankId);
  }
</script>

{#if sections.length === 0}
  <p class="muted">{empty}</p>
{:else}
  {#each sections as section (section.tankId ?? 'system')}
    <section class="stack" aria-labelledby="section-{section.tankId ?? 'system'}">
      <h2 id="section-{section.tankId ?? 'system'}">{heading(section)}</h2>
      <ul class="list">
        {#each section.tasks as task (task.id)}
          <TaskRow {task} {timeZone} {onchanged} {onremoved} />
        {/each}
      </ul>
    </section>
  {/each}
{/if}

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

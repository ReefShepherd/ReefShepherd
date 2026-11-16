<script lang="ts">
  import { maintenanceApi, type Task } from '#lib/api/maintenance.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import ScheduleEditor from '#lib/components/forms/ScheduleEditor.svelte';
  import SelectField from '#lib/components/forms/SelectField.svelte';
  import TextAreaField from '#lib/components/forms/TextAreaField.svelte';
  import TextField from '#lib/components/forms/TextField.svelte';
  import { focusFirstField, focusFirstInvalid } from '#lib/dom.js';
  import { todayIn } from '#lib/format/local-time.js';
  import { hasErrors, type FieldErrors } from '#lib/forms/codes.js';
  import { guarded, type GuardHooks } from '#lib/forms/guarded.js';
  import { fieldTexts } from '#lib/forms/serverErrors.js';
  import {
    blankDraft,
    descriptionMax,
    draftFromPreset,
    draftFromTask,
    nameMax,
    newTask,
    tankChoices,
    taskChanges,
    wholeSystem
  } from '#lib/maintenance/draft.js';
  import { useOptions } from '#lib/maintenance/options.js';
  import { m } from '#lib/paraglide/messages.js';
  import { scheduleKinds, type ScheduleProblem } from '#lib/schedule/schedule.js';

  interface Props {
    /** The task being edited; without it the form adds a new one. */
    task?: Task;
    /** The tank a new task starts in, null for the whole system. */
    defaultTankId?: number | null;
    timeZone: string;
    onsaved: (task: Task) => void;
    oncancel: () => void;
  }

  let { task, defaultTankId = null, timeZone, onsaved, oncancel }: Props = $props();

  const options = useOptions();

  // svelte-ignore state_referenced_locally
  let draft = $state(task ? draftFromTask(task) : blankDraft(defaultTankId, options.dueTime));
  let errors = $state<FieldErrors>({});
  const texts = $derived(fieldTexts(errors, { name: nameMax, description: descriptionMax }));
  let scheduleProblems = $state<ScheduleProblem[]>([]);
  let showScheduleErrors = $state(false);
  let failure = $state('');
  let busy = $state(false);
  let form = $state<HTMLFormElement>();

  const tankOptions = $derived([
    { value: wholeSystem, label: m.maintenance_whole_system() },
    ...tankChoices(options.tanks, task).map((t) => ({ value: String(t.id), label: t.name }))
  ]);
  const presetOptions = $derived([
    { value: '', label: m.maintenance_template_blank() },
    ...options.presets.map((p) => ({ value: String(p.id), label: p.name }))
  ]);

  // A tank template moves the choice back to the page's tank when an earlier template had picked the whole system.
  const tankChoice = $derived(defaultTankId === null ? wholeSystem : String(defaultTankId));

  function choosePreset(id: string) {
    const preset = options.presets.find((p) => String(p.id) === id);
    draft = preset
      ? draftFromPreset(draft, preset, options.dueTime, tankChoice)
      : { ...draft, presetId: '' };
  }

  const hooks: GuardHooks = {
    failure: (text) => (failure = text),
    busy: (b) => (busy = b),
    fields: (f) => (errors = f),
    shown: [
      'name',
      'description',
      'tank_id',
      'preset_id',
      'last_done_on',
      'schedule',
      'schedule.kind'
    ],
    form: () => form
  };

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    failure = '';
    showScheduleErrors = true;
    if (task) return save(task);

    const entry = newTask(draft, todayIn(timeZone));
    errors = entry.errors;
    scheduleProblems = entry.schedule;
    const body = entry.body;
    if (hasErrors(errors) || scheduleProblems.length > 0 || body === null) {
      return focusFirstInvalid(form);
    }
    await guarded(async () => onsaved(await maintenanceApi.createTask(body)), hooks);
  }

  async function save(current: Task) {
    const edit = taskChanges(current, draft);
    errors = edit.errors;
    scheduleProblems = edit.schedule;
    if (hasErrors(errors) || scheduleProblems.length > 0) return focusFirstInvalid(form);
    if (Object.keys(edit.changes).length === 0) return oncancel();
    await guarded(
      async () => onsaved(await maintenanceApi.updateTask(current.id, edit.changes)),
      hooks
    );
  }
</script>

<form class="card stack" novalidate bind:this={form} onsubmit={submit} use:focusFirstField>
  <h2>{task ? m.maintenance_form_edit({ name: task.name }) : m.maintenance_form_new()}</h2>
  {#if failure}
    <Alert kind="error">{failure}</Alert>
  {/if}

  {#if !task && options.presets.length > 0}
    <SelectField
      label={m.maintenance_template()}
      bind:value={() => draft.presetId, choosePreset}
      options={presetOptions}
      hint={m.maintenance_template_hint()}
    />
  {/if}

  <TextField
    label={m.maintenance_name()}
    bind:value={draft.name}
    error={texts.name}
    autocomplete="off"
  />
  <TextAreaField
    label={m.maintenance_description()}
    bind:value={draft.description}
    error={texts.description}
  />
  <SelectField
    label={m.maintenance_tank()}
    bind:value={draft.tank}
    options={tankOptions}
    error={texts.tank_id}
  />

  <ScheduleEditor
    bind:value={draft.schedule}
    allowedKinds={scheduleKinds}
    showErrors={showScheduleErrors}
    error={texts.schedule ?? texts['schedule.kind']}
  />

  <label class="choice">
    <input type="checkbox" bind:checked={draft.notify} />
    <span class="choice-text">
      <span>{m.maintenance_notify()}</span>
      <span class="hint">{m.maintenance_notify_hint()}</span>
    </span>
  </label>

  {#if task}
    <label class="choice">
      <input type="checkbox" bind:checked={draft.paused} />
      <span class="choice-text">
        <span>{m.maintenance_paused()}</span>
        <span class="hint">{m.maintenance_paused_hint()}</span>
      </span>
    </label>
  {:else}
    <TextField
      label={m.maintenance_last_done_on()}
      type="date"
      max={todayIn(timeZone)}
      bind:value={draft.lastDoneOn}
      error={texts.last_done_on}
      hint={m.maintenance_last_done_on_hint()}
    />
  {/if}

  <div class="row">
    <button class="btn primary" type="submit" disabled={busy}>
      {task ? m.form_save() : m.maintenance_add()}
    </button>
    <button class="btn" type="button" disabled={busy} onclick={oncancel}>
      {m.form_cancel()}
    </button>
  </div>
</form>

<style>
  h2 {
    margin: 0;
  }
</style>

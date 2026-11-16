<script lang="ts">
  import { onMount } from 'svelte';
  import { errorText } from '#lib/api/error-text.js';
  import { settingsApi, type InstanceSettings } from '#lib/api/settings.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import TextField from '#lib/components/forms/TextField.svelte';
  import {
    backupPatch,
    draftErrors,
    draftFromSettings,
    type BackupDraft
  } from '#lib/backups/draft.js';
  import { focusFirstInvalid } from '#lib/dom.js';
  import { hasErrors, type FieldErrors } from '#lib/forms/codes.js';
  import { guarded, type GuardHooks } from '#lib/forms/guarded.js';
  import { fieldTexts } from '#lib/forms/serverErrors.js';
  import { m } from '#lib/paraglide/messages.js';
  import { session } from '#lib/session/session.svelte.js';
  import { hasChanges } from '#lib/settings/diff.js';

  let original = $state<InstanceSettings | null>(null);
  let draft = $state<BackupDraft | null>(null);
  let loadError = $state('');
  let failure = $state('');
  let saved = $state(false);
  let busy = $state(false);
  let errors = $state<FieldErrors>({});
  const texts = $derived(fieldTexts(errors));
  let form = $state<HTMLFormElement>();

  const patch = $derived(original && draft ? backupPatch(original, draft) : {});

  onMount(() => {
    settingsApi
      .get()
      .then((settings) => {
        original = settings;
        draft = draftFromSettings(settings);
      })
      .catch((e: unknown) => (loadError = errorText(e)));
  });

  const hooks: GuardHooks = {
    failure: (text) => (failure = text),
    busy: (b) => (busy = b),
    fields: (f) => (errors = f),
    shown: ['backup_time', 'backup_keep'],
    form: () => form
  };

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    if (!original || !draft) return;
    saved = false;
    failure = '';
    errors = draftErrors(draft);
    if (hasErrors(errors)) return focusFirstInvalid(form);
    if (!hasChanges(patch)) return;

    await guarded(async () => {
      const result = await settingsApi.update(patch);
      original = result;
      draft = draftFromSettings(result);
      saved = true;
    }, hooks);
  }
</script>

{#if loadError}
  <Alert kind="error">{loadError}</Alert>
{:else if original === null || draft === null}
  <p class="muted" role="status">{m.loading()}</p>
{:else}
  <form
    class="card stack"
    novalidate
    bind:this={form}
    onsubmit={submit}
    aria-labelledby="backups-settings"
  >
    <h2 id="backups-settings">{m.backups_settings_heading()}</h2>
    {#if failure}
      <Alert kind="error">{failure}</Alert>
    {/if}
    {#if saved}
      <Alert kind="success">{m.backups_saved()}</Alert>
    {/if}

    <label class="choice">
      <input type="checkbox" bind:checked={draft.enabled} />
      <span class="choice-text">{m.backups_enabled()}</span>
    </label>
    <TextField
      label={m.backups_time()}
      type="time"
      bind:value={draft.time}
      error={texts.backup_time}
      hint={m.backups_time_hint({ timezone: session.timeZone })}
    />
    <TextField
      label={m.backups_keep()}
      inputmode="numeric"
      autocomplete="off"
      bind:value={draft.keep}
      error={texts.backup_keep}
      hint={m.backups_keep_hint()}
    />

    <div>
      <button class="btn primary" type="submit" disabled={busy || !hasChanges(patch)}>
        {m.form_save()}
      </button>
    </div>
  </form>
{/if}

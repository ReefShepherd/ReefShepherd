<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';
  import { errorText } from '#lib/api/error-text.js';
  import { settingsApi, type InstanceSettings, type UnitFamily } from '#lib/api/settings.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import LoginModeField from '#lib/components/forms/LoginModeField.svelte';
  import TextField from '#lib/components/forms/TextField.svelte';
  import TimeZoneField from '#lib/components/forms/TimeZoneField.svelte';
  import UnitPicker from '#lib/components/forms/UnitPicker.svelte';
  import { focusFirstInvalid } from '#lib/dom.js';
  import { hasErrors, type FieldErrors } from '#lib/forms/codes.js';
  import { guarded, type GuardHooks } from '#lib/forms/guarded.js';
  import { fieldTexts, moveField } from '#lib/forms/serverErrors.js';
  import { collectErrors, validateRequired, validateTimeZone } from '#lib/forms/validation.js';
  import { m } from '#lib/paraglide/messages.js';
  import { session } from '#lib/session/session.svelte.js';
  import {
    draftFromSettings,
    hasChanges,
    settingsPatch,
    turnsLoginOff,
    turnsLoginOn,
    type InstanceDraft
  } from '#lib/settings/diff.js';

  let original = $state<InstanceSettings | null>(null);
  let catalog = $state<UnitFamily[]>([]);
  let draft = $state<InstanceDraft | null>(null);
  let loadError = $state('');
  let failure = $state('');
  let saved = $state(false);
  let busy = $state(false);
  let errors = $state<FieldErrors>({});
  const texts = $derived(fieldTexts(errors));
  let form = $state<HTMLFormElement>();

  const patch = $derived(original && draft ? settingsPatch(original, draft) : {});
  const loginOff = $derived(original && draft ? turnsLoginOff(original, draft) : false);
  // Turning login back on needs an administrator who can sign in with a password.
  const needsPassword = $derived(
    original?.auth_mode === 'disabled' && session.me?.has_password === false
  );

  const lockedBy = (field: string) => original?.env_locked[field];
  const loginNote = $derived.by(() => {
    const variable = lockedBy('auth_mode');
    if (variable) return m.instance_env_locked({ variable });
    return needsPassword ? m.instance_login_needs_password() : undefined;
  });

  onMount(() => {
    Promise.all([settingsApi.get(), settingsApi.units()])
      .then(([settings, units]) => {
        original = settings;
        catalog = units;
        draft = draftFromSettings(settings);
      })
      .catch((e: unknown) => (loadError = errorText(e)));
  });

  const hooks: GuardHooks = {
    failure: (text) => (failure = text),
    busy: (b) => (busy = b),
    // The confirmation password is the server's `password`; it reads as the current password here.
    fields: (f) => (errors = moveField(f, 'password', 'current_password')),
    shown: ['instance_name', 'timezone', 'auth_mode', 'password'],
    form: () => form
  };

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    if (!original || !draft) return;
    saved = false;
    failure = '';
    errors = collectErrors({
      instance_name: validateRequired(draft.instanceName),
      timezone: validateTimeZone(draft.timeZone),
      current_password: loginOff ? validateRequired(draft.password) : null
    });
    if (hasErrors(errors)) return focusFirstInvalid(form);
    if (!hasChanges(patch)) return;

    const goingBackToLogin = turnsLoginOn(original, draft);
    await guarded(async () => {
      const result = await settingsApi.update(patch);
      original = result;
      draft = draftFromSettings(result);
      saved = true;
      if (goingBackToLogin) {
        // The browser has no session yet, so the next step is to sign in.
        session.clear();
        await goto('/login');
        return;
      }
      await session.load();
    }, hooks);
  }
</script>

{#if loadError}
  <Alert kind="error">{loadError}</Alert>
{:else if original === null || draft === null}
  <p class="muted" role="status">{m.loading()}</p>
{:else}
  <form class="stack" novalidate bind:this={form} onsubmit={submit}>
    {#if failure}
      <Alert kind="error">{failure}</Alert>
    {/if}
    {#if saved}
      <Alert kind="success">{m.instance_saved()}</Alert>
    {/if}

    <section class="card stack" aria-labelledby="instance-general">
      <h2 id="instance-general">{m.instance_general()}</h2>
      <TextField
        label={m.instance_name()}
        bind:value={draft.instanceName}
        error={texts.instance_name}
        disabled={lockedBy('instance_name') !== undefined}
        hint={lockedBy('instance_name')
          ? m.instance_env_locked({ variable: lockedBy('instance_name') ?? '' })
          : undefined}
        autocomplete="off"
      />
      <TimeZoneField
        label={m.instance_timezone()}
        bind:value={draft.timeZone}
        error={texts.timezone}
        disabled={lockedBy('timezone') !== undefined}
        hint={lockedBy('timezone')
          ? m.instance_env_locked({ variable: lockedBy('timezone') ?? '' })
          : m.instance_timezone_hint()}
      />
    </section>

    <section class="card stack" aria-labelledby="instance-units">
      <div>
        <h2 id="instance-units">{m.instance_units()}</h2>
        <p class="hint">{m.instance_units_hint()}</p>
      </div>
      <UnitPicker {catalog} bind:value={draft.units} disabled={lockedBy('units') !== undefined} />
    </section>

    <section class="card stack" aria-labelledby="instance-login">
      <LoginModeField
        legend={m.instance_login_mode()}
        legendId="instance-login"
        bind:loginRequired={draft.loginRequired}
        disableOn={lockedBy('auth_mode') !== undefined || needsPassword}
        disableOff={lockedBy('auth_mode') !== undefined}
        note={loginNote}
        error={texts.auth_mode}
      />

      {#if loginOff}
        <Alert kind="warning">{m.instance_login_warning()}</Alert>
        <TextField
          label={m.instance_confirm_password()}
          type="password"
          hint={m.instance_confirm_password_hint()}
          bind:value={draft.password}
          error={texts.current_password}
          autocomplete="current-password"
        />
      {/if}
    </section>

    <div>
      <button class="btn primary" type="submit" disabled={busy || !hasChanges(patch)}>
        {m.form_save()}
      </button>
    </div>
  </form>
{/if}

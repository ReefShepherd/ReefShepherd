<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';
  import { authApi } from '#lib/api/auth.js';
  import { errorText } from '#lib/api/error-text.js';
  import { settingsApi, type UnitFamily } from '#lib/api/settings.js';
  import { usersApi } from '#lib/api/users.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import LoginModeField from '#lib/components/forms/LoginModeField.svelte';
  import TextField from '#lib/components/forms/TextField.svelte';
  import ThemePicker from '#lib/components/forms/ThemePicker.svelte';
  import TimeZoneField from '#lib/components/forms/TimeZoneField.svelte';
  import UnitPicker from '#lib/components/forms/UnitPicker.svelte';
  import PageTitle from '#lib/components/shell/PageTitle.svelte';
  import { focusFirstField, focusFirstInvalid } from '#lib/dom.js';
  import { browserTimeZone } from '#lib/format/timezone.js';
  import { hasErrors, type FieldErrors } from '#lib/forms/codes.js';
  import { fieldTexts } from '#lib/forms/serverErrors.js';
  import { passwordLimits } from '#lib/forms/validation.js';
  import { m } from '#lib/paraglide/messages.js';
  import { session } from '#lib/session/session.svelte.js';
  import {
    buildSetupRequest,
    loginChoice,
    newSetupDraft,
    setupServerErrors,
    setupSteps,
    validateSetupStep
  } from '#lib/setup/wizard.js';
  import { theme, type ThemeName } from '#lib/theme/theme.svelte.js';
  import { defaultUnits } from '#lib/units/catalog.js';

  const draft = $state(
    newSetupDraft(m.setup_instance_name_default(), browserTimeZone() ?? 'UTC', {})
  );
  let catalog = $state<UnitFamily[]>([]);
  let ready = $state(false);
  let loadError = $state('');
  let loginLocked = $state(false);
  let stepIndex = $state(0);
  let errors = $state<FieldErrors>({});
  const texts = $derived(fieldTexts(errors));
  let failure = $state('');
  let busy = $state(false);
  let form = $state<HTMLFormElement>();

  const step = $derived(setupSteps[stepIndex]);
  const last = $derived(stepIndex === setupSteps.length - 1);
  const stepTitle = $derived(
    step === 'instance'
      ? m.setup_step_instance()
      : step === 'units'
        ? m.setup_step_units()
        : m.setup_step_access()
  );

  onMount(() => {
    Promise.all([authApi.config(), settingsApi.units()])
      .then(([config, units]) => {
        if (!config.setup_required) {
          void goto('/login', { replaceState: true });
          return;
        }
        catalog = units;
        draft.units = defaultUnits(units);
        const choice = loginChoice(config);
        draft.loginRequired = choice.loginRequired;
        loginLocked = choice.locked;
        ready = true;
      })
      .catch((e: unknown) => (loadError = errorText(e)));
  });

  function chooseTheme(name: ThemeName) {
    draft.theme = name;
    theme.set(name);
  }

  function back() {
    errors = {};
    failure = '';
    stepIndex = Math.max(0, stepIndex - 1);
  }

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    failure = '';
    errors = validateSetupStep(step, draft);
    if (hasErrors(errors)) return focusFirstInvalid(form);
    if (!last) {
      stepIndex += 1;
      return;
    }
    busy = true;
    try {
      const me = await usersApi.setup(buildSetupRequest(draft));
      session.adopt(me);
      await goto('/');
    } catch (e) {
      failure = errorText(e);
      const server = setupServerErrors(e);
      errors = server.errors;
      if (server.step !== undefined) stepIndex = setupSteps.indexOf(server.step);
      await focusFirstInvalid(form);
    } finally {
      busy = false;
    }
  }
</script>

<PageTitle title={m.setup_title()} />

<div class="narrow">
  <h1>{m.setup_title()}</h1>
  <p class="muted">{m.setup_intro()}</p>

  {#if loadError}
    <Alert kind="error">{loadError}</Alert>
  {/if}

  {#if ready}
    {#if failure}
      <Alert kind="error">{failure}</Alert>
    {/if}

    {#key step}
      <form class="card stack" novalidate bind:this={form} onsubmit={submit} use:focusFirstField>
        <div>
          <p class="hint">{m.setup_step({ current: stepIndex + 1, total: setupSteps.length })}</p>
          <h2>{stepTitle}</h2>
        </div>

        {#if step === 'instance'}
          <TextField
            label={m.setup_instance_name()}
            hint={m.setup_instance_name_hint()}
            bind:value={draft.instanceName}
            error={texts.instance_name}
            autocomplete="off"
          />
          <TimeZoneField
            label={m.setup_timezone()}
            hint={m.setup_timezone_hint()}
            bind:value={draft.timeZone}
            error={texts.timezone}
          />
          <ThemePicker value={draft.theme} onchange={chooseTheme} />
        {:else if step === 'units'}
          <p class="hint">{m.setup_units_intro()}</p>
          <UnitPicker {catalog} bind:value={draft.units} />
        {:else}
          <LoginModeField
            legend={m.setup_access_title()}
            bind:loginRequired={draft.loginRequired}
            disableOn={loginLocked}
            disableOff={loginLocked}
            note={loginLocked ? m.setup_login_locked() : undefined}
            error={texts.auth_mode}
          />

          {#if draft.loginRequired}
            <TextField
              label={m.setup_username()}
              bind:value={draft.username}
              error={texts.username}
              autocomplete="username"
              autocapitalize="none"
              spellcheck={false}
            />
            <TextField
              label={m.setup_display_name()}
              bind:value={draft.displayName}
              error={texts.display_name}
              autocomplete="name"
            />
            <TextField
              label={m.setup_password()}
              type="password"
              hint={m.password_hint({ min: passwordLimits.min })}
              bind:value={draft.password}
              error={texts.password}
              autocomplete="new-password"
            />
            <TextField
              label={m.setup_password_repeat()}
              type="password"
              bind:value={draft.passwordRepeat}
              error={texts.password_repeat}
              autocomplete="new-password"
            />
          {:else}
            <Alert kind="warning">{m.setup_no_login_account()}</Alert>
          {/if}
        {/if}

        <div class="row actions">
          {#if stepIndex > 0}
            <button class="btn" type="button" disabled={busy} onclick={back}>{m.form_back()}</button
            >
          {/if}
          <button class="btn primary" type="submit" disabled={busy}>
            {last ? m.setup_finish() : m.form_next()}
          </button>
        </div>
      </form>
    {/key}
  {/if}
</div>

<style>
  .actions {
    justify-content: flex-end;
  }
</style>

<script lang="ts">
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { onMount } from 'svelte';
  import { authApi, type AuthConfig } from '#lib/api/auth.js';
  import { isApiError } from '#lib/api/client.js';
  import { errorText } from '#lib/api/error-text.js';
  import { safeNext } from '#lib/auth/next.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import TextField from '#lib/components/forms/TextField.svelte';
  import PageTitle from '#lib/components/shell/PageTitle.svelte';
  import { focusFirstField, focusFirstInvalid } from '#lib/dom.js';
  import { hasErrors, type FieldErrors } from '#lib/forms/codes.js';
  import { fieldTexts } from '#lib/forms/serverErrors.js';
  import {
    collectErrors,
    normalizeTotpCode,
    validateRecoveryCode,
    validateRequired,
    validateTotpCode
  } from '#lib/forms/validation.js';
  import { m } from '#lib/paraglide/messages.js';
  import { session } from '#lib/session/session.svelte.js';

  type Step = 'password' | 'code' | 'recovery';

  let config = $state<AuthConfig | null>(null);
  let loadError = $state('');
  let step = $state<Step>('password');
  let username = $state('');
  let password = $state('');
  let code = $state('');
  let recoveryCode = $state('');
  let errors = $state<FieldErrors>({});
  const texts = $derived(fieldTexts(errors));
  let failure = $state('');
  let busy = $state(false);
  let form = $state<HTMLFormElement>();

  // Only a same-origin relative path is ever followed, so a crafted link cannot send anyone elsewhere.
  const next = $derived(safeNext(page.url.searchParams.get('next')));

  onMount(() => {
    authApi
      .config()
      .then(async (c) => {
        config = c;
        if (c.setup_required) return void goto('/setup', { replaceState: true });
        if (c.mode === 'disabled') return void goto(next, { replaceState: true });
        // Someone who is already signed in has nothing to do here.
        const signedIn = await session.load().then(
          () => true,
          () => false
        );
        if (signedIn) void goto(next, { replaceState: true });
      })
      .catch((e: unknown) => (loadError = errorText(e)));
  });

  function switchStep(next: Step) {
    step = next;
    failure = '';
    errors = {};
    code = '';
    recoveryCode = '';
  }

  async function finish() {
    session.clear();
    await goto(next);
  }

  async function run(action: () => Promise<void>) {
    failure = '';
    busy = true;
    try {
      await action();
    } catch (e) {
      // An expired first step means the password has to be entered again.
      if (step !== 'password' && isApiError(e) && e.code === 'unauthorized') {
        switchStep('password');
        password = '';
      }
      failure = errorText(e);
      if (isApiError(e) && e.code === 'invalid_code') {
        errors = { [step === 'code' ? 'code' : 'recovery_code']: 'incorrect' };
      }
      await focusFirstInvalid(form);
    } finally {
      busy = false;
    }
  }

  function submitPassword(event: SubmitEvent) {
    event.preventDefault();
    errors = collectErrors({
      username: validateRequired(username),
      password: validateRequired(password)
    });
    if (hasErrors(errors)) return void focusFirstInvalid(form);
    void run(async () => {
      const result = await authApi.login(username.trim(), password);
      password = '';
      if (result.mfa_required) switchStep('code');
      else await finish();
    });
  }

  function submitCode(event: SubmitEvent) {
    event.preventDefault();
    errors = collectErrors({ code: validateTotpCode(code) });
    if (hasErrors(errors)) return void focusFirstInvalid(form);
    void run(async () => {
      await authApi.verifyCode(normalizeTotpCode(code));
      await finish();
    });
  }

  function submitRecovery(event: SubmitEvent) {
    event.preventDefault();
    errors = collectErrors({ recovery_code: validateRecoveryCode(recoveryCode) });
    if (hasErrors(errors)) return void focusFirstInvalid(form);
    void run(async () => {
      await authApi.verifyRecoveryCode(recoveryCode.trim());
      await finish();
    });
  }
</script>

<PageTitle title={m.login_title()} />

<div class="narrow">
  <h1>{step === 'password' ? m.login_title() : m.login_mfa_title()}</h1>
  {#if config?.instance_name}
    <p class="muted">{m.login_subtitle({ name: config.instance_name })}</p>
  {/if}

  {#if loadError}
    <Alert kind="error">{loadError}</Alert>
  {/if}

  {#if config && !config.setup_required && config.mode !== 'disabled'}
    {#if failure}
      <Alert kind="error">{failure}</Alert>
    {/if}

    {#if step === 'password'}
      <form
        class="card stack"
        novalidate
        bind:this={form}
        onsubmit={submitPassword}
        use:focusFirstField
      >
        <TextField
          label={m.login_username()}
          bind:value={username}
          error={texts.username}
          autocomplete="username"
          autocapitalize="none"
          spellcheck={false}
        />
        <TextField
          label={m.login_password()}
          type="password"
          bind:value={password}
          error={texts.password}
          autocomplete="current-password"
        />
        <button class="btn primary" type="submit" disabled={busy}>{m.login_submit()}</button>
      </form>
    {:else if step === 'code'}
      <form
        class="card stack"
        novalidate
        bind:this={form}
        onsubmit={submitCode}
        use:focusFirstField
      >
        <p class="hint">{m.login_mfa_intro()}</p>
        <TextField
          label={m.login_mfa_code()}
          bind:value={code}
          error={texts.code}
          inputmode="numeric"
          autocomplete="one-time-code"
        />
        <button class="btn primary" type="submit" disabled={busy}>{m.login_mfa_submit()}</button>
        <button class="btn" type="button" onclick={() => switchStep('recovery')}>
          {m.login_mfa_use_recovery()}
        </button>
        <button class="btn" type="button" onclick={() => switchStep('password')}>
          {m.login_mfa_start_over()}
        </button>
      </form>
    {:else}
      <form
        class="card stack"
        novalidate
        bind:this={form}
        onsubmit={submitRecovery}
        use:focusFirstField
      >
        <p class="hint">{m.login_mfa_recovery_intro()}</p>
        <TextField
          label={m.login_mfa_recovery()}
          bind:value={recoveryCode}
          error={texts.recovery_code}
          autocomplete="off"
          autocapitalize="characters"
          spellcheck={false}
        />
        <button class="btn primary" type="submit" disabled={busy}>{m.login_mfa_submit()}</button>
        <button class="btn" type="button" onclick={() => switchStep('code')}>
          {m.login_mfa_use_code()}
        </button>
        <button class="btn" type="button" onclick={() => switchStep('password')}>
          {m.login_mfa_start_over()}
        </button>
      </form>
    {/if}
  {/if}
</div>

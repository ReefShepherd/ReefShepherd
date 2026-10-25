<script lang="ts">
  import { authApi, type TotpEnrollment } from '#lib/api/auth.js';
  import { authenticatorLink } from '#lib/auth/otpauth.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import TextField from '#lib/components/forms/TextField.svelte';
  import { focusFirstField, focusFirstInvalid } from '#lib/dom.js';
  import { hasErrors, type FieldErrors } from '#lib/forms/codes.js';
  import { guarded } from '#lib/forms/guarded.js';
  import { fieldTexts } from '#lib/forms/serverErrors.js';
  import {
    collectErrors,
    normalizeTotpCode,
    validateRequired,
    validateTotpCode
  } from '#lib/forms/validation.js';
  import { m } from '#lib/paraglide/messages.js';
  import { session } from '#lib/session/session.svelte.js';
  import RecoveryCodes from './RecoveryCodes.svelte';

  const enabled = $derived(session.me?.totp_enabled ?? false);
  const hasPassword = $derived(session.me?.has_password ?? true);

  let enrollment = $state<TotpEnrollment | null>(null);
  const appLink = $derived(enrollment ? authenticatorLink(enrollment.otpauth_url) : undefined);
  let recoveryCodes = $state<string[] | null>(null);
  let code = $state('');
  let proof = $state('');
  let errors = $state<FieldErrors>({});
  const texts = $derived(fieldTexts(errors));
  // Turning two-step sign-in off asks for the password, or for a code when the account has none.
  const proofField = $derived(hasPassword ? 'password' : 'code');
  let failure = $state('');
  let notice = $state('');
  let busy = $state(false);
  let form = $state<HTMLFormElement>();

  function guard(action: () => Promise<void>, shown: readonly string[] = []) {
    notice = '';
    return guarded(action, {
      failure: (text) => (failure = text),
      busy: (b) => (busy = b),
      fields: (f) => (errors = f),
      shown,
      form: () => form
    });
  }

  function begin() {
    return guard(async () => {
      enrollment = await authApi.beginTotp();
      code = '';
    });
  }

  function cancel() {
    return guard(async () => {
      await authApi.disableTotp();
      enrollment = null;
    });
  }

  async function confirm(event: SubmitEvent) {
    event.preventDefault();
    errors = collectErrors({ code: validateTotpCode(code) });
    if (hasErrors(errors)) return focusFirstInvalid(form);
    await guard(async () => {
      const result = await authApi.confirmTotp(normalizeTotpCode(code));
      recoveryCodes = result.recovery_codes;
      enrollment = null;
      await session.load();
    }, ['code']);
  }

  async function disable(event: SubmitEvent) {
    event.preventDefault();
    errors = collectErrors({
      [proofField]: hasPassword ? validateRequired(proof) : validateTotpCode(proof)
    });
    if (hasErrors(errors)) return focusFirstInvalid(form);
    await guard(async () => {
      await authApi.disableTotp(
        hasPassword ? { password: proof } : { code: normalizeTotpCode(proof) }
      );
      proof = '';
      notice = m.totp_disabled();
      await session.load();
    }, [proofField]);
  }
</script>

<section class="card stack" aria-labelledby="totp-heading">
  <div>
    <h2 id="totp-heading">{m.totp_title()}</h2>
    <p class="hint">{enabled ? m.totp_on_intro() : m.totp_off_intro()}</p>
  </div>

  {#if failure}
    <Alert kind="error">{failure}</Alert>
  {/if}
  {#if notice}
    <Alert kind="success">{notice}</Alert>
  {/if}

  {#if recoveryCodes}
    <RecoveryCodes codes={recoveryCodes} ondone={() => (recoveryCodes = null)} />
  {:else if enrollment}
    <form class="stack" novalidate bind:this={form} onsubmit={confirm} use:focusFirstField>
      <div>
        <h3>{m.totp_enroll_title()}</h3>
        <p class="hint">{m.totp_enroll_intro()}</p>
      </div>
      <img class="qr" src={enrollment.qr_code} alt={m.totp_qr_alt()} width="200" height="200" />
      {#if appLink}
        <p><a class="btn" href={appLink}>{m.totp_open_app()}</a></p>
      {/if}
      <p class="hint">
        {m.totp_secret_label()} <span class="mono">{enrollment.secret}</span>
      </p>
      <TextField
        label={m.totp_code()}
        bind:value={code}
        error={texts.code}
        inputmode="numeric"
        autocomplete="one-time-code"
      />
      <div class="row">
        <button class="btn primary" type="submit" disabled={busy}>{m.totp_confirm()}</button>
        <button class="btn" type="button" disabled={busy} onclick={cancel}>
          {m.totp_cancel()}
        </button>
      </div>
    </form>
  {:else if enabled}
    <form class="stack" novalidate bind:this={form} onsubmit={disable}>
      <h3>{m.totp_disable_title()}</h3>
      <TextField
        label={hasPassword ? m.login_password() : m.totp_code()}
        type={hasPassword ? 'password' : 'text'}
        hint={hasPassword ? m.totp_disable_intro_password() : m.totp_disable_intro_code()}
        bind:value={proof}
        error={texts[proofField]}
        autocomplete={hasPassword ? 'current-password' : 'one-time-code'}
        inputmode={hasPassword ? undefined : 'numeric'}
      />
      <div>
        <button class="btn danger" type="submit" disabled={busy}>{m.totp_disable_submit()}</button>
      </div>
    </form>
  {:else}
    <div>
      <button class="btn primary" type="button" disabled={busy} onclick={begin}>
        {m.totp_enable()}
      </button>
    </div>
  {/if}
</section>

<style>
  .qr {
    width: 12.5rem;
    height: 12.5rem;
    max-width: 100%;
    border-radius: var(--radius);
    image-rendering: pixelated;
  }
</style>

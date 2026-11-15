<script lang="ts">
  import { authApi } from '#lib/api/auth.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import TextField from '#lib/components/forms/TextField.svelte';
  import { focusFirstInvalid } from '#lib/dom.js';
  import { hasErrors, type FieldErrors } from '#lib/forms/codes.js';
  import { guarded, type GuardHooks } from '#lib/forms/guarded.js';
  import { fieldTexts } from '#lib/forms/serverErrors.js';
  import {
    collectErrors,
    passwordLimits,
    validateNewPassword,
    validatePasswordMatch,
    validateRequired
  } from '#lib/forms/validation.js';
  import { m } from '#lib/paraglide/messages.js';
  import { session } from '#lib/session/session.svelte.js';

  let { onchanged }: { onchanged: () => void } = $props();

  // An account without a password has nothing to confirm, so it sets one without the current password.
  const hasPassword = $derived(session.me?.has_password ?? true);

  let current = $state('');
  let next = $state('');
  let repeat = $state('');
  let errors = $state<FieldErrors>({});
  const texts = $derived(fieldTexts(errors));
  let failure = $state('');
  let done = $state(false);
  let busy = $state(false);
  let form = $state<HTMLFormElement>();

  const hooks: GuardHooks = {
    failure: (text) => (failure = text),
    busy: (b) => (busy = b),
    fields: (f) => (errors = f),
    shown: ['current_password', 'new_password'],
    form: () => form
  };

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    done = false;
    failure = '';
    errors = collectErrors({
      current_password: hasPassword ? validateRequired(current) : null,
      new_password: validateNewPassword(next),
      password_repeat: validatePasswordMatch(next, repeat)
    });
    if (hasErrors(errors)) return focusFirstInvalid(form);

    await guarded(async () => {
      await authApi.changePassword(next, hasPassword ? current : undefined);
      current = next = repeat = '';
      done = true;
      await session.load();
      onchanged();
    }, hooks);
  }
</script>

<section class="card" aria-labelledby="password-heading">
  <form class="stack" novalidate bind:this={form} onsubmit={submit}>
    <div>
      <h2 id="password-heading">{hasPassword ? m.password_title() : m.password_set_title()}</h2>
      <p class="hint">{hasPassword ? m.password_intro() : m.password_set_intro()}</p>
    </div>
    {#if failure}
      <Alert kind="error">{failure}</Alert>
    {/if}
    {#if done}
      <Alert kind="success">{m.password_changed()}</Alert>
    {/if}
    {#if hasPassword}
      <TextField
        label={m.password_current()}
        type="password"
        bind:value={current}
        error={texts.current_password}
        autocomplete="current-password"
      />
    {/if}
    <TextField
      label={m.password_new()}
      type="password"
      hint={m.password_hint({ min: passwordLimits.min })}
      bind:value={next}
      error={texts.new_password}
      autocomplete="new-password"
    />
    <TextField
      label={m.password_confirm()}
      type="password"
      bind:value={repeat}
      error={texts.password_repeat}
      autocomplete="new-password"
    />
    <div>
      <button class="btn primary" type="submit" disabled={busy}>
        {hasPassword ? m.password_submit() : m.password_set_submit()}
      </button>
    </div>
  </form>
</section>

<script lang="ts">
  import { authApi } from '#lib/api/auth.js';
  import { errorText } from '#lib/api/error-text.js';
  import { fieldText } from '#lib/api/field-errors.js';
  import {
    collectErrors,
    passwordLimits,
    validateNewPassword,
    validatePasswordMatch,
    validateRequired
  } from '#lib/auth/validation.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import TextField from '#lib/components/forms/TextField.svelte';
  import { focusFirstInvalid } from '#lib/dom.js';
  import { m } from '#lib/paraglide/messages.js';
  import { session } from '#lib/session/session.svelte.js';

  let { onchanged }: { onchanged: () => void } = $props();

  // An account without a password has nothing to confirm, so it sets one without the current password.
  const hasPassword = $derived(session.me?.has_password ?? true);

  let current = $state('');
  let next = $state('');
  let repeat = $state('');
  let errors = $state<Partial<Record<'current' | 'next' | 'repeat', string>>>({});
  let failure = $state('');
  let done = $state(false);
  let busy = $state(false);
  let form = $state<HTMLFormElement>();

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    done = false;
    failure = '';
    errors = collectErrors({
      current: hasPassword ? validateRequired(current) : null,
      next: validateNewPassword(next),
      repeat: validatePasswordMatch(next, repeat)
    });
    if (Object.keys(errors).length > 0) return focusFirstInvalid(form);

    busy = true;
    try {
      await authApi.changePassword(next, hasPassword ? current : undefined);
      current = next = repeat = '';
      done = true;
      await session.load();
      onchanged();
    } catch (e) {
      errors = {
        current: fieldText(e, 'current_password'),
        next: fieldText(e, 'new_password')
      };
      if (!errors.current && !errors.next) failure = errorText(e);
      await focusFirstInvalid(form);
    } finally {
      busy = false;
    }
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
        error={errors.current}
        autocomplete="current-password"
      />
    {/if}
    <TextField
      label={m.password_new()}
      type="password"
      hint={m.password_hint({ min: passwordLimits.min })}
      bind:value={next}
      error={errors.next}
      autocomplete="new-password"
    />
    <TextField
      label={m.password_confirm()}
      type="password"
      bind:value={repeat}
      error={errors.repeat}
      autocomplete="new-password"
    />
    <div>
      <button class="btn primary" type="submit" disabled={busy}>
        {hasPassword ? m.password_submit() : m.password_set_submit()}
      </button>
    </div>
  </form>
</section>

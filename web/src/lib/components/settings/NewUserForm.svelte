<script lang="ts">
  import { usersApi, type Role, type User } from '#lib/api/users.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import SelectField from '#lib/components/forms/SelectField.svelte';
  import TextField from '#lib/components/forms/TextField.svelte';
  import { focusFirstField, focusFirstInvalid } from '#lib/dom.js';
  import { hasErrors, type FieldErrors } from '#lib/forms/codes.js';
  import { guarded, type GuardHooks } from '#lib/forms/guarded.js';
  import { fieldTexts } from '#lib/forms/serverErrors.js';
  import {
    collectErrors,
    passwordLimits,
    validateDisplayName,
    validateNewPassword,
    validateOptionalEmail,
    validateUsername
  } from '#lib/forms/validation.js';
  import { m } from '#lib/paraglide/messages.js';

  interface Props {
    oncreated: (user: User) => void;
    oncancel: () => void;
  }

  let { oncreated, oncancel }: Props = $props();

  let username = $state('');
  let displayName = $state('');
  let email = $state('');
  let role = $state<Role>('member');
  let password = $state('');
  let errors = $state<FieldErrors>({});
  const texts = $derived(fieldTexts(errors));
  let failure = $state('');
  let busy = $state(false);
  let form = $state<HTMLFormElement>();

  const hooks: GuardHooks = {
    failure: (text) => (failure = text),
    busy: (b) => (busy = b),
    fields: (f) => (errors = f),
    shown: ['username', 'display_name', 'email', 'password'],
    form: () => form
  };

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    failure = '';
    errors = collectErrors({
      username: validateUsername(username),
      display_name: validateDisplayName(displayName),
      email: validateOptionalEmail(email),
      password: validateNewPassword(password)
    });
    if (hasErrors(errors)) return focusFirstInvalid(form);

    await guarded(async () => {
      const user = await usersApi.create({
        username: username.trim(),
        display_name: displayName.trim(),
        email: email.trim() === '' ? null : email.trim(),
        role,
        password
      });
      oncreated(user);
    }, hooks);
  }
</script>

<form class="card stack" novalidate bind:this={form} onsubmit={submit} use:focusFirstField>
  <h2>{m.users_add_title()}</h2>
  {#if failure}
    <Alert kind="error">{failure}</Alert>
  {/if}
  <TextField
    label={m.users_username()}
    bind:value={username}
    error={texts.username}
    autocomplete="off"
    autocapitalize="none"
    spellcheck={false}
  />
  <TextField
    label={m.users_display_name()}
    bind:value={displayName}
    error={texts.display_name}
    autocomplete="off"
  />
  <TextField
    label={m.users_email()}
    type="email"
    bind:value={email}
    error={texts.email}
    autocomplete="off"
  />
  <SelectField
    label={m.users_role()}
    bind:value={role}
    options={[
      { value: 'member', label: m.role_member() },
      { value: 'admin', label: m.role_admin() }
    ]}
  />
  <TextField
    label={m.users_password()}
    type="password"
    hint={m.password_hint({ min: passwordLimits.min })}
    bind:value={password}
    error={texts.password}
    autocomplete="new-password"
  />
  <div class="row">
    <button class="btn primary" type="submit" disabled={busy}>{m.users_create()}</button>
    <button class="btn" type="button" disabled={busy} onclick={oncancel}>{m.form_cancel()}</button>
  </div>
</form>

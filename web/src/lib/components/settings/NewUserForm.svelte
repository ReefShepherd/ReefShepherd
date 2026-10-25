<script lang="ts">
  import { errorText } from '#lib/api/error-text.js';
  import { fieldText } from '#lib/api/field-errors.js';
  import { usersApi, type Role, type User } from '#lib/api/users.js';
  import {
    collectErrors,
    passwordLimits,
    validateDisplayName,
    validateNewPassword,
    validateOptionalEmail,
    validateUsername
  } from '#lib/auth/validation.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import SelectField from '#lib/components/forms/SelectField.svelte';
  import TextField from '#lib/components/forms/TextField.svelte';
  import { focusFirstField, focusFirstInvalid } from '#lib/dom.js';
  import { m } from '#lib/paraglide/messages.js';

  interface Props {
    oncreated: (user: User) => void;
    oncancel: () => void;
  }

  let { oncreated, oncancel }: Props = $props();

  type Field = 'username' | 'displayName' | 'email' | 'password';

  let username = $state('');
  let displayName = $state('');
  let email = $state('');
  let role = $state<Role>('member');
  let password = $state('');
  let errors = $state<Partial<Record<Field, string>>>({});
  let failure = $state('');
  let busy = $state(false);
  let form = $state<HTMLFormElement>();

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    failure = '';
    errors = collectErrors({
      username: validateUsername(username),
      displayName: validateDisplayName(displayName),
      email: validateOptionalEmail(email),
      password: validateNewPassword(password)
    });
    if (Object.keys(errors).length > 0) return focusFirstInvalid(form);

    busy = true;
    try {
      const user = await usersApi.create({
        username: username.trim(),
        display_name: displayName.trim(),
        email: email.trim() === '' ? null : email.trim(),
        role,
        password
      });
      oncreated(user);
    } catch (e) {
      errors = {
        username: fieldText(e, 'username'),
        displayName: fieldText(e, 'display_name'),
        email: fieldText(e, 'email'),
        password: fieldText(e, 'password')
      };
      if (!Object.values(errors).some(Boolean)) failure = errorText(e);
      await focusFirstInvalid(form);
    } finally {
      busy = false;
    }
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
    error={errors.username}
    autocomplete="off"
    autocapitalize="none"
    spellcheck={false}
  />
  <TextField
    label={m.users_display_name()}
    bind:value={displayName}
    error={errors.displayName}
    autocomplete="off"
  />
  <TextField
    label={m.users_email()}
    type="email"
    bind:value={email}
    error={errors.email}
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
    error={errors.password}
    autocomplete="new-password"
  />
  <div class="row">
    <button class="btn primary" type="submit" disabled={busy}>{m.users_create()}</button>
    <button class="btn" type="button" disabled={busy} onclick={oncancel}>{m.form_cancel()}</button>
  </div>
</form>

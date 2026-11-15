<script lang="ts">
  import { errorText } from '#lib/api/error-text.js';
  import { fieldText } from '#lib/api/field-errors.js';
  import { guarded } from '#lib/api/guarded.js';
  import { usersApi, type User } from '#lib/api/users.js';
  import {
    collectErrors,
    passwordLimits,
    validateDisplayName,
    validateNewPassword,
    validateOptionalEmail
  } from '#lib/auth/validation.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import ConfirmButton from '#lib/components/forms/ConfirmButton.svelte';
  import SelectField from '#lib/components/forms/SelectField.svelte';
  import TextField from '#lib/components/forms/TextField.svelte';
  import { focusFirstField, focusFirstInvalid } from '#lib/dom.js';
  import { m } from '#lib/paraglide/messages.js';
  import { draftFromUser, userChanges, userLabel } from '#lib/users/rules.js';

  interface Props {
    user: User;
    isSelf: boolean;
    /** True when this is the only enabled administrator. */
    isLastAdmin: boolean;
    onchanged: (user: User, message: string) => void;
    onremoved: (user: User) => void;
  }

  let { user, isSelf, isLastAdmin, onchanged, onremoved }: Props = $props();

  type Field = 'displayName' | 'email' | 'password';

  let editing = $state(false);
  // The draft is rebuilt from the current user each time editing starts.
  // svelte-ignore state_referenced_locally
  let draft = $state(draftFromUser(user));
  let errors = $state<Partial<Record<Field, string>>>({});
  let failure = $state('');
  let busy = $state(false);
  let form = $state<HTMLFormElement>();

  const name = $derived(userLabel(user));
  const roleLocked = $derived(isLastAdmin);
  const disableLocked = $derived(isSelf || isLastAdmin);

  function startEditing() {
    draft = draftFromUser(user);
    errors = {};
    failure = '';
    editing = true;
  }

  async function save(event: SubmitEvent) {
    event.preventDefault();
    failure = '';
    errors = collectErrors({
      displayName: validateDisplayName(draft.displayName),
      email: validateOptionalEmail(draft.email),
      password: draft.password === '' ? null : validateNewPassword(draft.password)
    });
    if (Object.keys(errors).length > 0) return focusFirstInvalid(form);

    const changes = userChanges(user, draft);
    if (Object.keys(changes).length === 0) {
      editing = false;
      return;
    }
    busy = true;
    try {
      const saved = await usersApi.update(user.id, changes);
      editing = false;
      onchanged(saved, m.users_saved({ name: userLabel(saved) }));
    } catch (e) {
      errors = {
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

  const guard = (action: () => Promise<void>) =>
    guarded(action, { failure: (text) => (failure = text) });

  const resetMfa = () =>
    guard(async () => {
      await usersApi.resetMfa(user.id);
      onchanged({ ...user, totp_enabled: false }, m.users_reset_mfa_done({ name }));
    });

  const remove = () =>
    guard(async () => {
      await usersApi.remove(user.id);
      onremoved(user);
    });
</script>

<li class="card user">
  <div class="head">
    <div class="who">
      <strong>{name}</strong>
      <span class="muted mono">{user.username}</span>
    </div>
    <div class="row">
      <span class="badge accent">{user.role === 'admin' ? m.role_admin() : m.role_member()}</span>
      {#if isSelf}<span class="badge">{m.users_badge_you()}</span>{/if}
      {#if user.disabled}<span class="badge warning">{m.users_badge_disabled()}</span>{/if}
      {#if user.totp_enabled}<span class="badge">{m.users_badge_two_step()}</span>{/if}
      {#if !user.has_password}<span class="badge">{m.users_badge_no_password()}</span>{/if}
    </div>
  </div>

  {#if failure}
    <Alert kind="error">{failure}</Alert>
  {/if}

  {#if editing}
    <form class="stack" novalidate bind:this={form} onsubmit={save} use:focusFirstField>
      <TextField
        label={m.users_display_name()}
        bind:value={draft.displayName}
        error={errors.displayName}
        autocomplete="off"
      />
      <TextField
        label={m.users_email()}
        type="email"
        bind:value={draft.email}
        error={errors.email}
        autocomplete="off"
      />
      <SelectField
        label={m.users_role()}
        bind:value={draft.role}
        disabled={roleLocked}
        hint={roleLocked ? m.users_last_admin() : undefined}
        options={[
          { value: 'member', label: m.role_member() },
          { value: 'admin', label: m.role_admin() }
        ]}
      />
      <label class="choice">
        <input type="checkbox" bind:checked={draft.disabled} disabled={disableLocked} />
        <span class="choice-text">
          <span>{m.users_disabled()}</span>
          {#if isSelf}<span class="hint">{m.users_self_note()}</span>{/if}
        </span>
      </label>
      <TextField
        label={m.users_new_password()}
        type="password"
        hint={m.password_hint({ min: passwordLimits.min })}
        bind:value={draft.password}
        error={errors.password}
        autocomplete="new-password"
      />
      <div class="row">
        <button class="btn primary" type="submit" disabled={busy}>{m.form_save()}</button>
        <button class="btn" type="button" disabled={busy} onclick={() => (editing = false)}>
          {m.form_cancel()}
        </button>
      </div>
    </form>
  {:else}
    <div class="row actions">
      <button type="button" class="btn" onclick={startEditing}>
        {m.form_edit()}<span class="sr-only"> {name}</span>
      </button>
      {#if user.totp_enabled}
        <ConfirmButton
          label={m.users_reset_mfa()}
          question={m.users_reset_mfa_question({ name })}
          confirmLabel={m.users_reset_mfa()}
          onconfirm={resetMfa}
        />
      {/if}
      {#if !isSelf && !isLastAdmin}
        <ConfirmButton
          label={m.form_delete()}
          question={m.users_delete_question({ name })}
          confirmLabel={m.form_delete()}
          onconfirm={remove}
        />
      {/if}
    </div>
  {/if}
</li>

<style>
  .user {
    margin: 0;
  }

  .head {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
    margin-bottom: var(--space-2);
  }

  .who {
    display: flex;
    flex-direction: column;
    min-width: 0;
    overflow-wrap: anywhere;
  }

  .actions {
    margin-top: var(--space-2);
  }
</style>

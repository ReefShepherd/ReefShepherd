<script lang="ts">
  import { goto } from '$app/navigation';
  import { authApi } from '#lib/api/auth.js';
  import { errorText } from '#lib/api/error-text.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import PasswordForm from '#lib/components/settings/PasswordForm.svelte';
  import SessionList from '#lib/components/settings/SessionList.svelte';
  import TotpSection from '#lib/components/settings/TotpSection.svelte';
  import PageTitle from '#lib/components/shell/PageTitle.svelte';
  import { m } from '#lib/paraglide/messages.js';
  import { session } from '#lib/session/session.svelte.js';
  import { userLabel } from '#lib/users/rules.js';

  let sessionsVersion = $state(0);
  let failure = $state('');

  const me = $derived(session.me);
  const loginOn = $derived(me?.instance.auth_mode !== 'disabled');

  async function signOut() {
    failure = '';
    try {
      await authApi.logout();
      session.clear();
      await goto('/login');
    } catch (e) {
      failure = errorText(e);
    }
  }
</script>

<PageTitle title={m.settings_nav_account()} />

{#if me === null}
  <p class="muted" role="status">{m.loading()}</p>
{:else}
  <section class="card stack" aria-labelledby="account-heading">
    <div>
      <h2 id="account-heading">{m.settings_nav_account()}</h2>
      <p>
        {m.account_signed_in_as({ name: userLabel(me) })}
        <span class="badge">{me.role === 'admin' ? m.role_admin() : m.role_member()}</span>
      </p>
    </div>
    {#if failure}
      <Alert kind="error">{failure}</Alert>
    {/if}
    {#if loginOn}
      <div>
        <button type="button" class="btn" onclick={signOut}>{m.sign_out()}</button>
      </div>
    {/if}
  </section>

  <PasswordForm onchanged={() => (sessionsVersion += 1)} />
  <TotpSection />
  {#if loginOn}
    <SessionList version={sessionsVersion} />
  {/if}
{/if}

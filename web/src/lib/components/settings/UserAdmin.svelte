<script lang="ts">
  import { onMount } from 'svelte';
  import { errorText } from '#lib/api/error-text.js';
  import { usersApi, type User } from '#lib/api/users.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import { m } from '#lib/paraglide/messages.js';
  import { session } from '#lib/session/session.svelte.js';
  import { isLastEnabledAdmin, userLabel } from '#lib/users/rules.js';
  import NewUserForm from './NewUserForm.svelte';
  import UserRow from './UserRow.svelte';

  let users = $state<User[] | null>(null);
  let failure = $state('');
  let notice = $state('');
  let adding = $state(false);

  onMount(() => {
    usersApi
      .list()
      .then((list) => (users = list))
      .catch((e: unknown) => (failure = errorText(e)));
  });

  // Changing your own account can change what you are allowed to see, so the cached user is refreshed.
  function refreshSelf(id: number) {
    if (id === session.me?.id) void session.load().catch(() => {});
  }

  function startAdding() {
    adding = true;
    notice = '';
  }

  function created(user: User) {
    users = [...(users ?? []), user];
    adding = false;
    notice = m.users_created({ name: userLabel(user) });
  }

  function changed(user: User, message: string) {
    users = (users ?? []).map((u) => (u.id === user.id ? user : u));
    notice = message;
    refreshSelf(user.id);
  }

  function removed(user: User) {
    users = (users ?? []).filter((u) => u.id !== user.id);
    notice = m.users_deleted({ name: userLabel(user) });
  }
</script>

<div class="stack">
  <div class="row head">
    <p class="hint grow">{m.users_intro()}</p>
    {#if !adding}
      <button type="button" class="btn primary" onclick={startAdding}>
        {m.users_add()}
      </button>
    {/if}
  </div>

  {#if failure}
    <Alert kind="error">{failure}</Alert>
  {/if}
  {#if notice}
    <Alert kind="success">{notice}</Alert>
  {/if}

  {#if adding}
    <NewUserForm oncreated={created} oncancel={() => (adding = false)} />
  {/if}

  {#if users === null}
    {#if !failure}<p class="muted" role="status">{m.loading()}</p>{/if}
  {:else}
    <ul class="list">
      {#each users as user (user.id)}
        <UserRow
          {user}
          isSelf={user.id === session.me?.id}
          isLastAdmin={isLastEnabledAdmin(users, user.id)}
          onchanged={changed}
          onremoved={removed}
        />
      {/each}
    </ul>
  {/if}
</div>

<style>
  .head {
    justify-content: space-between;
  }

  .grow {
    flex: 1 1 14rem;
  }

  .list {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    margin: 0;
    padding: 0;
    list-style: none;
  }
</style>

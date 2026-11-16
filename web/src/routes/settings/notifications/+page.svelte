<script lang="ts">
  import { onMount } from 'svelte';
  import { errorText } from '#lib/api/error-text.js';
  import { notifyApi, type Channel } from '#lib/api/notify.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import ChannelForm from '#lib/components/notifications/ChannelForm.svelte';
  import ChannelList from '#lib/components/notifications/ChannelList.svelte';
  import AdminOnly from '#lib/components/settings/AdminOnly.svelte';
  import PageTitle from '#lib/components/shell/PageTitle.svelte';
  import { channelsFor } from '#lib/notifications/channels.js';
  import { m } from '#lib/paraglide/messages.js';
  import { session } from '#lib/session/session.svelte.js';

  const docsUrl = 'https://github.com/ReefShepherd/ReefShepherd/blob/main/docs/notifications.md';

  let channels = $state<Channel[] | null>(null);
  let loadError = $state('');
  let adding = $state(false);

  // Members get no channels from the server, so only an administrator asks.
  onMount(() => {
    const controller = new AbortController();
    session
      .ready()
      .then((me) => channelsFor(me, notifyApi, controller.signal))
      .then((list) => (channels = list))
      .catch((e: unknown) => {
        if (!controller.signal.aborted) loadError = errorText(e);
      });
    return () => controller.abort();
  });

  function added(channel: Channel) {
    adding = false;
    channels = [...(channels ?? []), channel];
  }
</script>

<PageTitle title={m.settings_nav_notifications()} />

<AdminOnly>
  <section class="stack" aria-labelledby="channels-heading">
    <div>
      <h2 id="channels-heading">{m.settings_nav_notifications()}</h2>
      <p>{m.channels_intro()}</p>
      <p>
        <a href={docsUrl} target="_blank" rel="noopener noreferrer">{m.channels_docs()}</a>
      </p>
    </div>

    {#if loadError}
      <Alert kind="error">{loadError}</Alert>
    {:else if channels === null}
      <p class="muted" role="status">{m.loading()}</p>
    {:else}
      {#if channels.length === 0 && !adding}
        <p class="muted">{m.channels_empty()}</p>
      {/if}
      <ChannelList {channels} onchange={(next) => (channels = next)} />
      {#if adding}
        <ChannelForm onsaved={added} oncancel={() => (adding = false)} />
      {:else}
        <div>
          <button type="button" class="btn primary" onclick={() => (adding = true)}>
            {m.channels_add()}
          </button>
        </div>
      {/if}
    {/if}
  </section>
</AdminOnly>

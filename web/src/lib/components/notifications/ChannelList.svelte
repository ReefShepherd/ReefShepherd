<script lang="ts">
  import { errorText } from '#lib/api/error-text.js';
  import { notifyApi, type Channel, type TestSend } from '#lib/api/notify.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import ConfirmButton from '#lib/components/forms/ConfirmButton.svelte';
  import { relativeTime } from '#lib/format/datetime.js';
  import { removeChannel, replaceChannel } from '#lib/notifications/channels.js';
  import { eventLabel, kindLabel } from '#lib/notifications/labels.js';
  import { m } from '#lib/paraglide/messages.js';
  import ChannelForm from './ChannelForm.svelte';

  interface Props {
    channels: Channel[];
    onchange: (channels: Channel[]) => void;
  }

  let { channels, onchange }: Props = $props();

  let editingId = $state<number | null>(null);
  let busyId = $state<number | null>(null);
  let failure = $state('');
  // Test results stay on screen until the next test of the same channel.
  let tests = $state<Record<number, TestSend>>({});

  async function run(id: number, action: () => Promise<void>) {
    failure = '';
    busyId = id;
    try {
      await action();
    } catch (e) {
      failure = errorText(e);
    } finally {
      busyId = null;
    }
  }

  const toggle = (channel: Channel, enabled: boolean) =>
    run(channel.id, async () => {
      onchange(replaceChannel(channels, await notifyApi.updateChannel(channel.id, { enabled })));
    });

  const sendTest = (channel: Channel) =>
    run(channel.id, async () => {
      tests[channel.id] = await notifyApi.testChannel(channel.id);
      // A test changes the channel's last send or last error.
      const fresh = (await notifyApi.channels()).find((c) => c.id === channel.id);
      if (fresh) onchange(replaceChannel(channels, fresh));
    });

  const remove = (channel: Channel) =>
    run(channel.id, async () => {
      await notifyApi.removeChannel(channel.id);
      onchange(removeChannel(channels, channel.id));
    });

  function saved(channel: Channel) {
    editingId = null;
    onchange(replaceChannel(channels, channel));
  }
</script>

{#if failure}
  <Alert kind="error">{failure}</Alert>
{/if}

<ul class="list">
  {#each channels as channel (channel.id)}
    <li>
      {#if editingId === channel.id}
        <ChannelForm {channel} onsaved={saved} oncancel={() => (editingId = null)} />
      {:else}
        <section class="card stack" aria-labelledby="channel-{channel.id}">
          <div class="head">
            <h3 id="channel-{channel.id}">{channel.name}</h3>
            <span class="badge accent">{kindLabel(channel.kind)}</span>
            {#if !channel.enabled}<span class="badge">{m.channel_off()}</span>{/if}
          </div>

          <p class="hint">
            {channel.events.map(eventLabel).join(', ')}
          </p>

          <p class="hint">
            {#if channel.last_sent_at}
              {m.channel_last_sent({ when: relativeTime(channel.last_sent_at) })}
            {:else}
              {m.channel_never_sent()}
            {/if}
          </p>

          {#if channel.last_error}
            <Alert kind="warning">
              {m.channel_last_error({
                when: channel.last_error_at ? relativeTime(channel.last_error_at) : '',
                error: channel.last_error
              })}
            </Alert>
          {/if}

          {#if tests[channel.id]}
            {#if tests[channel.id].sent}
              <Alert kind="success">{m.channel_test_sent()}</Alert>
            {:else}
              <Alert kind="error">{m.channel_test_failed({ error: tests[channel.id].error })}</Alert
              >
            {/if}
          {/if}

          <label class="choice">
            <input
              type="checkbox"
              checked={channel.enabled}
              disabled={busyId === channel.id}
              onchange={(e) => toggle(channel, e.currentTarget.checked)}
            />
            <span class="choice-text">{m.channel_enabled()}</span>
          </label>

          <div class="row">
            <button
              type="button"
              class="btn"
              disabled={busyId === channel.id}
              onclick={() => (editingId = channel.id)}
            >
              {m.form_edit()}
            </button>
            <button
              type="button"
              class="btn"
              disabled={busyId === channel.id}
              onclick={() => sendTest(channel)}
            >
              {m.channel_test()}
            </button>
            <ConfirmButton
              label={m.channel_delete()}
              question={m.channel_delete_question({ name: channel.name })}
              confirmLabel={m.channel_delete_confirm()}
              disabled={busyId === channel.id}
              onconfirm={() => remove(channel)}
            />
          </div>
        </section>
      {/if}
    </li>
  {/each}
</ul>

<style>
  .list {
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .head {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
  }

  .head h3 {
    margin: 0;
    overflow-wrap: anywhere;
  }
</style>

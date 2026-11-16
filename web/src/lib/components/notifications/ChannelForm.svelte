<script lang="ts">
  import { untrack } from 'svelte';
  import {
    channelKinds,
    eventTypes,
    notifyApi,
    type Channel,
    type ChannelKind,
    type EmailSecurity
  } from '#lib/api/notify.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import SelectField from '#lib/components/forms/SelectField.svelte';
  import TextField from '#lib/components/forms/TextField.svelte';
  import { focusFirstInvalid } from '#lib/dom.js';
  import { hasErrors, type FieldErrors } from '#lib/forms/codes.js';
  import { guarded, type GuardHooks } from '#lib/forms/guarded.js';
  import { fieldTexts } from '#lib/forms/serverErrors.js';
  import {
    changeKind,
    channelChanges,
    channelNameMax,
    draftFromChannel,
    emptyDraft,
    newChannel,
    shownFields,
    toggleEvent,
    validateDraft
  } from '#lib/notifications/channel-draft.js';
  import { eventLabel, kindLabel } from '#lib/notifications/labels.js';
  import { m } from '#lib/paraglide/messages.js';
  import SecretField from './SecretField.svelte';

  interface Props {
    /** The channel being edited; left out to add a new one. */
    channel?: Channel;
    onsaved: (saved: Channel) => void;
    oncancel: () => void;
  }

  let { channel, onsaved, oncancel }: Props = $props();

  let draft = $state(untrack(() => (channel ? draftFromChannel(channel) : emptyDraft('discord'))));
  let kind = $state<ChannelKind>(untrack(() => draft.kind));
  let errors = $state<FieldErrors>({});
  let failure = $state('');
  let busy = $state(false);
  let form = $state<HTMLFormElement>();
  const texts = $derived(fieldTexts(errors, { name: channelNameMax }));

  // Choosing another kind starts that kind's settings from blank.
  $effect(() => {
    const chosen = kind;
    untrack(() => {
      if (chosen !== draft.kind) draft = changeKind(draft, chosen);
    });
  });

  const securityOptions: { value: EmailSecurity; label: string }[] = [
    { value: 'starttls', label: m.channel_email_starttls() },
    { value: 'tls', label: m.channel_email_tls() },
    { value: 'none', label: m.channel_email_none() }
  ];

  const aboutText: Record<ChannelKind, () => string> = {
    discord: m.channel_about_discord,
    slack: m.channel_about_slack,
    ntfy: m.channel_about_ntfy,
    webhook: m.channel_about_webhook,
    email: m.channel_about_email
  };

  const hooks: GuardHooks = {
    failure: (text) => (failure = text),
    busy: (b) => (busy = b),
    fields: (f) => (errors = f),
    get shown() {
      return shownFields(draft.kind);
    },
    form: () => form
  };

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    errors = validateDraft(draft);
    if (hasErrors(errors)) return focusFirstInvalid(form);
    await guarded(async () => {
      const saved = channel
        ? await notifyApi.updateChannel(channel.id, channelChanges(channel, draft))
        : await notifyApi.createChannel(newChannel(draft));
      onsaved(saved);
    }, hooks);
  }
</script>

<form class="card stack" novalidate bind:this={form} onsubmit={submit}>
  <h3>{channel ? m.channel_edit_title({ name: channel.name }) : m.channel_new_title()}</h3>

  {#if failure}
    <Alert kind="error">{failure}</Alert>
  {/if}

  <SelectField
    label={m.channel_kind()}
    bind:value={kind}
    options={channelKinds.map((k) => ({ value: k, label: kindLabel(k) }))}
    disabled={channel !== undefined}
    hint={aboutText[draft.kind]()}
  />

  <TextField
    label={m.channel_name()}
    bind:value={draft.name}
    error={texts.name}
    autocomplete="off"
  />

  {#if draft.kind === 'discord'}
    <SecretField
      label={m.channel_discord_url()}
      bind:secret={draft.url}
      hint={m.channel_discord_url_hint()}
      error={texts['config.url']}
    />
  {:else if draft.kind === 'slack'}
    <SecretField
      label={m.channel_slack_url()}
      bind:secret={draft.url}
      hint={m.channel_slack_url_hint()}
      error={texts['config.url']}
    />
  {:else if draft.kind === 'webhook'}
    <SecretField
      label={m.channel_webhook_url()}
      bind:secret={draft.url}
      hint={m.channel_webhook_url_hint()}
      error={texts['config.url']}
    />
  {:else if draft.kind === 'ntfy'}
    <TextField
      label={m.channel_ntfy_server()}
      bind:value={draft.server}
      hint={m.channel_ntfy_server_hint()}
      error={texts['config.server']}
      type="url"
      autocomplete="off"
    />
    <SecretField
      label={m.channel_ntfy_topic()}
      bind:secret={draft.topic}
      hint={m.channel_ntfy_topic_hint()}
      error={texts['config.topic']}
    />
    <SecretField
      label={m.channel_ntfy_token()}
      bind:secret={draft.token}
      hint={m.channel_ntfy_token_hint()}
      error={texts['config.token']}
      removeLabel={m.channel_token_remove()}
    />
  {:else}
    <TextField
      label={m.channel_email_host()}
      bind:value={draft.host}
      hint={m.channel_email_host_hint()}
      error={texts['config.host']}
      autocomplete="off"
    />
    <TextField
      label={m.channel_email_port()}
      bind:value={draft.port}
      hint={m.channel_email_port_hint()}
      error={texts['config.port']}
      inputmode="numeric"
      autocomplete="off"
    />
    <SelectField
      label={m.channel_email_security()}
      bind:value={draft.security}
      options={securityOptions}
      hint={m.channel_email_security_hint()}
      error={texts['config.security']}
    />
    <TextField
      label={m.channel_email_username()}
      bind:value={draft.username}
      error={texts['config.username']}
      autocomplete="off"
    />
    <SecretField
      label={m.channel_email_password()}
      bind:secret={draft.password}
      hint={m.channel_email_password_hint()}
      error={texts['config.password']}
      removeLabel={m.channel_password_remove()}
    />
    <TextField
      label={m.channel_email_from()}
      bind:value={draft.from}
      hint={m.channel_email_from_hint()}
      error={texts['config.from']}
      autocomplete="off"
    />
    <TextField
      label={m.channel_email_to()}
      bind:value={draft.to}
      hint={m.channel_email_to_hint()}
      error={texts['config.to']}
      autocomplete="off"
    />
  {/if}

  <fieldset class="stack" aria-describedby="channel-events-hint">
    <legend>{m.channel_events()}</legend>
    <p class="hint" id="channel-events-hint">{m.channel_events_hint()}</p>
    {#each eventTypes as type (type)}
      <label class="choice">
        <input
          type="checkbox"
          checked={draft.events.includes(type)}
          onchange={(e) =>
            (draft.events = toggleEvent(draft.events, type, e.currentTarget.checked))}
        />
        <span class="choice-text">{eventLabel(type)}</span>
      </label>
    {/each}
    {#if texts.events}<p class="field-error">{texts.events}</p>{/if}
  </fieldset>

  <label class="choice">
    <input type="checkbox" bind:checked={draft.enabled} />
    <span class="choice-text">
      <span>{m.channel_enabled()}</span>
      <span class="hint">{m.channel_enabled_hint()}</span>
    </span>
  </label>

  <div class="row">
    <button class="btn primary" type="submit" disabled={busy}>{m.channel_save()}</button>
    <button class="btn" type="button" disabled={busy} onclick={oncancel}>{m.form_cancel()}</button>
  </div>
</form>

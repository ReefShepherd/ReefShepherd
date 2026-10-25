<script lang="ts">
  import { errorText } from '#lib/api/error-text.js';
  import { fieldText } from '#lib/api/field-errors.js';
  import { tanksApi, type Tank } from '#lib/api/tanks.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import TextField from '#lib/components/forms/TextField.svelte';
  import { m } from '#lib/paraglide/messages.js';

  interface Props {
    tank: Tank;
    ondeleted: () => void;
  }

  let { tank, ondeleted }: Props = $props();

  let typed = $state('');
  let error = $state<string>();
  let failure = $state('');
  let busy = $state(false);

  async function remove(event: SubmitEvent) {
    event.preventDefault();
    failure = '';
    error = undefined;
    busy = true;
    try {
      await tanksApi.remove(tank.id, typed);
      ondeleted();
    } catch (e) {
      error = fieldText(e, 'confirm');
      if (error === undefined) failure = errorText(e);
    } finally {
      busy = false;
    }
  }
</script>

<form class="card stack" onsubmit={remove}>
  <h2>{m.tanks_delete_title()}</h2>
  <p>{m.tanks_delete_warning({ name: tank.name })}</p>
  {#if failure}
    <Alert kind="error">{failure}</Alert>
  {/if}
  <TextField
    label={m.tanks_delete_confirm_label({ name: tank.name })}
    bind:value={typed}
    {error}
    autocomplete="off"
  />
  <div>
    <button class="btn danger" type="submit" disabled={busy || typed !== tank.name}>
      {m.tanks_delete_button()}
    </button>
  </div>
</form>

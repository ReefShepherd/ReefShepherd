<script lang="ts">
  import { tanksApi, type Tank } from '#lib/api/tanks.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import TextField from '#lib/components/forms/TextField.svelte';
  import type { FieldErrors } from '#lib/forms/codes.js';
  import { guarded } from '#lib/forms/guarded.js';
  import { fieldTexts } from '#lib/forms/serverErrors.js';
  import { m } from '#lib/paraglide/messages.js';

  interface Props {
    tank: Tank;
    ondeleted: () => void;
  }

  let { tank, ondeleted }: Props = $props();

  let typed = $state('');
  let errors = $state<FieldErrors>({});
  const texts = $derived(fieldTexts(errors));
  let failure = $state('');
  let busy = $state(false);

  const remove = (event: SubmitEvent) => {
    event.preventDefault();
    return guarded(
      async () => {
        await tanksApi.remove(tank.id, typed);
        ondeleted();
      },
      {
        failure: (text) => (failure = text),
        busy: (b) => (busy = b),
        fields: (f) => (errors = f),
        shown: ['confirm']
      }
    );
  };
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
    error={texts.confirm}
    autocomplete="off"
  />
  <div>
    <button class="btn danger" type="submit" disabled={busy || typed !== tank.name}>
      {m.tanks_delete_button()}
    </button>
  </div>
</form>

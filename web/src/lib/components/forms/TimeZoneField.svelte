<script lang="ts">
  import { timeZoneNames } from '#lib/format/timezone.js';
  import TextField from './TextField.svelte';

  interface Props {
    label: string;
    value: string;
    hint?: string;
    error?: string;
    disabled?: boolean;
  }

  let { label, value = $bindable(), hint, error, disabled = false }: Props = $props();

  const listId = $props.id();
  const names = timeZoneNames();
</script>

<TextField
  {label}
  {hint}
  {error}
  {disabled}
  bind:value
  list={listId}
  autocomplete="off"
  autocapitalize="none"
  spellcheck={false}
/>
<datalist id={listId}>
  {#each names as name (name)}
    <option value={name}></option>
  {/each}
</datalist>

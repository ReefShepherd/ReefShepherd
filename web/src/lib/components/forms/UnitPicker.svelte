<script lang="ts">
  import type { UnitFamily } from '#lib/api/settings.js';
  import { familyLabel, selectableFamilies } from '#lib/units/catalog.js';
  import SelectField from './SelectField.svelte';

  interface Props {
    catalog: readonly UnitFamily[];
    /** The chosen unit code for each family. */
    value: Record<string, string>;
    disabled?: boolean;
  }

  let { catalog, value = $bindable(), disabled = false }: Props = $props();

  const families = $derived(selectableFamilies(catalog));
</script>

<div class="grid">
  {#each families as family (family.family)}
    <SelectField
      label={familyLabel(family.family)}
      bind:value={value[family.family]}
      {disabled}
      options={family.units.map((u) => ({ value: u.unit, label: u.label }))}
    />
  {/each}
</div>

<style>
  .grid {
    display: grid;
    gap: var(--space-3);
    grid-template-columns: repeat(auto-fit, minmax(10rem, 1fr));
  }
</style>

<script lang="ts">
  import AdminOnly from '#lib/components/settings/AdminOnly.svelte';
  import InstanceForm from '#lib/components/settings/InstanceForm.svelte';
  import PageTitle from '#lib/components/shell/PageTitle.svelte';
  import { m } from '#lib/paraglide/messages.js';
</script>

<PageTitle title={m.settings_nav_instance()} />

<AdminOnly>
  <InstanceForm />
</AdminOnly>

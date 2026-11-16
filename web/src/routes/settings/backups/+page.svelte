<script lang="ts">
  import AdminOnly from '#lib/components/settings/AdminOnly.svelte';
  import PageTitle from '#lib/components/shell/PageTitle.svelte';
  import { m } from '#lib/paraglide/messages.js';
</script>

<PageTitle title={m.backups_title()} />

<AdminOnly>
  <h2>{m.backups_title()}</h2>
</AdminOnly>

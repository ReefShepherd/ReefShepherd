<script lang="ts">
  import AdminOnly from '#lib/components/settings/AdminOnly.svelte';
  import PageTitle from '#lib/components/shell/PageTitle.svelte';
  import { m } from '#lib/paraglide/messages.js';
</script>

<PageTitle title={m.settings_nav_notifications()} />

<AdminOnly>
  <h2>{m.settings_nav_notifications()}</h2>
</AdminOnly>

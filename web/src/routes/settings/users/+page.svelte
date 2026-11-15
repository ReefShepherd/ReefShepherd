<script lang="ts">
  import AdminOnly from '#lib/components/settings/AdminOnly.svelte';
  import UserAdmin from '#lib/components/settings/UserAdmin.svelte';
  import PageTitle from '#lib/components/shell/PageTitle.svelte';
  import { m } from '#lib/paraglide/messages.js';
</script>

<PageTitle title={m.settings_nav_users()} />

<AdminOnly>
  <UserAdmin />
</AdminOnly>

<script lang="ts">
  import BackupList from '#lib/components/backups/BackupList.svelte';
  import BackupSettingsForm from '#lib/components/backups/BackupSettingsForm.svelte';
  import Alert from '#lib/components/forms/Alert.svelte';
  import AdminOnly from '#lib/components/settings/AdminOnly.svelte';
  import PageTitle from '#lib/components/shell/PageTitle.svelte';
  import { m } from '#lib/paraglide/messages.js';
</script>

<PageTitle title={m.backups_title()} />

<AdminOnly>
  <div class="stack">
    <Alert kind="info">{m.backups_secrets_note()}</Alert>
    <BackupSettingsForm />
    <BackupList />
  </div>
</AdminOnly>

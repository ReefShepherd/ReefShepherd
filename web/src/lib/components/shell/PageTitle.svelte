<script lang="ts">
  import { m } from '#lib/paraglide/messages.js';

  let { title }: { title: string } = $props();
</script>

<svelte:head>
  <title>{m.page_title({ page: title, app: m.app_name() })}</title>
</svelte:head>

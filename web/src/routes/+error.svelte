<script lang="ts">
  import { page } from '$app/state';
  import PageTitle from '#lib/components/shell/PageTitle.svelte';
  import { m } from '#lib/paraglide/messages.js';

  const notFound = $derived(page.status === 404);
  const title = $derived(notFound ? m.error_not_found_title() : m.error_generic_title());
</script>

<PageTitle {title} />
<h1>{title}</h1>
<p class="muted">{notFound ? m.error_not_found_body() : m.error_generic_body()}</p>
<p><a href="/">{m.error_back_home()}</a></p>

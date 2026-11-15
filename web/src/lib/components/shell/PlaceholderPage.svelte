<script lang="ts">
  import { m } from '#lib/paraglide/messages.js';
  import PageTitle from './PageTitle.svelte';

  let { title }: { title: string } = $props();
</script>

<PageTitle {title} />
<h1>{title}</h1>
<p class="muted">{m.page_placeholder()}</p>

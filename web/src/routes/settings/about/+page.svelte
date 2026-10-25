<script lang="ts">
  import { onMount } from 'svelte';
  import PageTitle from '#lib/components/shell/PageTitle.svelte';
  import { m } from '#lib/paraglide/messages.js';

  const sourceUrl = 'https://github.com/ReefShepherd/ReefShepherd';

  let version = $state('');

  onMount(() => {
    fetch('/healthz', { headers: { Accept: 'application/json' } })
      .then((res) => res.json() as Promise<{ version?: unknown }>)
      .then((body) => {
        if (typeof body.version === 'string') version = body.version;
      })
      .catch(() => {});
  });
</script>

<PageTitle title={m.settings_nav_about()} />

<section class="card stack" aria-labelledby="about-heading">
  <div>
    <h2 id="about-heading">{m.app_name()}</h2>
    <p>{m.about_version({ version: version || m.about_unknown_version() })}</p>
  </div>
  <p>{m.about_license()}</p>
  <p>
    {m.about_source_intro()}
    <a href={sourceUrl} target="_blank" rel="noopener noreferrer">{m.about_source()}</a>
  </p>
</section>

<script lang="ts">
  import PageTitle from '#lib/components/shell/PageTitle.svelte';
  import ParameterDefinitions from '#lib/components/tanks/ParameterDefinitions.svelte';
  import { m } from '#lib/paraglide/messages.js';
</script>

<PageTitle title={m.params_def_title()} />

<h1>{m.params_def_title()}</h1>
<p><a href="/tanks">{m.tanks_back_to_list()}</a></p>
<ParameterDefinitions />

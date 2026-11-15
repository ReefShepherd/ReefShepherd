<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Props {
    kind?: 'error' | 'warning' | 'success' | 'info';
    children: Snippet;
  }

  let { kind = 'info', children }: Props = $props();
</script>

<!-- Errors interrupt the reader; the other kinds wait their turn. -->
<div
  class="alert {kind}"
  role={kind === 'error' ? 'alert' : kind === 'info' ? undefined : 'status'}
>
  {@render children()}
</div>

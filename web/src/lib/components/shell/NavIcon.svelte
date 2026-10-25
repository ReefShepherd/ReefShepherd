<script lang="ts">
  import type { NavIconName } from './nav.js';

  let { name }: { name: NavIconName } = $props();
</script>

<svg
  viewBox="0 0 24 24"
  width="24"
  height="24"
  fill="none"
  stroke="currentColor"
  stroke-width="1.8"
  stroke-linecap="round"
  stroke-linejoin="round"
  aria-hidden="true"
  focusable="false"
>
  {#if name === 'dashboard'}
    <rect x="3.5" y="3.5" width="7" height="7" rx="1.5" />
    <rect x="13.5" y="3.5" width="7" height="7" rx="1.5" />
    <rect x="3.5" y="13.5" width="7" height="7" rx="1.5" />
    <rect x="13.5" y="13.5" width="7" height="7" rx="1.5" />
  {:else if name === 'tanks'}
    <rect x="3" y="5" width="18" height="14" rx="2" />
    <path d="M3 12c2.5-2 4.5-2 6 0s3.5 2 6 0 4.5-2 6 0" />
  {:else if name === 'tests'}
    <path d="M9 3h6M10 3v6l-5 9a2 2 0 0 0 1.8 3h10.4A2 2 0 0 0 19 18l-5-9V3" />
    <path d="M7.5 15h9" />
  {:else if name === 'charts'}
    <path d="M3.5 3.5v17h17" />
    <path d="M7 15l4-5 3 3 5-6" />
  {:else if name === 'settings'}
    <path d="M4 7h9M17 7h3M4 17h3M11 17h9" />
    <circle cx="15" cy="7" r="2" />
    <circle cx="9" cy="17" r="2" />
  {/if}
</svg>

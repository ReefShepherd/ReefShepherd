<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { Card } from './cards.js';
  import { registry } from './registry.js';

  let { card, bar }: { card: Card; bar?: Snippet } = $props();

  const Body = $derived(registry[card.card_type]?.default);
</script>

<div class="frame" class:editing={bar}>
  {@render bar?.()}
  <!-- While editing, the card is a preview: its links and controls stay out of the tab order. -->
  <div class="body" inert={bar ? true : undefined}>
    {#if Body}
      <Body {card} />
    {/if}
  </div>
</div>

<style>
  .frame {
    height: 100%;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface);
  }

  .frame.editing {
    outline: 2px dashed var(--accent);
    outline-offset: 2px;
  }

  .body {
    padding: var(--space-3);
  }
</style>

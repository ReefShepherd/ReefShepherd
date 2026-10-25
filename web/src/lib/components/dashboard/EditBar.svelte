<script lang="ts">
  import { m } from '#lib/paraglide/messages.js';
  import { dragHandle } from 'svelte-dnd-action';
  import type { Card } from './cards.js';

  interface Props {
    card: Card;
    /** The card's name, which tells apart the same button on different cards. */
    name: string;
    first: boolean;
    last: boolean;
    /** The only card left cannot be removed. */
    only: boolean;
    onmove: (delta: number, action: 'up' | 'down') => void;
    ontoggle: () => void;
    onremove: () => void;
  }

  let { card, name, first, last, only, onmove, ontoggle, onremove }: Props = $props();
</script>

<div class="bar">
  <!-- A span, not a button: the library ignores a keyboard drag that starts on an element that can be disabled. -->
  <span class="grip" use:dragHandle aria-label={m.dashboard_grip({ name })}>&#x2807;</span>
  <span class="spacer"></span>
  <button
    type="button"
    class="tool"
    data-action="up"
    disabled={first}
    aria-label={m.params_move_up({ name })}
    onclick={() => onmove(-1, 'up')}
  >
    &#x2191;
  </button>
  <button
    type="button"
    class="tool"
    data-action="down"
    disabled={last}
    aria-label={m.params_move_down({ name })}
    onclick={() => onmove(1, 'down')}
  >
    &#x2193;
  </button>
  <button
    type="button"
    class="tool text"
    data-action="size"
    aria-pressed={card.size === 'wide'}
    onclick={ontoggle}
  >
    {m.dashboard_wide()}<span class="sr-only"> {name}</span>
  </button>
  <button
    type="button"
    class="tool"
    data-action="remove"
    disabled={only}
    aria-label={m.dashboard_remove({ name })}
    onclick={onremove}
  >
    &#x2715;
  </button>
</div>

<style>
  .bar {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    padding: var(--space-1);
    border-bottom: 1px solid var(--border);
  }

  .spacer {
    flex: 1;
  }

  .tool,
  .grip {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: var(--touch);
    min-height: var(--touch);
    font: inherit;
  }

  .tool {
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface);
    color: var(--text);
    cursor: pointer;
  }

  .tool.text {
    padding: 0 var(--space-2);
  }

  .tool:hover:not(:disabled) {
    background: var(--surface-hover);
  }

  .tool[aria-pressed='true'] {
    border-color: var(--accent);
    background: var(--accent-soft);
    font-weight: 600;
  }

  .tool:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .grip {
    cursor: grab;
    font-size: 1.4rem;
    /* The grip owns its touches, so dragging from it never scrolls the page. */
    touch-action: none;
    user-select: none;
    -webkit-user-select: none;
    -webkit-touch-callout: none;
  }
</style>

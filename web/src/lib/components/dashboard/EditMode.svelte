<script lang="ts">
  import { onMount, tick, untrack } from 'svelte';
  import { flip } from 'svelte/animate';
  import { dragHandleZone, type DndEvent } from 'svelte-dnd-action';
  import { isApiError } from '#lib/api/client.js';
  import { errorText } from '#lib/api/error-text.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import ConfirmButton from '#lib/components/forms/ConfirmButton.svelte';
  import { refocusMoveButton } from '#lib/dom.js';
  import { m } from '#lib/paraglide/messages.js';
  import CardFrame from './CardFrame.svelte';
  import {
    addCard,
    availableCards,
    moveCard,
    newCard,
    rejectedIndexes,
    removeCard,
    settledCards,
    toggleSize,
    type Card
  } from './cards.js';
  import { useDashboard } from './context.js';
  import { wireDndAria } from './dndAria.js';
  import EditBar from './EditBar.svelte';
  import { cardKinds, labelFor } from './registry.js';

  interface Props {
    cards: Card[];
    onsave: (cards: Card[]) => Promise<void>;
    onreset: () => Promise<void>;
    oncancel: () => void;
  }

  let { cards, onsave, onreset, oncancel }: Props = $props();

  const dash = useDashboard();

  // Plain arrays replaced on every change: the drag library copies and compares them, so no deep proxy is wanted.
  let draft = $state.raw<Card[]>(untrack(() => cards));
  let chosen = $state('');
  let failure = $state('');
  let busy = $state(false);
  let rejected = $state.raw<string[]>([]);
  let nextId = 0;

  wireDndAria();

  // An empty dashboard has nothing to arrange, so the first thing to do is add a card.
  onMount(() => {
    if (draft.length === 0) document.getElementById('dash-add')?.focus();
  });

  const reduceMotion = matchMedia('(prefers-reduced-motion: reduce)').matches;
  const flipDurationMs = reduceMotion ? 0 : 200;

  // The library inserts a placeholder copy of the dragged card while a drag is in flight.
  const settled = $derived(settledCards(draft));

  const name = (card: Card) => dash.parameter(card.parameter_id)?.name ?? null;
  const label = (card: Card) => labelFor(card, name(card));

  const enabledIds = $derived(dash.parameters.filter((p) => p.enabled).map((p) => p.parameter_id));
  const choices = $derived(availableCards(settled, cardKinds, enabledIds));
  const choiceLabel = (c: { card_type: string; parameter_id: number | null }) =>
    labelFor(c, dash.parameter(c.parameter_id)?.name ?? null);

  function onDnd(e: CustomEvent<DndEvent<Card>>) {
    draft = e.detail.items;
  }

  async function move(card: Card, delta: number, action: 'up' | 'down') {
    draft = moveCard(draft, card.id, delta);
    await tick();
    // Moving a keyed node can drop focus, so it goes back to the button that was used.
    refocusMoveButton(document.querySelector(`[data-card-id="${card.id}"]`), action);
  }

  function add() {
    const choice = choices.find((c) => c.key === chosen);
    if (!choice) return;
    draft = addCard(draft, newCard(choice, `n${nextId++}`));
    chosen = '';
  }

  async function run(action: () => Promise<void>) {
    busy = true;
    failure = '';
    rejected = [];
    const sent = settled;
    try {
      await action();
    } catch (e) {
      failure = errorText(e);
      if (isApiError(e)) {
        rejected = rejectedIndexes(e.fields).flatMap((i) => (sent[i] ? [sent[i].id] : []));
      }
      busy = false;
    }
  }
</script>

<div class="stack">
  <p class="hint">{m.dashboard_edit_intro()}</p>

  {#if failure}
    <Alert kind="error">{failure}</Alert>
  {/if}

  <div class="row toolbar">
    <div class="field add">
      <label for="dash-add">{m.dashboard_add_label()}</label>
      <select id="dash-add" bind:value={chosen} disabled={choices.length === 0}>
        <option value="">
          {choices.length === 0 ? m.dashboard_add_none() : m.dashboard_add_placeholder()}
        </option>
        {#each choices as c (c.key)}
          <option value={c.key}>{choiceLabel(c)}</option>
        {/each}
      </select>
    </div>
    <button type="button" class="btn" disabled={chosen === ''} onclick={add}>
      {m.dashboard_add()}
    </button>
    <span class="spacer"></span>
    <button
      type="button"
      class="btn primary"
      disabled={busy || settled.length === 0}
      onclick={() => run(() => onsave(settled))}
    >
      {m.dashboard_save()}
    </button>
    <button type="button" class="btn" disabled={busy} onclick={oncancel}>
      {m.form_cancel()}
    </button>
    <ConfirmButton
      label={m.dashboard_reset()}
      question={m.dashboard_reset_question()}
      confirmLabel={m.dashboard_reset_confirm()}
      disabled={busy}
      onconfirm={() => run(onreset)}
    />
  </div>

  {#if settled.length === 1}
    <p class="hint">{m.dashboard_last_card()}</p>
  {/if}

  <div
    class="dash-grid"
    aria-label={m.dashboard_cards_label()}
    use:dragHandleZone={{ items: draft, flipDurationMs, dropTargetStyle: {} }}
    onconsider={onDnd}
    onfinalize={onDnd}
  >
    {#each draft as card, i (card.id)}
      <div
        class="dash-slot"
        class:wide={card.size === 'wide'}
        data-card-id={card.id}
        aria-label={label(card)}
        animate:flip={{ duration: flipDurationMs }}
      >
        <CardFrame {card}>
          {#snippet bar()}
            <EditBar
              {card}
              name={label(card)}
              first={i === 0}
              last={i === draft.length - 1}
              only={settled.length <= 1}
              onmove={(delta, action) => move(card, delta, action)}
              ontoggle={() => (draft = toggleSize(draft, card.id))}
              onremove={() => (draft = removeCard(draft, card.id))}
            />
          {/snippet}
        </CardFrame>
        {#if rejected.includes(card.id)}
          <p class="field-error" role="alert">{m.dashboard_card_rejected()}</p>
        {/if}
      </div>
    {/each}
  </div>
</div>

<style>
  .toolbar {
    align-items: flex-end;
  }

  .add {
    min-width: min(16rem, 100%);
  }

  .spacer {
    flex: 1;
  }
</style>

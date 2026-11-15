import type { Component } from 'svelte';
import type { CardSize, DashboardBody, DashboardCard } from '#lib/api/dashboard.js';

/** A card in the editable list; the id is only a key for the page, because the server numbers its own rows. */
export interface Card {
  id: string;
  card_type: string;
  parameter_id: number | null;
  size: CardSize;
  options: Record<string, unknown>;
}

/** The property the drag library sets on the placeholder copy it inserts while a card is in flight. */
export const shadowMarker = 'isDndShadowItem';

/** A card that can be added: a card type, with the parameter it shows when the type needs one. */
export interface CardChoice {
  key: string;
  card_type: string;
  parameter_id: number | null;
}

/** What a card file exports: its component and what the picker and the editor need to know about the card. */
export interface CardKind {
  default: Component<{ card: Card }>;
  cardType: string;
  /** True when the card shows one parameter and so needs a parameter id. */
  needsParameter: boolean;
  /** The card's name for pickers and screen readers; the parameter name is null for cards without one. */
  cardLabel: (parameterName: string | null) => string;
}

export function isShadow(card: Card): boolean {
  return Boolean((card as unknown as Record<string, unknown>)[shadowMarker]);
}

/** The list without the drag library's placeholder, which must never reach a save. */
export function settledCards(cards: readonly Card[]): Card[] {
  return cards.filter((c) => !isShadow(c));
}

/** The cards in saved order, keyed for the page. */
export function fromLayout(cards: readonly DashboardCard[]): Card[] {
  return [...cards]
    .sort((a, b) => a.position - b.position)
    .map((c) => ({
      id: `s${c.id}`,
      card_type: c.card_type,
      parameter_id: c.parameter_id,
      size: c.size,
      options: c.options ?? {}
    }));
}

/** Moves the card by delta places, stopping at the ends of the list. */
export function moveCard(cards: Card[], id: string, delta: number): Card[] {
  const from = cards.findIndex((c) => c.id === id);
  if (from < 0) return cards;
  const to = Math.min(Math.max(from + delta, 0), cards.length - 1);
  if (to === from) return cards;
  const next = [...cards];
  const [card] = next.splice(from, 1);
  next.splice(to, 0, card);
  return next;
}

export function toggleSize(cards: Card[], id: string): Card[] {
  return cards.map((c) =>
    c.id === id ? { ...c, size: c.size === 'wide' ? 'normal' : 'wide' } : c
  );
}

/** Removes the card, but never the last one, because the server rejects an empty dashboard. */
export function removeCard(cards: Card[], id: string): Card[] {
  if (cards.length <= 1) return cards;
  return cards.filter((c) => c.id !== id);
}

export function addCard(cards: Card[], card: Card): Card[] {
  return [...cards, card];
}

/** The cards the user can still add: every card type, once per parameter for types that show one, minus those already on the list. */
export function availableCards(
  cards: readonly Card[],
  kinds: readonly Pick<CardKind, 'cardType' | 'needsParameter'>[],
  parameterIds: readonly number[]
): CardChoice[] {
  const have = new Set(cards.map((c) => choiceKey(c.card_type, c.parameter_id)));
  const out: CardChoice[] = [];
  for (const kind of kinds) {
    const ids = kind.needsParameter ? parameterIds : [null];
    for (const parameterId of ids) {
      const key = choiceKey(kind.cardType, parameterId);
      if (!have.has(key)) out.push({ key, card_type: kind.cardType, parameter_id: parameterId });
    }
  }
  return out;
}

function choiceKey(cardType: string, parameterId: number | null): string {
  return `${cardType}:${parameterId ?? ''}`;
}

export function newCard(choice: CardChoice, id: string): Card {
  return {
    id,
    card_type: choice.card_type,
    parameter_id: choice.parameter_id,
    size: 'normal',
    options: {}
  };
}

/** The body of the save request for the list in its current order. */
export function toBody(cards: readonly Card[]): DashboardBody {
  return {
    cards: cards.map((c, position) => ({
      card_type: c.card_type,
      parameter_id: c.parameter_id,
      position,
      size: c.size,
      options: c.options
    }))
  };
}

/** The positions of the cards the server flagged, read from field names such as cards.2.parameter_id. */
export function rejectedIndexes(fields: Record<string, string>): number[] {
  const out = new Set<number>();
  for (const key of Object.keys(fields)) {
    const match = /^cards\.(\d+)(\.|$)/.exec(key);
    if (match) out.add(Number(match[1]));
  }
  return [...out].sort((a, b) => a - b);
}

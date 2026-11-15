import type { Component } from 'svelte';
import type { Card } from './cards.js';

/** What a card file exports besides its component. */
export interface CardModule {
  default: Component<{ card: Card }>;
  cardType: string;
  /** True when the card shows one parameter and so needs a parameter id. */
  needsParameter: boolean;
  /** The card's name for pickers and screen readers; the parameter name is null for cards without one. */
  cardLabel: (parameterName: string | null) => string;
}

export interface CardKind {
  type: string;
  needsParameter: boolean;
  label: (parameterName: string | null) => string;
  component: Component<{ card: Card }>;
}

/** Keys the card files by the type each one exports; a file without a type or a repeated type is a mistake, so it throws. */
export function buildRegistry(modules: Record<string, CardModule>): Record<string, CardKind> {
  const out: Record<string, CardKind> = {};
  for (const [path, mod] of Object.entries(modules)) {
    if (typeof mod.cardType !== 'string' || mod.cardType === '') {
      throw new Error(`${path} does not export a cardType`);
    }
    if (mod.cardType in out) throw new Error(`${path} repeats the card type ${mod.cardType}`);
    out[mod.cardType] = {
      type: mod.cardType,
      needsParameter: mod.needsParameter,
      label: mod.cardLabel,
      component: mod.default
    };
  }
  return out;
}

/** Every card file in ./cards, so adding a card type needs no list to update. */
export const registry = buildRegistry(
  import.meta.glob<CardModule>('./cards/*.svelte', { eager: true })
);

export const cardKinds: CardKind[] = Object.values(registry);

/** The display name of a card; a type this build does not know falls back to the raw type. */
export function labelFor(card: Pick<Card, 'card_type'>, parameterName: string | null): string {
  return registry[card.card_type]?.label(parameterName) ?? card.card_type;
}

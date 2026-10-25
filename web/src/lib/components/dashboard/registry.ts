import type { CardKind, Card } from './cards.js';

/** Keys the card files by the type each one exports; a file without a type or a repeated type is a mistake, so it throws. */
export function buildRegistry(modules: Record<string, CardKind>): Record<string, CardKind> {
  const out: Record<string, CardKind> = {};
  for (const [path, mod] of Object.entries(modules)) {
    if (typeof mod.cardType !== 'string' || mod.cardType === '') {
      throw new Error(`${path} does not export a cardType`);
    }
    if (mod.cardType in out) throw new Error(`${path} repeats the card type ${mod.cardType}`);
    out[mod.cardType] = mod;
  }
  return out;
}

/** Every card file in ./cards, so adding a card type needs no list to update. */
export const registry = buildRegistry(
  import.meta.glob<CardKind>('./cards/*.svelte', { eager: true })
);

export const cardKinds: CardKind[] = Object.values(registry);

/** The display name of a card; a type this build does not know falls back to the raw type. */
export function labelFor(card: Pick<Card, 'card_type'>, parameterName: string | null): string {
  return registry[card.card_type]?.cardLabel(parameterName) ?? card.card_type;
}

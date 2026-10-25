import { describe, expect, it } from 'vitest';
import * as library from 'svelte-dnd-action';
import type { DashboardCard } from '#lib/api/dashboard.js';
import {
  addCard,
  availableCards,
  fromLayout,
  moveCard,
  newCard,
  rejectedIndexes,
  removeCard,
  settledCards,
  shadowMarker,
  toBody,
  toggleSize,
  type Card
} from './cards.js';

const card = (id: string, extra: Partial<Card> = {}): Card => ({
  id,
  card_type: 'parameter',
  parameter_id: 1,
  size: 'normal',
  options: {},
  ...extra
});

const ids = (cards: Card[]) => cards.map((c) => c.id);

describe('shadow placeholder', () => {
  it('uses the property name the drag library sets', () => {
    expect(shadowMarker).toBe(library.SHADOW_ITEM_MARKER_PROPERTY_NAME);
  });

  it('drops the placeholder and keeps every real card in order', () => {
    const shadow = { ...card('id:dnd-shadow-placeholder-0000'), [shadowMarker]: true };
    expect(ids(settledCards([card('a'), shadow, card('b')]))).toEqual(['a', 'b']);
  });
});

describe('moveCard', () => {
  const list = [card('a'), card('b'), card('c')];

  it('moves a card down and up by one place', () => {
    expect(ids(moveCard(list, 'a', 1))).toEqual(['b', 'a', 'c']);
    expect(ids(moveCard(list, 'c', -1))).toEqual(['a', 'c', 'b']);
  });

  it('stops at the ends and returns the same list when nothing moves', () => {
    expect(moveCard(list, 'a', -1)).toBe(list);
    expect(moveCard(list, 'c', 1)).toBe(list);
    expect(ids(moveCard(list, 'a', 10))).toEqual(['b', 'c', 'a']);
  });

  it('leaves the list alone for an unknown id and never changes the input', () => {
    expect(moveCard(list, 'zzz', 1)).toBe(list);
    moveCard(list, 'a', 1);
    expect(ids(list)).toEqual(['a', 'b', 'c']);
  });
});

describe('toggleSize', () => {
  it('switches only the named card between normal and wide', () => {
    const once = toggleSize([card('a'), card('b')], 'a');
    expect(once.map((c) => c.size)).toEqual(['wide', 'normal']);
    expect(toggleSize(once, 'a').map((c) => c.size)).toEqual(['normal', 'normal']);
  });
});

describe('removeCard', () => {
  it('removes the named card', () => {
    expect(ids(removeCard([card('a'), card('b'), card('c')], 'b'))).toEqual(['a', 'c']);
  });

  it('keeps the last card, because the server rejects an empty dashboard', () => {
    const only = [card('a')];
    expect(removeCard(only, 'a')).toBe(only);
  });
});

describe('addCard and availableCards', () => {
  const kinds = [
    { type: 'parameter', needsParameter: true },
    { type: 'parameter_chart', needsParameter: true },
    { type: 'tests_due', needsParameter: false }
  ];

  it('offers every type once, and per parameter for the types that show one', () => {
    expect(availableCards([], kinds, [1, 2]).map((c) => c.key)).toEqual([
      'parameter:1',
      'parameter:2',
      'parameter_chart:1',
      'parameter_chart:2',
      'tests_due:'
    ]);
  });

  it('leaves out cards already on the dashboard', () => {
    const have = [
      card('a'),
      card('b', { card_type: 'tests_due', parameter_id: null }),
      card('c', { card_type: 'parameter_chart', parameter_id: 2 })
    ];
    expect(availableCards(have, kinds, [1, 2]).map((c) => c.key)).toEqual([
      'parameter:2',
      'parameter_chart:1'
    ]);
  });

  it('offers nothing for parameter cards when no parameter is enabled', () => {
    expect(availableCards([], kinds, []).map((c) => c.key)).toEqual(['tests_due:']);
  });

  it('appends a normal-sized card for a choice', () => {
    const [choice] = availableCards([], kinds, [7]);
    const added = addCard([card('a')], newCard(choice, 'n0'));
    expect(added[1]).toEqual({
      id: 'n0',
      card_type: 'parameter',
      parameter_id: 7,
      size: 'normal',
      options: {}
    });
  });
});

describe('fromLayout', () => {
  it('orders by position and keys each card by its server id', () => {
    const server: DashboardCard[] = [
      { id: 9, card_type: 'tests_due', parameter_id: null, position: 1, size: 'wide', options: {} },
      {
        id: 4,
        card_type: 'parameter',
        parameter_id: 3,
        position: 0,
        size: 'normal',
        options: { a: 1 }
      }
    ];
    const cards = fromLayout(server);
    expect(cards.map((c) => c.id)).toEqual(['s4', 's9']);
    expect(cards[0]).toMatchObject({ parameter_id: 3, size: 'normal', options: { a: 1 } });
  });
});

describe('toBody', () => {
  it('numbers positions from zero in list order and sends every field the server stores', () => {
    const body = toBody([
      card('x', { parameter_id: 5, size: 'wide', options: { range: '30d' } }),
      card('y', { card_type: 'tests_due', parameter_id: null })
    ]);
    expect(body).toEqual({
      cards: [
        {
          card_type: 'parameter',
          parameter_id: 5,
          position: 0,
          size: 'wide',
          options: { range: '30d' }
        },
        { card_type: 'tests_due', parameter_id: null, position: 1, size: 'normal', options: {} }
      ]
    });
  });

  it('does not send the client key', () => {
    expect(JSON.stringify(toBody([card('secret')]))).not.toContain('secret');
  });
});

describe('rejectedIndexes', () => {
  it('reads the card positions out of dotted field names', () => {
    expect(
      rejectedIndexes({
        'cards.2.parameter_id': 'invalid',
        'cards.0.card_type': 'invalid',
        'cards.2.size': 'invalid'
      })
    ).toEqual([0, 2]);
  });

  it('ignores list-wide and unrelated fields', () => {
    expect(rejectedIndexes({ cards: 'must have at least one card', other: 'x' })).toEqual([]);
  });
});

import { describe, expect, it } from 'vitest';
import { buildRegistry, cardKinds, labelFor, registry, type CardModule } from './registry.js';

const fake = (cardType: string): CardModule => ({
  default: (() => {}) as unknown as CardModule['default'],
  cardType,
  needsParameter: false,
  cardLabel: () => cardType
});

describe('registry', () => {
  it('finds the card files by glob and keys them by their exported type', () => {
    expect(Object.keys(registry).sort()).toEqual(['parameter', 'parameter_chart', 'tests_due']);
    expect(registry.parameter.needsParameter).toBe(true);
    expect(registry.parameter_chart.needsParameter).toBe(true);
    expect(registry.tests_due.needsParameter).toBe(false);
    for (const kind of cardKinds) expect(kind.component).toBeTypeOf('function');
  });

  it('labels cards by kind and falls back to the raw type for an unknown one', () => {
    expect(labelFor({ card_type: 'parameter' }, 'pH')).toBe('pH');
    expect(labelFor({ card_type: 'parameter_chart' }, 'pH')).toContain('pH');
    expect(labelFor({ card_type: 'tests_due' }, null)).not.toBe('tests_due');
    expect(labelFor({ card_type: 'parameter' }, null)).not.toBe('parameter');
    expect(labelFor({ card_type: 'parameter_chart' }, null)).not.toContain('parameter_chart');
    expect(labelFor({ card_type: 'from_the_future' }, null)).toBe('from_the_future');
  });

  it('refuses a card file without a type', () => {
    expect(() => buildRegistry({ './cards/A.svelte': fake('') })).toThrow(/A\.svelte.*cardType/);
  });

  it('refuses two files that claim the same type', () => {
    expect(() =>
      buildRegistry({ './cards/A.svelte': fake('x'), './cards/B.svelte': fake('x') })
    ).toThrow(/B\.svelte.*repeats/);
  });
});

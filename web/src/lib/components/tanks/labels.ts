import type { TankType } from '#lib/api/tanks.js';
import { m } from '#lib/paraglide/messages.js';

export function tankTypeLabel(type: TankType): string {
  switch (type) {
    case 'reef':
      return m.tanks_type_reef();
    case 'fowlr':
      return m.tanks_type_fowlr();
    case 'frag':
      return m.tanks_type_frag();
    case 'quarantine':
      return m.tanks_type_quarantine();
    default:
      return m.tanks_type_other();
  }
}

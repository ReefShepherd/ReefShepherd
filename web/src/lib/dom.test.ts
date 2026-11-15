import { describe, expect, it, vi } from 'vitest';
import { refocusMoveButton } from './dom.js';

function fakeRoot(buttons: Partial<Record<'up' | 'down', { disabled: boolean }>>) {
  const focus = { up: vi.fn(), down: vi.fn() };
  const root = {
    querySelector: (selector: string) => {
      const name = /data-action="(up|down)"/.exec(selector)?.[1] as 'up' | 'down';
      const found = buttons[name];
      return found ? { ...found, focus: focus[name] } : null;
    }
  } as unknown as ParentNode;
  return { root, focus };
}

describe('refocusMoveButton', () => {
  it('focuses the button that was used', () => {
    const { root, focus } = fakeRoot({ up: { disabled: false }, down: { disabled: false } });
    refocusMoveButton(root, 'down');
    expect(focus.down).toHaveBeenCalledTimes(1);
    expect(focus.up).not.toHaveBeenCalled();
  });

  it('focuses the other button when the used one is disabled at the end of the list', () => {
    const { root, focus } = fakeRoot({ up: { disabled: false }, down: { disabled: true } });
    refocusMoveButton(root, 'down');
    expect(focus.up).toHaveBeenCalledTimes(1);
    expect(focus.down).not.toHaveBeenCalled();
  });

  it('does nothing when the row is gone', () => {
    expect(() => refocusMoveButton(null, 'up')).not.toThrow();
    const { root } = fakeRoot({});
    expect(() => refocusMoveButton(root, 'up')).not.toThrow();
  });
});

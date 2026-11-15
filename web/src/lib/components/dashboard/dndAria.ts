import { m } from '#lib/paraglide/messages.js';
import { setAriaStrings } from 'svelte-dnd-action';

/** Routes the drag library's screen reader announcements through the message catalogue. */
export function wireDndAria(): void {
  setAriaStrings({
    dragStarted: ({ itemLabel }) => m.dashboard_aria_picked({ name: itemLabel }),
    movedToPosition: ({ itemLabel, position, count }) =>
      m.dashboard_aria_moved({ name: itemLabel, position, count }),
    movedToZoneStart: ({ itemLabel, count }) =>
      m.dashboard_aria_moved({ name: itemLabel, position: 1, count }),
    movedToZoneEnd: ({ itemLabel, count }) =>
      m.dashboard_aria_moved({ name: itemLabel, position: count, count }),
    dropped: ({ itemLabel }) => m.dashboard_aria_dropped({ name: itemLabel }),
    zoneActiveInstruction: m.dashboard_aria_instruction()
  });
}

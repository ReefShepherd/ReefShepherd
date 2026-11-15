import { parseId } from '#lib/api/tank-id.js';

export const currentTankKey = 'current_tank';

function read(): number | null {
  try {
    return parseId(localStorage.getItem(currentTankKey));
  } catch {
    return null;
  }
}

let current = $state<number | null>(typeof localStorage === 'undefined' ? null : read());

export const currentTank = {
  get id(): number | null {
    return current;
  },
  set(id: number | null): void {
    current = id;
    try {
      if (id === null) localStorage.removeItem(currentTankKey);
      else localStorage.setItem(currentTankKey, String(id));
    } catch {
      /* storage unavailable */
    }
  }
};

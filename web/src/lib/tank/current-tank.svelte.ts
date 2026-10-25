export const currentTankKey = 'current_tank';

// The id ends up in link paths, so only plain id characters are accepted from storage.
export function isTankId(value: unknown): value is string {
  return typeof value === 'string' && /^[A-Za-z0-9_-]+$/.test(value);
}

function read(): string | null {
  try {
    const stored = localStorage.getItem(currentTankKey);
    return isTankId(stored) ? stored : null;
  } catch {
    return null;
  }
}

let current = $state<string | null>(typeof localStorage === 'undefined' ? null : read());

export const currentTank = {
  get id(): string | null {
    return current;
  },
  set(id: string | null): void {
    current = id;
    try {
      if (id === null) localStorage.removeItem(currentTankKey);
      else localStorage.setItem(currentTankKey, id);
    } catch {
      /* storage unavailable */
    }
  }
};

import { tick } from 'svelte';

/** Moves focus to the first field in the form that the last submit flagged, so keyboard and screen reader users land on the problem. */
export async function focusFirstInvalid(form: HTMLFormElement | null | undefined): Promise<void> {
  await tick();
  form?.querySelector<HTMLElement>('[aria-invalid="true"]')?.focus();
}

/** Puts focus back on the up or down button that was just used, or on its twin when that one is disabled at an end of the list. */
export function refocusMoveButton(
  root: Pick<ParentNode, 'querySelector'> | null | undefined,
  action: 'up' | 'down'
): void {
  const button = (name: 'up' | 'down') =>
    root?.querySelector<HTMLButtonElement>(`[data-action="${name}"]`);
  const same = button(action);
  (same && !same.disabled ? same : button(action === 'up' ? 'down' : 'up'))?.focus();
}

/** Offers text as a file download without a round trip to the server. */
export function downloadTextFile(filename: string, text: string): void {
  const url = URL.createObjectURL(new Blob([text], { type: 'text/plain;charset=utf-8' }));
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  document.body.append(link);
  link.click();
  link.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

/** Svelte action: focuses the first form control inside the node when it appears, such as the next step of a wizard. */
export function focusFirstField(node: HTMLElement): void {
  node.querySelector<HTMLElement>('input:not([type="hidden"]), select, textarea')?.focus();
}

import { describe, expect, it, vi } from 'vitest';
import { cachedResource } from './cached-resource.svelte.js';

// A read the test finishes by hand, so a late answer can arrive after the read was cancelled.
function manualReads() {
  const reads: {
    signal: AbortSignal;
    resolve: (v: string) => void;
    reject: (e: unknown) => void;
  }[] = [];
  const read = vi.fn(
    (signal: AbortSignal) =>
      new Promise<string>((resolve, reject) => {
        reads.push({ signal, resolve, reject });
      })
  );
  return { read, reads };
}

const isAbort = (e: unknown) => e instanceof DOMException && e.name === 'AbortError';

describe('cachedResource', () => {
  it('shares one read between callers made while it is under way', async () => {
    const { read, reads } = manualReads();
    const resource = cachedResource(read);
    const a = resource.load();
    const b = resource.load();
    expect(a).toBe(b);
    reads[0].resolve('x');
    expect(await a).toBe('x');
    expect(read).toHaveBeenCalledTimes(1);
    expect(resource.value).toBe('x');
  });

  it('reads again on the next load once a read has finished, even after a failure', async () => {
    const { read, reads } = manualReads();
    const resource = cachedResource(read);
    const first = resource.load();
    reads[0].reject(new Error('offline'));
    await expect(first).rejects.toThrow('offline');
    const second = resource.load();
    reads[1].resolve('y');
    expect(await second).toBe('y');
    expect(read).toHaveBeenCalledTimes(2);
  });

  it('cancels the read under way when a refresh starts, discards its late result and answers its callers with the new one', async () => {
    const { read, reads } = manualReads();
    const resource = cachedResource(read);
    const old = resource.load();
    const fresh = resource.refresh();
    expect(reads[0].signal.aborted).toBe(true);
    expect(reads[1].signal.aborted).toBe(false);
    reads[0].resolve('stale');
    await Promise.resolve();
    expect(resource.value).toBeNull();
    reads[1].resolve('fresh');
    expect(await fresh).toBe('fresh');
    expect(await old).toBe('fresh');
    expect(resource.value).toBe('fresh');
  });

  it('answers callers of a read cancelled by a refresh with the new read even when the old one fails with an abort', async () => {
    const { read, reads } = manualReads();
    const resource = cachedResource(read);
    const old = resource.load();
    const fresh = resource.refresh();
    reads[0].reject(new DOMException('aborted', 'AbortError'));
    reads[1].resolve('fresh');
    expect(await old).toBe('fresh');
    expect(await fresh).toBe('fresh');
  });

  it('cancels the read under way on invalidate, which also forgets the value', async () => {
    const { read, reads } = manualReads();
    const resource = cachedResource(read);
    const first = resource.refresh();
    reads[0].resolve('one');
    await first;
    const second = resource.refresh();
    resource.invalidate();
    expect(reads[1].signal.aborted).toBe(true);
    expect(resource.value).toBeNull();
    reads[1].resolve('late');
    await expect(second).rejects.toSatisfy(isAbort);
    expect(resource.value).toBeNull();
  });

  it('cancels the read under way when a newer copy is set and answers its callers with that copy', async () => {
    const { read, reads } = manualReads();
    const resource = cachedResource(read);
    const pending = resource.load();
    resource.set('given');
    expect(reads[0].signal.aborted).toBe(true);
    reads[0].resolve('stale');
    expect(await pending).toBe('given');
    expect(resource.value).toBe('given');
  });

  it('keeps the last value when a refresh fails', async () => {
    const { read, reads } = manualReads();
    const resource = cachedResource(read);
    const first = resource.refresh();
    reads[0].resolve('kept');
    await first;
    const failing = resource.refresh();
    reads[1].reject(new Error('offline'));
    await expect(failing).rejects.toThrow('offline');
    expect(resource.value).toBe('kept');
  });
});

// Generates fields with gen.wasm in a Web Worker, one request at a time
// per worker. Where workers are missing, as in tests, it runs the module
// on the page.

import type { FieldGenerator, FieldInfo, Generated } from './fields';
import type { MapRequest } from './generator';

/** A request to the worker. */
export interface FieldRequest extends MapRequest {
  id: number;
}

/** The worker's answer: a field, or an error. */
export type FieldResponse =
  | {
      id: number;
      checksum: string;
      wmap: Uint8Array<ArrayBuffer>;
      attempt: number;
      ms: number;
      /** The module's memory after the field, in bytes. */
      memory: number;
    }
  | { id: number; error: string };

/**
 * The module's memory above which the worker stops after its field.
 * TinyGo's heap never shrinks, and large fields leave it fragmented, so
 * a new worker with a new module takes the next field.
 */
const MAX_WORKER_MEMORY = 256 << 20;

/** The worker's request for a map's info. A home map has none. */
export function requestOf(id: number, info: FieldInfo): FieldRequest {
  if (info.kind === 'home' || info.key === undefined) {
    throw new Error(`${info.id} is not a generated map`);
  }
  return {
    id,
    version: info.version,
    rng: info.rng,
    key: info.key,
    width: info.width,
    height: info.height,
    biome: info.biome,
    kind: info.kind ?? 'field',
    params: info.params ?? '',
  };
}

let worker: Worker | undefined;
let next = 0;
const waiting = new Map<number, (r: FieldResponse) => void>();

function startWorker(): Worker {
  const w = new Worker(new URL('./fieldWorker.ts', import.meta.url), { type: 'module' });
  w.addEventListener('message', (e: MessageEvent<FieldResponse>) => {
    const done = waiting.get(e.data.id);
    waiting.delete(e.data.id);
    done?.(e.data);
  });
  w.addEventListener('error', (e) => {
    // A worker that fails to start fails every waiting request, and the
    // next request starts a new one.
    for (const [id, done] of waiting) {
      done({ id, error: `the field worker failed: ${e.message}` });
    }
    waiting.clear();
    worker = undefined;
  });
  return w;
}

/** Generates a field in the worker. */
export const generateInWorker: FieldGenerator = async (info: FieldInfo): Promise<Generated> => {
  if (typeof Worker === 'undefined') {
    const { Generator, runRequest } = await import('./generator');
    const g = await Generator.load();
    if (!g.supports(info.version, info.rng)) {
      throw new Error(`gen.wasm lacks generator ${info.version} with ${info.rng}`);
    }
    return runRequest(g, requestOf(0, info));
  }
  const id = ++next;
  const request = requestOf(id, info);
  worker ??= startWorker();
  const r = await new Promise<FieldResponse>((resolve) => {
    waiting.set(id, resolve);
    worker!.postMessage(request);
  });
  if ('error' in r) {
    throw new Error(r.error);
  }
  if (r.memory > MAX_WORKER_MEMORY && waiting.size === 0) {
    worker.terminate();
    worker = undefined;
  }
  return { checksum: r.checksum, wmap: r.wmap, attempt: r.attempt };
};

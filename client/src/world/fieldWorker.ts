// The Web Worker that generates fields with gen.wasm, so a field of up
// to 1024 × 1024 cells never blocks the page (strategy section 8.5,
// "Performance"). fieldGenerator.ts starts it.

import { Generator, keyFromHex } from './generator';
import type { FieldRequest, FieldResponse } from './fieldGenerator';

interface WorkerScope {
  addEventListener: (type: 'message', listener: (e: MessageEvent<FieldRequest>) => void) => void;
  postMessage: (message: FieldResponse, transfer: Transferable[]) => void;
}

// oxlint-disable-next-line typescript/no-unsafe-type-assertion
const scope = globalThis as unknown as WorkerScope;
let generator: Promise<Generator> | undefined;

scope.addEventListener('message', (e) => {
  const r = e.data;
  generator ??= Generator.load();
  generator
    .then((g) => {
      if (!g.supports(r.version, r.rng)) {
        throw new Error(`gen.wasm lacks generator ${r.version} with ${r.rng}`);
      }
      const began = performance.now();
      const out = g.generate(r.version, keyFromHex(r.key), r.width, r.height, r.biome);
      scope.postMessage(
        {
          id: r.id,
          checksum: out.checksum,
          wmap: out.wmap,
          attempt: out.attempt,
          ms: performance.now() - began,
          memory: g.memoryBytes,
        },
        [out.wmap.buffer],
      );
    })
    .catch((error: unknown) => {
      scope.postMessage({ id: r.id, error: String(error) }, []);
    });
});

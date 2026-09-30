// Reads WPAL palettes, as defined in docs/specs/assets-v1.md, "Palette
// (WPAL)".

import { FormatError, Reader } from './reader';

/** Returns the 256 RGBA colors of a palette. Index 0 is transparent. */
export default function decodeWPAL(data: Uint8Array): Uint8Array {
  const r = new Reader(data);
  if (r.tag() !== 'WPAL' || r.u16() !== 1 || r.remaining() !== 1024) {
    throw new FormatError('wpal: invalid palette');
  }
  return r.take(1024);
}

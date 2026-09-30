import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { FormatError } from './reader';
import decodeWEFF from './weff';

// sample.weff and sample-weff.json are written by the Go encoder's tests
// (internal/assets/effect), so the two stay in step.
const bytes = new Uint8Array(readFileSync(new URL('testdata/sample.weff', import.meta.url)));

interface GoFrame {
  Visible: boolean;
  Texture: number;
  SrcBlend: number;
  DstBlend: number;
  Color: number[];
  Offset: number[];
  Angle: number;
  Corners: number[][];
}

const want = JSON.parse(
  readFileSync(new URL('testdata/sample-weff.json', import.meta.url), 'utf8'),
) as { FPS: number; Frames: number; Textures: string[]; Layers: GoFrame[][] };

describe('decodeWEFF', () => {
  it('reads what the Go encoder writes', () => {
    const e = decodeWEFF(bytes);
    expect([e.fps, e.frames, e.textures, e.layers]).toEqual([
      want.FPS,
      want.Frames,
      want.Textures,
      want.Layers.length,
    ]);
    const frames = want.Layers.flat();
    expect([...e.visible]).toEqual(frames.map((f) => (f.Visible ? 1 : 0)));
    expect([...e.texture]).toEqual(frames.map((f) => f.Texture));
    expect([...e.srcBlend]).toEqual(frames.map((f) => f.SrcBlend));
    expect([...e.dstBlend]).toEqual(frames.map((f) => f.DstBlend));
    expect([...e.color]).toEqual(frames.flatMap((f) => f.Color));
    expect([...e.offset]).toEqual(frames.flatMap((f) => f.Offset.map(Math.fround)));
    expect([...e.angle]).toEqual(frames.map((f) => Math.fround(f.Angle)));
    expect([...e.corners]).toEqual(frames.flatMap((f) => f.Corners.flat().map(Math.fround)));
  });

  it('throws only FormatError on damaged input', () => {
    let seed = 3;
    const next = (): number => {
      seed = (seed * 1103515245 + 12345) % 2 ** 31;
      return seed;
    };
    for (let i = 0; i < 2000; i++) {
      const b = bytes.slice(0, 6 + (next() % (bytes.length - 6)) + 1);
      for (let k = next() % 4; k >= 0; k--) {
        b[next() % b.length] = next() % 256;
      }
      try {
        decodeWEFF(b);
      } catch (error) {
        expect(error).toBeInstanceOf(FormatError);
      }
    }
  });
});

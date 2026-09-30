import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { FormatError } from './reader';
import { decodeWSPR } from './wspr';

// sample.wspr and sample-wspr.json are written by the Go encoder's tests
// (internal/assets/wspr), so the two stay in step.
const bytes = new Uint8Array(readFileSync(new URL('testdata/sample.wspr', import.meta.url)));

interface GoSprite {
  Pages: { Kind: number; Width: number; Height: number; Pix: string }[];
  Palette: number[][] | null;
  Images: { Page: number; X: number; Y: number; Width: number; Height: number }[];
  Actions: {
    IntervalMs: number;
    Frames: {
      Event: number;
      Anchors: { X: number; Y: number }[] | null;
      Layers: {
        Image: number;
        X: number;
        Y: number;
        Mirror: boolean;
        Color: number[];
        ScaleX: number;
        ScaleY: number;
        Rotation: number;
      }[];
    }[];
  }[];
  Events: string[];
}

const want = JSON.parse(
  readFileSync(new URL('testdata/sample-wspr.json', import.meta.url), 'utf8'),
) as GoSprite;

describe('decodeWSPR', () => {
  it('reads what the Go encoder writes', () => {
    const s = decodeWSPR(bytes);
    expect(
      s.pages.map((p) => ({ ...p, pixels: Buffer.from(p.pixels).toString('base64') })),
    ).toEqual(
      want.Pages.map((p) => ({ kind: p.Kind, width: p.Width, height: p.Height, pixels: p.Pix })),
    );
    expect([...(s.palette ?? [])]).toEqual(want.Palette?.flat() ?? []);
    expect(s.images).toEqual(
      want.Images.map((i) => ({ page: i.Page, x: i.X, y: i.Y, width: i.Width, height: i.Height })),
    );
    expect(s.actions).toEqual(
      want.Actions.map((a) => ({
        intervalMs: a.IntervalMs,
        frames: a.Frames.map((f) => ({
          event: f.Event,
          anchors: (f.Anchors ?? []).map((p) => [p.X, p.Y]),
          layers: f.Layers.map((l) => ({
            image: l.Image,
            x: l.X,
            y: l.Y,
            mirror: l.Mirror,
            color: l.Color,
            scaleX: Math.fround(l.ScaleX),
            scaleY: Math.fround(l.ScaleY),
            rotation: l.Rotation,
          })),
        })),
      })),
    );
    expect(s.events).toEqual(want.Events);
  });

  it('throws only FormatError on damaged input', () => {
    let seed = 7;
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
        decodeWSPR(b);
      } catch (error) {
        expect(error).toBeInstanceOf(FormatError);
      }
    }
  });
});

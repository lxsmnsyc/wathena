import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { FormatError } from './reader';
import { decodeWMap } from './wmap';

// sample.wmap and sample.json are written by the Go encoder's tests
// (internal/assets/wmap), so the two stay in step.
const bytes = new Uint8Array(readFileSync(new URL('testdata/sample.wmap', import.meta.url)));

interface GoZone {
  Level: number;
  Type: number;
  WaveHeight: number;
  WaveSpeed: number;
  WavePitch: number;
  AnimSpeed: number;
}

interface GoMap {
  Width: number;
  Height: number;
  Cells: { Flags: number; Heights: number[] }[];
  Ground: {
    Width: number;
    Height: number;
    Textures: string[];
    Tiles: { U: number[]; V: number[]; Texture: number; Lightmap: number; Color: number[] }[];
    Surfaces: { Heights: number[]; Top: number; North: number; East: number }[];
    Lightmap: string;
    LightmapCount: number;
    LightmapColumns: number;
  };
  Light: {
    Longitude: number;
    Latitude: number;
    Diffuse: number[];
    Ambient: number[];
    Opacity: number;
    Direction: number[];
  };
  Water: GoZone & {
    SplitWidth: number;
    SplitHeight: number;
    Zones: GoZone[];
    Textures: { Type: number; Frames: string[] }[];
  };
  Objects: {
    Model: string;
    Matrix: number[];
    AnimType: number;
    AnimSpeed: number;
    BlockType: number;
  }[];
  Sounds: {
    Name: string;
    Pos: number[];
    Volume: number;
    Width: number;
    Height: number;
    Range: number;
    Cycle: number;
  }[];
  Effects: { ID: number; Pos: number[]; Delay: number; Param: number[] }[];
  Emitters: {
    Pos: number[];
    Texture: string;
    SrcBlend: number;
    DstBlend: number;
    Color: number[];
    Dir1: number[];
    Dir2: number[];
    Gravity: number[];
    Radius: number[];
    Life: number[];
    Rate: number[];
    Size: number[];
    Speed: number;
    MaxCount: number;
    Flags: number;
    Scale: number[];
    Rotate: number[];
  }[];
  Music: string;
  Deps: string[];
}

const want = JSON.parse(
  readFileSync(new URL('testdata/sample.json', import.meta.url), 'utf8'),
) as GoMap;

// Go writes float32 values in their shortest form, so compare them as
// float32.
const f32 = (v: number[]): number[] => v.map((x) => Math.fround(x));
const zone = (z: GoZone) => ({
  level: Math.fround(z.Level),
  type: z.Type,
  waveHeight: Math.fround(z.WaveHeight),
  waveSpeed: Math.fround(z.WaveSpeed),
  wavePitch: Math.fround(z.WavePitch),
  animSpeed: z.AnimSpeed,
});

describe('decodeWMap', () => {
  it('reads what the Go encoder writes', () => {
    const m = decodeWMap(bytes);
    expect([m.width, m.height]).toEqual([want.Width, want.Height]);
    expect([...m.cellFlags]).toEqual(want.Cells.map((c) => c.Flags));
    expect([...m.cellHeights]).toEqual(f32(want.Cells.flatMap((c) => c.Heights)));

    const g = m.ground;
    const wg = want.Ground;
    expect([g.width, g.height, g.textures]).toEqual([wg.Width, wg.Height, wg.Textures]);
    expect([...g.tiles.u]).toEqual(f32(wg.Tiles.flatMap((t) => t.U)));
    expect([...g.tiles.v]).toEqual(f32(wg.Tiles.flatMap((t) => t.V)));
    expect([...g.tiles.texture]).toEqual(wg.Tiles.map((t) => t.Texture));
    expect([...g.tiles.lightmap]).toEqual(wg.Tiles.map((t) => t.Lightmap));
    expect([...g.tiles.color]).toEqual(wg.Tiles.flatMap((t) => t.Color));
    expect([...g.surfaceHeights]).toEqual(f32(wg.Surfaces.flatMap((s) => s.Heights)));
    expect([...g.top]).toEqual(wg.Surfaces.map((s) => s.Top));
    expect([...g.north]).toEqual(wg.Surfaces.map((s) => s.North));
    expect([...g.east]).toEqual(wg.Surfaces.map((s) => s.East));
    expect([g.lightmap, g.lightmapCount, g.lightmapColumns]).toEqual([
      wg.Lightmap,
      wg.LightmapCount,
      wg.LightmapColumns,
    ]);

    const l = want.Light;
    expect(m.light).toEqual({
      longitude: l.Longitude,
      latitude: l.Latitude,
      diffuse: f32(l.Diffuse),
      ambient: f32(l.Ambient),
      opacity: Math.fround(l.Opacity),
      direction: f32(l.Direction),
    });

    const w = want.Water;
    expect(m.water).toEqual({
      ...zone(w),
      splitWidth: w.SplitWidth,
      splitHeight: w.SplitHeight,
      zones: w.Zones.map(zone),
      textures: new Map(w.Textures.map((t) => [t.Type, t.Frames])),
    });

    expect(m.objects).toEqual(
      want.Objects.map((o) => ({
        model: o.Model,
        matrix: new Float32Array(o.Matrix),
        animType: o.AnimType,
        animSpeed: Math.fround(o.AnimSpeed),
        blockType: o.BlockType,
      })),
    );
    expect(m.sounds).toEqual(
      want.Sounds.map((s) => ({
        name: s.Name,
        position: f32(s.Pos),
        volume: Math.fround(s.Volume),
        width: Math.fround(s.Width),
        height: Math.fround(s.Height),
        range: Math.fround(s.Range),
        cycle: Math.fround(s.Cycle),
      })),
    );
    expect(m.effects).toEqual(
      want.Effects.map((e) => ({
        id: e.ID,
        position: f32(e.Pos),
        delay: Math.fround(e.Delay),
        params: f32(e.Param),
      })),
    );
    expect(m.emitters).toEqual(
      want.Emitters.map((e) => ({
        position: f32(e.Pos),
        texture: e.Texture,
        srcBlend: e.SrcBlend,
        dstBlend: e.DstBlend,
        color: e.Color,
        dir1: f32(e.Dir1),
        dir2: f32(e.Dir2),
        gravity: f32(e.Gravity),
        radius: f32(e.Radius),
        life: f32(e.Life),
        rate: f32(e.Rate),
        size: f32(e.Size),
        speed: Math.fround(e.Speed),
        maxCount: e.MaxCount,
        flags: e.Flags,
        scale: f32(e.Scale),
        rotate: f32(e.Rotate),
      })),
    );
    expect(m.music).toBe(want.Music);
    expect(m.deps).toEqual(want.Deps);
  });

  it('skips unknown sections', () => {
    const extra = new Uint8Array([...bytes, ...new TextEncoder().encode('NEW!\x02\x00\x00\x00ab')]);
    expect(decodeWMap(extra).width).toBe(want.Width);
  });

  it('rejects truncated and repeated sections', () => {
    for (const cut of [0, 5, 12, bytes.length - 1]) {
      expect(() => decodeWMap(bytes.subarray(0, cut))).toThrow(FormatError);
    }
    const twice = new Uint8Array([...bytes, ...bytes.subarray(10, 10 + 8 + 2 * 17)]);
    expect(() => decodeWMap(twice)).toThrow(/repeated/);
  });

  it('throws only FormatError on damaged input', () => {
    // A fixed generator keeps the test repeatable.
    let seed = 1;
    const next = (): number => {
      seed = (seed * 1103515245 + 12345) % 2 ** 31;
      return seed;
    };
    for (let i = 0; i < 2000; i++) {
      const b = bytes.slice(0, 10 + (next() % (bytes.length - 10)) + 1);
      for (let k = next() % 4; k >= 0; k--) {
        b[next() % b.length] = next() % 256;
      }
      try {
        decodeWMap(b);
      } catch (error) {
        expect(error).toBeInstanceOf(FormatError);
      }
    }
  });
});

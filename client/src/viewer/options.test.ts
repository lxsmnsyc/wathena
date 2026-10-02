import { create } from '@bufbuild/protobuf';
import { describe, expect, it } from 'vitest';
import { OptionsSchema } from '../gen/wathena/v1/event_pb';
import { PLAIN, appearance } from './options';

describe('appearance', () => {
  it('draws a unit without options as it is', () => {
    expect(appearance(undefined, false)).toEqual(PLAIN);
    expect(appearance(create(OptionsSchema), false)).toEqual(PLAIN);
  });

  it('tints stone grey and frozen blue', () => {
    expect(appearance(create(OptionsSchema, { body: 1 }), false).color).toEqual([0.6, 0.6, 0.6]);
    expect(appearance(create(OptionsSchema, { body: 6 }), false).color).toEqual([0.6, 0.6, 0.6]);
    const [r, g, b] = appearance(create(OptionsSchema, { body: 2 }), false).color;
    expect(b).toBeGreaterThan(r);
    expect(b).toBeGreaterThan(g);
  });

  it('tints poison green and darkens blindness', () => {
    const [r, g, b] = appearance(create(OptionsSchema, { health: 128 }), false).color;
    expect(g).toBeGreaterThan(r);
    expect(g).toBeGreaterThan(b);
    const blind = appearance(create(OptionsSchema, { health: 16 }), false).color;
    expect(blind).toEqual([0.6, 0.6, 0.6]);
  });

  it('hides other hidden units, and shows the player half', () => {
    for (const option of [0x2, 0x4, 0x40, 0x4000]) {
      const o = create(OptionsSchema, { option });
      expect(appearance(o, false).visible).toBe(false);
      expect(appearance(o, true)).toMatchObject({ visible: true, opacity: 0.5 });
    }
    // A cart (0x8) does not hide.
    expect(appearance(create(OptionsSchema, { option: 0x8 }), false)).toEqual(PLAIN);
  });
});

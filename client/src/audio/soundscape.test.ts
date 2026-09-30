import { describe, expect, it } from 'vitest';
import type { MapSound } from '../formats/wmap';
import Soundscape from './soundscape';

function sound(x: number, cycle: number): MapSound {
  return {
    name: `sound/at${x}`,
    position: [x, 0, 0],
    volume: 1,
    width: 0,
    height: 0,
    range: 10,
    cycle,
  };
}

const listener = { position: { x: 0, y: 0, z: 0 }, right: { x: 1, y: 0, z: 0 } };

describe('Soundscape', () => {
  it('repeats sounds in range, quieter with distance', () => {
    const played: [string, number, number][] = [];
    const s = new Soundscape(
      [sound(5, 2), sound(20, 2)],
      {
        play: (n, v = 1, p = 0) => {
          played.push([n, v, p]);
        },
      },
      () => 0,
    );
    s.update(0.1, listener);
    s.update(2, listener);
    s.update(2, listener);
    // The far sound is out of range. The near one is half as loud and on
    // the right.
    expect(played).toEqual([
      ['sound/at5', 0.5, 0.5],
      ['sound/at5', 0.5, 0.5],
      ['sound/at5', 0.5, 0.5],
    ]);
  });
});

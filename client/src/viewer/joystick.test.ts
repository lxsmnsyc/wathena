import { describe, expect, it } from 'vitest';
import {
  DEAD_ZONE,
  type Ground,
  JOYSTICK_CELLS,
  WALK_INTERVAL_MS,
  WalkRequests,
  cellDirection,
  joystickPush,
  joystickTarget,
  knobOffset,
  nearestDir,
  stopTarget,
} from './joystick';
import { step } from '../world/pathfinder';

/** A map of walkable cells from rows, north first: '.' walks, '#' does not. */
function ground(rows: string[]): Ground {
  const h = rows.length;
  const walkable = (x: number, y: number): boolean => rows[h - 1 - y]?.[x] === '.';
  return {
    walkable,
    // A straight-line path length is enough for these tests: the
    // number of diagonal steps a clear line takes, or undefined when a
    // cell on the way is blocked.
    pathLength: (x0, y0, x1, y1) => {
      let x = x0;
      let y = y0;
      let n = 0;
      while (x !== x1 || y !== y1) {
        x += Math.sign(x1 - x);
        y += Math.sign(y1 - y);
        n++;
        if (!walkable(x, y)) {
          return undefined;
        }
      }
      return n;
    },
  };
}

describe('joystickPush', () => {
  it('gives nothing in the dead zone', () => {
    expect(joystickPush(0, 0, 50, 50 * DEAD_ZONE, 0)).toBeUndefined();
    expect(joystickPush(0, 0, 0, 10, 0)).toBeUndefined();
  });

  it('gives a unit direction and a strength up to 1', () => {
    expect(joystickPush(100, 100, 50, 100, 75)).toEqual({ x: 0, y: -1, strength: 0.5 });
    const far = joystickPush(0, 0, 50, 300, 400);
    expect(far?.x).toBeCloseTo(0.6);
    expect(far?.y).toBeCloseTo(0.8);
    expect(far?.strength).toBe(1);
  });
});

describe('knobOffset', () => {
  it('follows the finger inside the radius and stops at its edge', () => {
    expect(knobOffset(0, 0, 50, 10, -20)).toEqual([10, -20]);
    expect(knobOffset(0, 0, 50, 0, 200)).toEqual([0, 50]);
  });
});

describe('cellDirection', () => {
  it('maps up to north and right to east with the default camera', () => {
    const up = cellDirection({ x: 0, y: -1 }, 0);
    expect(up[0]).toBeCloseTo(0);
    expect(up[1]).toBeCloseTo(1);
    const right = cellDirection({ x: 1, y: 0 }, 0);
    expect(right[0]).toBeCloseTo(1);
    expect(right[1]).toBeCloseTo(0);
  });

  it('turns with the camera', () => {
    // A camera east of the player looks west: up is west, right is north.
    const up = cellDirection({ x: 0, y: -1 }, Math.PI / 2);
    expect(up[0]).toBeCloseTo(-1);
    expect(up[1]).toBeCloseTo(0);
    const right = cellDirection({ x: 1, y: 0 }, Math.PI / 2);
    expect(right[0]).toBeCloseTo(0);
    expect(right[1]).toBeCloseTo(1);
    // A camera north of the player looks south.
    const back = cellDirection({ x: 0, y: -1 }, Math.PI);
    expect(back[1]).toBeCloseTo(-1);
  });
});

describe('nearestDir', () => {
  it('agrees with the pathfinder steps', () => {
    for (let d = 0; d < 8; d++) {
      const [dx, dy] = step(d);
      expect(nearestDir(dx, dy)).toBe(d);
    }
    expect(nearestDir(0.9, 0.2)).toBe(6);
  });
});

describe('joystickTarget', () => {
  const open = ground(Array.from({ length: 20 }, () => '.'.repeat(20)));

  it('aims the set number of cells ahead', () => {
    expect(joystickTarget([10, 10], [0, 1], open)).toEqual([10, 10 + JOYSTICK_CELLS]);
    expect(joystickTarget([10, 10], [1, 1], open)).toEqual([13, 13]);
    expect(joystickTarget([10, 10], [0, 0], open)).toBeUndefined();
  });

  it('aims closer in front of a wall', () => {
    const g = ground(['.....', '#####', '.....', '.....', '.....']);
    // The wall is the row y = 3.
    expect(joystickTarget([2, 0], [0, 1], g)).toEqual([2, 2]);
  });

  it('slides 45 degrees along a wall', () => {
    // A wall north and north-west of the player, open to the north-east.
    const g = ground(['......', '......', '......', '.##...', '......']);
    // From (2, 0): north is blocked at (2, 1), and north-west at (1, 1).
    expect(joystickTarget([2, 0], [0, 1], g)).toEqual([5, 3]);
  });

  it('gives nothing when every way is blocked', () => {
    const g = ground(['###', '#.#', '###']);
    expect(joystickTarget([1, 1], [0, 1], g)).toBeUndefined();
  });

  it('refuses a cell whose path goes far around', () => {
    const g: Ground = {
      walkable: () => true,
      pathLength: () => 20,
    };
    expect(joystickTarget([5, 5], [1, 0], g)).toBeUndefined();
  });
});

describe('stopTarget', () => {
  it('is one step on in the pushed direction, when it can be walked', () => {
    const g = ground(['...', '...', '...']);
    expect(stopTarget([1, 1], [0.1, 1], g)).toEqual([1, 2]);
    expect(stopTarget([1, 1], [-1, -1], g)).toEqual([0, 0]);
    expect(stopTarget([2, 2], [1, 0], g)).toBeUndefined();
  });
});

describe('WalkRequests', () => {
  it('sends a new cell at most once per interval', () => {
    const w = new WalkRequests();
    expect(w.next(0, [1, 1])).toEqual([1, 1]);
    expect(w.next(10, [1, 2])).toBeUndefined();
    expect(w.next(WALK_INTERVAL_MS, [1, 2])).toEqual([1, 2]);
  });

  it('says when a cell may go out', () => {
    const w = new WalkRequests();
    expect(w.due(0)).toBe(true);
    w.next(0, [1, 1]);
    expect(w.due(WALK_INTERVAL_MS - 1)).toBe(false);
    expect(w.due(WALK_INTERVAL_MS)).toBe(true);
  });

  it('never sends the same cell twice in a row', () => {
    const w = new WalkRequests();
    w.next(0, [1, 1]);
    expect(w.next(WALK_INTERVAL_MS * 4, [1, 1])).toBeUndefined();
    expect(w.next(WALK_INTERVAL_MS * 4, undefined)).toBeUndefined();
    w.reset();
    expect(w.next(WALK_INTERVAL_MS * 5, [1, 1])).toEqual([1, 1]);
  });

  it('sends a last cell at once, then waits the interval again', () => {
    const w = new WalkRequests();
    w.next(0, [1, 1]);
    expect(w.final(10, [1, 2])).toEqual([1, 2]);
    expect(w.next(20, [5, 5])).toBeUndefined();
    expect(w.next(10 + WALK_INTERVAL_MS, [5, 5])).toEqual([5, 5]);
    // A last cell that was just sent is not sent again.
    expect(w.final(10 + WALK_INTERVAL_MS, [5, 5])).toBeUndefined();
  });

  it('stays under 5 requests a second for a target that changes every frame', () => {
    const w = new WalkRequests();
    let sent = 0;
    for (let t = 0; t < 10_000; t += 16) {
      if (w.next(t, [t, 0]) !== undefined) {
        sent++;
      }
    }
    expect(sent).toBeLessThanOrEqual(10 * (1000 / WALK_INTERVAL_MS));
  });
});

import { describe, expect, it } from 'vitest';
import { GestureRecognizer, LONG_PRESS_MS, SLOP } from './gestures';

describe('GestureRecognizer', () => {
  it('taps on a short touch that stays still', () => {
    const g = new GestureRecognizer();
    expect(g.down(1, 100, 100, 0)).toEqual([]);
    expect(g.move(1, 103, 104, 50)).toEqual([]);
    expect(g.up(1, 103, 104, 120)).toEqual([{ kind: 'tap', x: 100, y: 100 }]);
    expect(g.active).toBe(false);
  });

  it('presses after the long press time, then holds until the finger lifts', () => {
    const g = new GestureRecognizer();
    g.down(1, 50, 60, 1000);
    expect(g.pressAt).toBe(1000 + LONG_PRESS_MS);
    expect(g.tick(1000 + LONG_PRESS_MS - 1)).toEqual([]);
    expect(g.tick(1000 + LONG_PRESS_MS)).toEqual([{ kind: 'press', x: 50, y: 60 }]);
    expect(g.tick(2000)).toEqual([]);
    expect(g.move(1, 80, 60, 2000)).toEqual([{ kind: 'hold', phase: 'move', x: 80, y: 60 }]);
    expect(g.up(1, 80, 60, 2100)).toEqual([{ kind: 'hold', phase: 'end', x: 80, y: 60 }]);
  });

  it('fires a due press before the up, so a late timer does not make a tap', () => {
    const g = new GestureRecognizer();
    g.down(1, 10, 10, 0);
    expect(g.up(1, 10, 10, LONG_PRESS_MS + 5)).toEqual([
      { kind: 'press', x: 10, y: 10 },
      { kind: 'hold', phase: 'end', x: 10, y: 10 },
    ]);
  });

  it('holds when the finger moves past the slop', () => {
    const g = new GestureRecognizer();
    g.down(1, 0, 0, 0);
    expect(g.move(1, SLOP, 0, 10)).toEqual([]);
    expect(g.move(1, SLOP + 1, 0, 20)).toEqual([
      { kind: 'hold', phase: 'start', x: SLOP + 1, y: 0 },
      { kind: 'hold', phase: 'move', x: SLOP + 1, y: 0 },
    ]);
    // A hold never becomes a press or a tap.
    expect(g.tick(LONG_PRESS_MS * 2)).toEqual([]);
    expect(g.up(1, 40, 0, LONG_PRESS_MS * 2)).toEqual([
      { kind: 'hold', phase: 'end', x: 40, y: 0 },
    ]);
  });

  it('ignores other pointers while one finger is down', () => {
    const g = new GestureRecognizer();
    g.down(1, 0, 0, 0);
    expect(g.move(2, 100, 100, 10)).toEqual([]);
    expect(g.up(2, 100, 100, 20)).toEqual([]);
    expect(g.up(1, 0, 0, 30)).toEqual([{ kind: 'tap', x: 0, y: 0 }]);
  });

  it('pinches with two fingers: scale, turn and the middle point', () => {
    const g = new GestureRecognizer();
    g.down(1, 100, 100, 0);
    expect(g.down(2, 200, 100, 10)).toEqual([
      { kind: 'pinch', phase: 'start', scale: 1, rotation: 0, dx: 0, dy: 0 },
    ]);
    // Apart to twice the distance.
    const [apart] = g.move(2, 300, 100, 20);
    expect(apart).toMatchObject({ kind: 'pinch', phase: 'move', scale: 2, dx: 50, dy: 0 });
    expect(apart?.kind === 'pinch' ? apart.rotation : NaN).toBeCloseTo(0);
    // A quarter turn clockwise on screen, around the first finger.
    const [twist] = g.move(2, 100, 300, 30);
    expect(twist?.kind === 'pinch' ? twist.rotation : NaN).toBeCloseTo(Math.PI / 2);
    expect(twist?.kind === 'pinch' ? twist.scale : NaN).toBeCloseTo(1);
    expect(g.up(1, 100, 100, 40)).toEqual([
      { kind: 'pinch', phase: 'end', scale: 1, rotation: 0, dx: 0, dy: 0 },
    ]);
    // The second finger lifting makes no tap.
    expect(g.up(2, 100, 300, 50)).toEqual([]);
    expect(g.active).toBe(false);
  });

  it('reports a turn across the -π to π line as a small turn', () => {
    const g = new GestureRecognizer();
    g.down(1, 0, 0, 0);
    g.down(2, -100, 1, 0);
    const [m] = g.move(2, -100, -1, 10);
    expect(m?.kind === 'pinch' ? Math.abs(m.rotation) : NaN).toBeLessThan(0.1);
  });

  it('ends a hold when a second finger comes', () => {
    const g = new GestureRecognizer();
    g.down(1, 0, 0, 0);
    g.move(1, 50, 0, 10);
    expect(g.down(2, 100, 0, 20)).toEqual([
      { kind: 'hold', phase: 'end', x: 50, y: 0 },
      { kind: 'pinch', phase: 'start', scale: 1, rotation: 0, dx: 0, dy: 0 },
    ]);
  });

  it('waits for every finger to lift after a pinch, a third one too', () => {
    const g = new GestureRecognizer();
    g.down(1, 0, 0, 0);
    g.down(2, 10, 0, 0);
    expect(g.down(3, 20, 0, 0)).toEqual([]);
    g.up(2, 10, 0, 10);
    g.down(4, 30, 0, 20);
    g.up(1, 0, 0, 30);
    expect(g.active).toBe(true);
    g.up(4, 30, 0, 40);
    expect(g.active).toBe(false);
    expect(g.down(5, 0, 0, 50)).toEqual([]);
    expect(g.up(5, 0, 0, 60)).toEqual([{ kind: 'tap', x: 0, y: 0 }]);
  });

  it('cancels without a tap', () => {
    const g = new GestureRecognizer();
    g.down(1, 0, 0, 0);
    expect(g.cancel()).toEqual([]);
    expect(g.up(1, 0, 0, 10)).toEqual([]);
    g.down(1, 0, 0, 0);
    g.move(1, 30, 0, 10);
    expect(g.cancel()).toEqual([{ kind: 'hold', phase: 'end', x: 30, y: 0 }]);
  });
});

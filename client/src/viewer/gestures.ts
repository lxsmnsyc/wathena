// Touch gestures on the game view (review decision 39). The recognizer
// is plain logic: it takes pointer downs, moves and ups with their times,
// and returns the gestures they make. main.ts feeds it touch pointers
// only, so a mouse keeps its own handling.
//
// - A tap is a touch that ends before the long press time without moving
//   past the slop.
// - A press is a touch held still for the long press time.
// - A hold is one finger that moved past the slop, or that goes on after
//   a press. It ends when the finger lifts.
// - A pinch is two fingers. Each move reports the change since the last
//   one: the distance ratio, the turn of the line between the fingers,
//   and the move of their middle point.

/** How far a finger may move, in CSS pixels, and still tap or press. */
export const SLOP = 10;
/** How long a finger stays still for a press, in milliseconds. */
export const LONG_PRESS_MS = 500;

export type Gesture =
  | { kind: 'tap'; x: number; y: number }
  | { kind: 'press'; x: number; y: number }
  | { kind: 'hold'; phase: 'start' | 'move' | 'end'; x: number; y: number }
  | {
      kind: 'pinch';
      phase: 'start' | 'move' | 'end';
      /** The distance between the fingers over the last distance. */
      scale: number;
      /** The turn since the last move, in radians, clockwise on screen. */
      rotation: number;
      /** The move of the middle point since the last move, in pixels. */
      dx: number;
      dy: number;
    };

interface Finger {
  id: number;
  x: number;
  y: number;
}

type State =
  | { kind: 'idle' }
  | { kind: 'down'; finger: Finger; startX: number; startY: number; at: number }
  | { kind: 'pressed'; finger: Finger }
  | { kind: 'hold'; finger: Finger }
  | {
      kind: 'pinch';
      a: Finger;
      b: Finger;
      distance: number;
      angle: number;
      midX: number;
      midY: number;
    }
  // After a pinch, nothing happens until every finger lifts.
  | { kind: 'done'; fingers: Set<number> };

export class GestureRecognizer {
  private state: State = { kind: 'idle' };

  constructor(
    private readonly slop = SLOP,
    private readonly longPressMs = LONG_PRESS_MS,
  ) {}

  /** Whether a finger is down. */
  get active(): boolean {
    return this.state.kind !== 'idle';
  }

  /** When a press would fire, for a timer, or undefined. */
  get pressAt(): number | undefined {
    return this.state.kind === 'down' ? this.state.at + this.longPressMs : undefined;
  }

  down(id: number, x: number, y: number, now: number): Gesture[] {
    const s = this.state;
    switch (s.kind) {
      case 'idle':
        this.state = { kind: 'down', finger: { id, x, y }, startX: x, startY: y, at: now };
        return [];
      case 'down':
      case 'pressed':
      case 'hold': {
        const out: Gesture[] = [];
        if (s.kind === 'hold' || s.kind === 'pressed') {
          out.push({ kind: 'hold', phase: 'end', x: s.finger.x, y: s.finger.y });
        }
        const a = s.finger;
        const b = { id, x, y };
        this.state = { kind: 'pinch', a, b, ...measure(a, b) };
        out.push({ kind: 'pinch', phase: 'start', scale: 1, rotation: 0, dx: 0, dy: 0 });
        return out;
      }
      case 'pinch':
        // A third finger is ignored, and counted until it lifts.
        return [];
      case 'done':
        s.fingers.add(id);
        return [];
      default:
        return [];
    }
  }

  move(id: number, x: number, y: number, now: number): Gesture[] {
    const s = this.state;
    switch (s.kind) {
      case 'down': {
        if (s.finger.id !== id) {
          return [];
        }
        s.finger.x = x;
        s.finger.y = y;
        // A press that is due fires before the move counts.
        const out = this.tick(now);
        if (this.state.kind !== 'down') {
          return [...out, ...this.move(id, x, y, now)];
        }
        if (Math.hypot(x - s.startX, y - s.startY) > this.slop) {
          this.state = { kind: 'hold', finger: s.finger };
          return [
            { kind: 'hold', phase: 'start', x, y },
            { kind: 'hold', phase: 'move', x, y },
          ];
        }
        return [];
      }
      case 'pressed':
      case 'hold':
        if (s.finger.id !== id) {
          return [];
        }
        s.finger.x = x;
        s.finger.y = y;
        return [{ kind: 'hold', phase: 'move', x, y }];
      case 'pinch': {
        const f = [s.a, s.b].find((finger) => finger.id === id);
        if (f === undefined) {
          return [];
        }
        f.x = x;
        f.y = y;
        const m = measure(s.a, s.b);
        const g: Gesture = {
          kind: 'pinch',
          phase: 'move',
          scale: s.distance > 0 && m.distance > 0 ? m.distance / s.distance : 1,
          rotation: turn(m.angle - s.angle),
          dx: m.midX - s.midX,
          dy: m.midY - s.midY,
        };
        s.distance = m.distance;
        s.angle = m.angle;
        s.midX = m.midX;
        s.midY = m.midY;
        return [g];
      }
      default:
        return [];
    }
  }

  up(id: number, x: number, y: number, now: number): Gesture[] {
    const s = this.state;
    switch (s.kind) {
      case 'down': {
        if (s.finger.id !== id) {
          return [];
        }
        const out = this.tick(now);
        if (this.state.kind === 'down') {
          this.state = { kind: 'idle' };
          return [{ kind: 'tap', x: s.startX, y: s.startY }];
        }
        return [...out, ...this.up(id, x, y, now)];
      }
      case 'pressed':
      case 'hold':
        if (s.finger.id !== id) {
          return [];
        }
        this.state = { kind: 'idle' };
        return [{ kind: 'hold', phase: 'end', x, y }];
      case 'pinch': {
        if (s.a.id !== id && s.b.id !== id) {
          return [];
        }
        const other = s.a.id === id ? s.b.id : s.a.id;
        this.state = { kind: 'done', fingers: new Set([other]) };
        return [{ kind: 'pinch', phase: 'end', scale: 1, rotation: 0, dx: 0, dy: 0 }];
      }
      case 'done':
        s.fingers.delete(id);
        if (s.fingers.size === 0) {
          this.state = { kind: 'idle' };
        }
        return [];
      default:
        return [];
    }
  }

  /** Ends every gesture without a tap, as when the browser takes the touch. */
  cancel(): Gesture[] {
    const s = this.state;
    this.state = { kind: 'idle' };
    switch (s.kind) {
      case 'pressed':
      case 'hold':
        return [{ kind: 'hold', phase: 'end', x: s.finger.x, y: s.finger.y }];
      case 'pinch':
        return [{ kind: 'pinch', phase: 'end', scale: 1, rotation: 0, dx: 0, dy: 0 }];
      default:
        return [];
    }
  }

  /** Fires a press once a finger has stayed still long enough. */
  tick(now: number): Gesture[] {
    const s = this.state;
    if (s.kind !== 'down' || now - s.at < this.longPressMs) {
      return [];
    }
    this.state = { kind: 'pressed', finger: s.finger };
    return [{ kind: 'press', x: s.startX, y: s.startY }];
  }
}

function measure(
  a: Finger,
  b: Finger,
): { distance: number; angle: number; midX: number; midY: number } {
  return {
    distance: Math.hypot(b.x - a.x, b.y - a.y),
    angle: Math.atan2(b.y - a.y, b.x - a.x),
    midX: (a.x + b.x) / 2,
    midY: (a.y + b.y) / 2,
  };
}

/** An angle change in the range -π to π. */
function turn(a: number): number {
  let t = a;
  while (t > Math.PI) {
    t -= 2 * Math.PI;
  }
  while (t < -Math.PI) {
    t += 2 * Math.PI;
  }
  return t;
}

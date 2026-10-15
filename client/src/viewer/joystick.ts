// The virtual joystick and walking by touch (review decision 39).
//
// RO has no "walk this way" command: the client asks for a walk to a
// cell, and the server finds the path. So the joystick walks by asking
// for a cell a few steps ahead in the pushed direction, and asks again as
// the player moves or the direction changes. The same requests walk
// toward a finger held on the ground.
//
// The gateway has no rate limit of its own for walks. Every command goes
// into the zone's queue, which holds 1,024 commands for all its players,
// and a full queue ends the connection with "Too many actions at once".
// So WalkRequests sends a new cell at most once per WALK_INTERVAL_MS, and
// only when the cell changed.

import type { Dir } from '../world/pathfinder';

/** How far ahead the joystick aims, in cells. */
export const JOYSTICK_CELLS = 4;
/** The shortest time between two walk requests, in milliseconds. */
export const WALK_INTERVAL_MS = 250;
/** The part of the radius that does nothing, from 0 to 1. */
export const DEAD_ZONE = 0.2;

/** A joystick push: a unit direction on screen (y down) and its strength. */
export interface Push {
  x: number;
  y: number;
  /** From 0 to 1, how far the knob is from the center. */
  strength: number;
}

/**
 * The push of a point on a joystick whose center and radius are given,
 * in screen pixels. A point in the dead zone pushes nothing.
 */
export function joystickPush(
  centerX: number,
  centerY: number,
  radius: number,
  x: number,
  y: number,
): Push | undefined {
  const dx = x - centerX;
  const dy = y - centerY;
  const d = Math.hypot(dx, dy);
  if (radius <= 0 || d / radius <= DEAD_ZONE) {
    return undefined;
  }
  return { x: dx / d, y: dy / d, strength: Math.min(1, d / radius) };
}

/** Where the knob shows, clamped to the radius. */
export function knobOffset(
  centerX: number,
  centerY: number,
  radius: number,
  x: number,
  y: number,
): [number, number] {
  const dx = x - centerX;
  const dy = y - centerY;
  const d = Math.hypot(dx, dy);
  if (d <= radius || d === 0) {
    return [dx, dy];
  }
  return [(dx / d) * radius, (dy / d) * radius];
}

/**
 * The direction on the map, in cells (x east, y north), of a push on
 * screen. azimuth is the camera's turn around the vertical axis, as
 * OrbitControls.getAzimuthalAngle gives it: 0 when the camera looks
 * north from the south, as RO's camera does.
 */
export function cellDirection(push: { x: number; y: number }, azimuth: number): [number, number] {
  // Screen right and up, from y down.
  const r = push.x;
  const u = -push.y;
  const s = Math.sin(azimuth);
  const c = Math.cos(azimuth);
  // World x is cell x, and world z is minus cell y. The camera's right is
  // (cos a, -sin a) in x and z, and its forward is (-sin a, -cos a).
  const x = r * c - u * s;
  const z = -r * s - u * c;
  return [x, -z];
}

/** The 8 walk directions' offsets, as world/pathfinder's step. */
const STEPS: readonly (readonly [number, number])[] = [
  [0, 1],
  [-1, 1],
  [-1, 0],
  [-1, -1],
  [0, -1],
  [1, -1],
  [1, 0],
  [1, 1],
];

/** The walk direction nearest a direction in cells. */
export function nearestDir(dx: number, dy: number): Dir {
  // Dir 0 is north, turning counterclockwise in 45° steps.
  const a = Math.atan2(-dx, dy);
  return ((Math.round(a / (Math.PI / 4)) % 8) + 8) % 8;
}

/** What joystickTarget needs to know about the map. */
export interface Ground {
  walkable(x: number, y: number): boolean;
  /** The length of the walk path between two cells, or undefined. */
  pathLength(x0: number, y0: number, x1: number, y1: number): number | undefined;
}

/**
 * The cell to walk to for a direction in cells, from a cell. It aims
 * `cells` ahead, and closer when that cell cannot be reached by a short
 * path. When nothing ahead can be reached, it tries 45° to each side, so
 * the player slides along a wall. Undefined means the way is blocked.
 */
export function joystickTarget(
  from: readonly [number, number],
  dir: readonly [number, number],
  ground: Ground,
  cells = JOYSTICK_CELLS,
): [number, number] | undefined {
  const len = Math.hypot(dir[0], dir[1]);
  if (len === 0) {
    return undefined;
  }
  const ux = dir[0] / len;
  const uy = dir[1] / len;
  const tries: [number, number][] = [[ux, uy]];
  for (const turnBy of [Math.PI / 4, -Math.PI / 4]) {
    const c = Math.cos(turnBy);
    const s = Math.sin(turnBy);
    tries.push([ux * c - uy * s, ux * s + uy * c]);
  }
  for (const [tx, ty] of tries) {
    for (let d = cells; d >= 1; d--) {
      const x = from[0] + Math.round(tx * d);
      const y = from[1] + Math.round(ty * d);
      if (x === from[0] && y === from[1]) {
        continue;
      }
      if (!ground.walkable(x, y)) {
        continue;
      }
      const n = ground.pathLength(from[0], from[1], x, y);
      // A path much longer than the straight line goes around a wall,
      // which is not where the player pushed.
      if (n !== undefined && n > 0 && n <= d + 2) {
        return [x, y];
      }
    }
  }
  return undefined;
}

/**
 * The cell one step past `from` in a direction, to end a walk soon after
 * the joystick is let go. The server keeps the current step and walks
 * from its end, so a walk to the next cell itself would change nothing.
 */
export function stopTarget(
  from: readonly [number, number],
  dir: readonly [number, number],
  ground: Pick<Ground, 'walkable'>,
): [number, number] | undefined {
  const s = STEPS[nearestDir(dir[0], dir[1])];
  if (s === undefined) {
    return undefined;
  }
  const x = from[0] + s[0];
  const y = from[1] + s[1];
  return ground.walkable(x, y) ? [x, y] : undefined;
}

/**
 * Walk requests from the joystick or a held finger: a new cell at most
 * once per interval, and never the same cell twice in a row.
 */
export class WalkRequests {
  private last: [number, number] | undefined;
  private sentAt = -Infinity;

  constructor(private readonly interval = WALK_INTERVAL_MS) {}

  /** Whether a new cell may go out now, so the caller can skip finding one. */
  due(now: number): boolean {
    return now - this.sentAt >= this.interval;
  }

  /**
   * The cell to send now for a wanted cell, or undefined to send
   * nothing yet.
   */
  next(now: number, cell: readonly [number, number] | undefined): [number, number] | undefined {
    if (cell === undefined || now - this.sentAt < this.interval) {
      return undefined;
    }
    if (this.last !== undefined && this.last[0] === cell[0] && this.last[1] === cell[1]) {
      return undefined;
    }
    this.last = [cell[0], cell[1]];
    this.sentAt = now;
    return [cell[0], cell[1]];
  }

  /**
   * The last cell to send when a walk ends: it waits for no interval, so
   * the player stops soon, but is never the cell sent last.
   */
  final(now: number, cell: readonly [number, number] | undefined): [number, number] | undefined {
    if (cell === undefined) {
      return undefined;
    }
    if (this.last !== undefined && this.last[0] === cell[0] && this.last[1] === cell[1]) {
      this.reset();
      return undefined;
    }
    this.reset();
    this.sentAt = now;
    return [cell[0], cell[1]];
  }

  /** Forgets the last cell, as when a new walk starts. */
  reset(): void {
    this.last = undefined;
  }
}

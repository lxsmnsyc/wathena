import type { Group, Vector3 } from 'three';
import { type WMap, cellHeight, walkable } from '../formats/wmap';
import {
  DEFAULT_SPEED,
  type Dir,
  type Pathfinder,
  facing,
  step,
  stepTime,
} from '../world/pathfinder';

// A character that walks where the user clicks, along the shared
// pathfinding and rAthena's step timing. The game will move it from
// server messages instead.

/** What a walker draws: a character, or a sprite such as a monster's. */
export interface Body {
  readonly group: Group;
  direction: number;
  play(action: number): void;
  update(deltaMs: number, cameraSteps: number, towardCamera: Vector3): void;
}

export default class Player {
  /** The cell the player stands on, or is stepping from. */
  x: number;
  y: number;
  private path: Dir[] = [];
  private elapsed = 0;
  /** A destination to take once the current step ends. */
  private next: [number, number] | undefined;

  constructor(
    readonly character: Body,
    private readonly map: WMap,
    private readonly pathfinder: Pathfinder,
    x: number,
    y: number,
    private readonly speed = DEFAULT_SPEED,
  ) {
    this.x = x;
    this.y = y;
    this.place(x + 0.5, y + 0.5);
  }

  /** The walkable cell nearest the map's center. */
  static start(map: WMap): [number, number] {
    const cx = Math.floor(map.width / 2);
    const cy = Math.floor(map.height / 2);
    for (let r = 0; r < Math.max(map.width, map.height); r++) {
      for (let dy = -r; dy <= r; dy++) {
        for (let dx = -r; dx <= r; dx++) {
          if (walkable(map, cx + dx, cy + dy)) {
            return [cx + dx, cy + dy];
          }
        }
      }
    }
    return [cx, cy];
  }

  /** Walks to a cell. Returns false if there is no path. */
  moveTo(x: number, y: number): boolean {
    if (this.path.length > 0) {
      // Finish the step under way, then turn toward the new goal.
      this.next = [x, y];
      return true;
    }
    return this.plan(x, y);
  }

  /**
   * Follows a walk the server sent: from a cell, the steps in rAthena's
   * directions, starting startMs from now (negative if already under way).
   */
  follow(fromX: number, fromY: number, steps: Dir[], startMs: number): void {
    this.x = fromX;
    this.y = fromY;
    this.path = [...steps];
    this.next = undefined;
    this.elapsed = -startMs;
    this.place(fromX + 0.5, fromY + 0.5);
  }

  /** Stands on a cell, facing a direction (0 south, clockwise). */
  stand(x: number, y: number, dir: number): void {
    this.x = x;
    this.y = y;
    this.path = [];
    this.character.direction = dir;
    this.place(x + 0.5, y + 0.5);
  }

  private plan(x: number, y: number): boolean {
    const path = this.pathfinder.search(this.x, this.y, x, y);
    if (path === undefined || path.length === 0) {
      return false;
    }
    this.path = path;
    this.elapsed = 0;
    return true;
  }

  update(deltaMs: number, cameraSteps: number, towardCamera: Vector3): void {
    let d = this.path[0];
    if (d !== undefined) {
      this.elapsed += deltaMs;
      // Finish every step whose time has passed.
      while (d !== undefined && this.elapsed >= stepTime(d, this.speed)) {
        this.elapsed -= stepTime(d, this.speed);
        const [dx, dy] = step(d);
        this.x += dx;
        this.y += dy;
        this.path.shift();
        if (this.next !== undefined) {
          const [nx, ny] = this.next;
          this.next = undefined;
          this.path = [];
          this.plan(nx, ny);
        }
        d = this.path[0];
      }
      if (d === undefined) {
        this.elapsed = 0;
        this.place(this.x + 0.5, this.y + 0.5);
      } else {
        const [dx, dy] = step(d);
        const t = Math.max(0, this.elapsed) / stepTime(d, this.speed);
        this.place(this.x + 0.5 + dx * t, this.y + 0.5 + dy * t);
        this.character.direction = facing(d);
      }
    }
    this.character.play(this.path.length > 0 ? 1 : 0);
    this.character.update(deltaMs, cameraSteps, towardCamera);
  }

  private place(x: number, y: number): void {
    this.character.group.position.set(x, cellHeight(this.map, x, y), -y);
  }
}

import type { Vector3 } from 'three';
import { type WMap, cellHeight, walkable } from '../formats/wmap';
import type { Character } from '../render/characters';
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

export default class Player {
  /** The cell the player stands on, or is stepping from. */
  x: number;
  y: number;
  private path: Dir[] = [];
  private elapsed = 0;
  /** A destination to take once the current step ends. */
  private next: [number, number] | undefined;

  constructor(
    readonly character: Character,
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
        const t = this.elapsed / stepTime(d, this.speed);
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

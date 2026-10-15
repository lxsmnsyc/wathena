import type { Group, Vector3 } from 'three';
import { type WMap, cellHeight, walkable } from '../formats/wmap';
import { type Appearance, PLAIN } from './options';
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
  /** Multiplies the body's colors and opacity, from 0 to 1. */
  setTint?(r: number, g: number, b: number, a: number): void;
}

export default class Player {
  /** The cell the player stands on, or is stepping from. */
  x: number;
  y: number;
  private path: Dir[] = [];
  private elapsed = 0;
  /** A destination to take once the current step ends. */
  private next: [number, number] | undefined;
  /** An action that plays over walking and standing, until a time. */
  private override: { action: number; until: number } | undefined;
  private clock = 0;
  private body: Body;
  /** How the player's status options make it look. */
  private look: Appearance = PLAIN;

  constructor(
    character: Body,
    private readonly map: WMap,
    private readonly pathfinder: Pathfinder,
    x: number,
    y: number,
    private readonly speed = DEFAULT_SPEED,
  ) {
    this.body = character;
    this.x = x;
    this.y = y;
    this.place(x + 0.5, y + 0.5);
  }

  /** The cell the player stands on, or the one it steps into now. */
  get nextCell(): [number, number] {
    const d = this.path[0];
    if (d === undefined) {
      return [this.x, this.y];
    }
    const [dx, dy] = step(d);
    return [this.x + dx, this.y + dy];
  }

  /** What the player draws. */
  get character(): Body {
    return this.body;
  }

  /**
   * Draws the player with another body, such as after a change of
   * equipment. The new body takes the old one's place and direction. The
   * action carries over, since update sets it on every frame. The caller
   * moves the body's group in the scene.
   */
  replace(body: Body): void {
    body.direction = this.body.direction;
    body.group.position.copy(this.body.group.position);
    this.body = body;
    this.applyLook();
  }

  /** How the player looks, such as tinted while frozen or hidden. */
  get appearance(): Appearance {
    return this.look;
  }

  /** Shows the player as its status options make it look. */
  set appearance(look: Appearance) {
    this.look = look;
    this.applyLook();
  }

  private applyLook(): void {
    const [r, g, b] = this.look.color;
    this.body.setTint?.(r, g, b, this.look.opacity);
    this.body.group.visible = this.look.visible;
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
    this.override = undefined;
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

  /**
   * Plays an action, such as an attack, for a time in milliseconds.
   * Infinity holds it, as for a death.
   */
  act(action: number, ms: number): void {
    this.override = { action, until: this.clock + ms };
  }

  /** Faces a cell. */
  face(x: number, y: number): void {
    const dx = Math.sign(x - this.x);
    const dy = Math.sign(y - this.y);
    const dirs: Record<string, number> = {
      '0,-1': 0,
      '-1,-1': 1,
      '-1,0': 2,
      '-1,1': 3,
      '0,1': 4,
      '1,1': 5,
      '1,0': 6,
      '1,-1': 7,
    };
    const f = dirs[`${dx},${dy}`];
    if (f !== undefined) {
      this.character.direction = f;
    }
  }

  update(deltaMs: number, cameraSteps: number, towardCamera: Vector3): void {
    this.clock += deltaMs;
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
    if (
      this.override !== undefined &&
      (this.clock >= this.override.until || this.path.length > 0)
    ) {
      this.override = undefined;
    }
    this.character.play(this.override?.action ?? (this.path.length > 0 ? 1 : 0));
    this.character.update(deltaMs, cameraSteps, towardCamera);
  }

  private place(x: number, y: number): void {
    this.character.group.position.set(x, cellHeight(this.map, x, y), -y);
  }
}

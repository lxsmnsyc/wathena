import { Group } from 'three';
import { type WMap, cellHeight, walkable } from '../formats/wmap';
import { type SpriteCache, SpriteView } from '../render/sprites';

// Monsters that wander a map, to test sprites in the viewer. The game
// moves entities from server messages instead.

/** Milliseconds per cell at RO's default walk speed. */
const WALK_MS = 150;

/** Steps to neighboring cells, with y north, and the facing for each: 0 to 7 from south, clockwise. */
const STEPS: [number, number, number][] = [
  [0, -1, 0],
  [-1, -1, 1],
  [-1, 0, 2],
  [-1, 1, 3],
  [0, 1, 4],
  [1, 1, 5],
  [1, 0, 6],
  [1, -1, 7],
];

function facing(dx: number, dy: number): number | undefined {
  return STEPS.find(([x, y]) => x === dx && y === dy)?.[2];
}

interface Walker {
  view: SpriteView;
  x: number;
  y: number;
  path: [number, number][];
  wait: number;
  progress: number;
}

export default class Walkers {
  readonly group = new Group();
  private readonly walkers: Walker[] = [];
  // A fixed generator keeps each map's placement the same between loads.
  private seed = 1;

  constructor(private readonly map: WMap) {
    this.group.name = 'walkers';
  }

  private random(n: number): number {
    this.seed = (this.seed * 1103515245 + 12345) % 2 ** 31;
    return this.seed % n;
  }

  /** Places count sprites on walkable cells near the map's center. */
  async add(sprites: SpriteCache, names: string[], count: number): Promise<void> {
    const m = this.map;
    const spots: [number, number][] = [];
    for (let tries = 0; spots.length < count && tries < count * 50; tries++) {
      const x = Math.floor(m.width / 2) + this.random(61) - 30;
      const y = Math.floor(m.height / 2) + this.random(61) - 30;
      if (walkable(m, x, y)) {
        spots.push([x, y]);
      }
    }
    const assets = await Promise.all(names.map(async (n) => sprites.get(n)));
    spots.forEach(([x, y], i) => {
      const asset = assets[i % assets.length];
      if (asset === undefined) {
        return;
      }
      const view = new SpriteView(asset);
      view.direction = this.random(8);
      const w: Walker = { view, x, y, path: [], wait: this.random(3000), progress: 0 };
      this.place(w, x + 0.5, y + 0.5);
      this.group.add(view.group);
      this.walkers.push(w);
    });
  }

  update(deltaMs: number, cameraSteps: number): void {
    for (const w of this.walkers) {
      this.step(w, deltaMs);
      w.view.update(deltaMs, cameraSteps);
    }
  }

  private step(w: Walker, deltaMs: number): void {
    if (w.path.length === 0) {
      w.view.play(0);
      w.wait -= deltaMs;
      if (w.wait <= 0) {
        this.plan(w);
        w.wait = 1000 + this.random(4000);
      }
      return;
    }
    const [nx, ny] = w.path[0]!;
    const dx = nx - w.x;
    const dy = ny - w.y;
    w.view.play(1);
    w.view.direction = facing(dx, dy) ?? w.view.direction;
    const cellMs = dx !== 0 && dy !== 0 ? WALK_MS * Math.SQRT2 : WALK_MS;
    w.progress += deltaMs / cellMs;
    if (w.progress >= 1) {
      w.progress -= 1;
      w.x = nx;
      w.y = ny;
      w.path.shift();
      if (w.path.length === 0) {
        w.progress = 0;
      }
    }
    const t = Math.min(w.progress, 1);
    this.place(w, w.x + dx * t + 0.5, w.y + dy * t + 0.5);
  }

  /** Picks a short walk in one direction over walkable cells. */
  private plan(w: Walker): void {
    const [dx, dy] = STEPS[this.random(STEPS.length)]!;
    let x = w.x;
    let y = w.y;
    for (let n = 1 + this.random(5); n > 0; n--) {
      if (!walkable(this.map, x + dx, y + dy)) {
        break;
      }
      x += dx;
      y += dy;
      w.path.push([x, y]);
    }
  }

  private place(w: Walker, x: number, y: number): void {
    w.view.group.position.set(x, cellHeight(this.map, x, y), -y);
  }
}

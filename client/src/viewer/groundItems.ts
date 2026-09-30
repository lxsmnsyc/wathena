import { Group, Vector3 } from 'three';
import { type WMap, cellHeight } from '../formats/wmap';
import type { GroundItem } from '../gen/wathena/v1/event_pb';
import { type SpriteCache, SpriteView } from '../render/sprites';

// Items lying on the ground, drawn with their item sprites, sprite/item/<id>.
// Each lies at a place inside its cell that the server chooses. An item
// that has just dropped jumps up, falls, and bounces once.

/** Places per cell side in GroundItem's sub_x and sub_y. */
const SUBCELLS = 16;

/**
 * Where an item lies, in cells: its cell plus its place inside the cell.
 * sub values are 0 to 15, and larger values count as 15.
 */
export function itemPosition(x: number, y: number, subX: number, subY: number): [number, number] {
  const sub = (s: number): number => (Math.min(Math.max(s, 0), SUBCELLS - 1) + 0.5) / SUBCELLS;
  return [x + sub(subX), y + sub(subY)];
}

/** The label under a ground item, as RO shows it on hover: "Red Potion: 5 ea." */
export function groundItemLabel(name: string, amount: number): string {
  return amount > 1 ? `${name}: ${amount} ea.` : name;
}

/** How long a drop takes, in milliseconds. */
export const DROP_MS = 600;
/** The first jump's time, and its height in cells. The bounce takes the rest. */
const JUMP_MS = 420;
const JUMP = 1.2;
const BOUNCE = 0.2;

/** How high a dropping item is above the ground, in cells, ms after it dropped. */
export function dropHeight(ms: number): number {
  if (ms <= 0 || ms >= DROP_MS) {
    return 0;
  }
  // Two arcs: a jump, then a small bounce. Each is a parabola.
  if (ms < JUMP_MS) {
    const t = ms / JUMP_MS;
    return 4 * JUMP * t * (1 - t);
  }
  const t = (ms - JUMP_MS) / (DROP_MS - JUMP_MS);
  return 4 * BOUNCE * t * (1 - t);
}

interface Item {
  itemId: number;
  amount: number;
  /** The cell. */
  x: number;
  y: number;
  /** Where the item lies on the ground, in world space. */
  position: Vector3;
  /** Milliseconds since the item appeared. */
  age: number;
  dropped: boolean;
  view: SpriteView | undefined;
}

export default class GroundItems {
  readonly group = new Group();
  private readonly items = new Map<number, Item>();

  constructor(private readonly sprites: SpriteCache) {
    this.group.name = 'items';
  }

  /** Shows an item on a map. Its sprite loads in the background. */
  add(id: number, map: WMap, x: number, y: number, g: GroundItem): void {
    this.remove(id);
    const [px, py] = itemPosition(x, y, g.subX, g.subY);
    const position = new Vector3(px, cellHeight(map, px, py), -py);
    const item: Item = {
      itemId: g.itemId,
      amount: g.amount,
      x,
      y,
      position,
      age: 0,
      dropped: g.dropped,
      view: undefined,
    };
    this.items.set(id, item);
    // A missing item sprite shows the placeholder sprite.
    this.sprites
      .get(`sprite/item/${g.itemId}`)
      .then((s) => {
        if (s === undefined || this.items.get(id) !== item) {
          return;
        }
        item.view = new SpriteView(s);
        item.view.group.position.copy(position);
        this.group.add(item.view.group);
      })
      .catch((error: unknown) => {
        console.error(`item ${g.itemId}:`, error);
      });
  }

  has(id: number): boolean {
    return this.items.has(id);
  }

  /** What lies on the ground, and where it is drawn, while its sprite shows. */
  info(id: number): { itemId: number; amount: number; position: Vector3 } | undefined {
    const item = this.items.get(id);
    if (item?.view === undefined) {
      return undefined;
    }
    return { itemId: item.itemId, amount: item.amount, position: item.view.group.position };
  }

  /** An item's cell. */
  cell(id: number): [number, number] | undefined {
    const item = this.items.get(id);
    return item === undefined ? undefined : [item.x, item.y];
  }

  remove(id: number): void {
    const item = this.items.get(id);
    if (item?.view !== undefined) {
      this.group.remove(item.view.group);
    }
    this.items.delete(id);
  }

  /** Items whose sprites have loaded, with where they are drawn. */
  *shown(): Generator<[number, Vector3]> {
    for (const [id, item] of this.items) {
      if (item.view !== undefined) {
        yield [id, item.view.group.position];
      }
    }
  }

  update(deltaMs: number): void {
    for (const item of this.items.values()) {
      item.age += deltaMs;
      const v = item.view;
      if (v === undefined) {
        continue;
      }
      if (item.dropped) {
        v.group.position.y = item.position.y + dropHeight(item.age);
        if (item.age >= DROP_MS) {
          item.dropped = false;
        }
      }
      // Item sprites have one view, whatever the camera's turn.
      v.update(deltaMs, 0);
    }
  }
}

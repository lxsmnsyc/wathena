import { Group, type Object3D, type Texture, type Vector3 } from 'three';
import type { Frame } from '../formats/wspr';
import { PIXELS_PER_CELL, type SpriteAsset, SpriteView } from './sprites';

/** Where characters get their sprites and palettes, such as a SpriteCache. */
export interface SpriteSource {
  get(name: string, placeholder?: boolean): Promise<SpriteAsset | undefined>;
  palette(name: string): Promise<Texture | undefined>;
}

// A player character drawn from separate sprites: body, head, headgear,
// weapon, shield, and garment. Heads and headgear attach to the body at
// the body frame's anchor. The order the parts are drawn in depends on
// whether the character faces the camera. A companion and a cart are
// drawn apart from the body:
// - The companion sits half a cell to the right of the body and up.
// - The cart sits one cell behind the body, in its facing direction.
// Both play their own idle or walk action and face the body's direction.

/** Asset names for a character's parts. Only body is required. */
export interface Look {
  body: string;
  head?: string;
  /** Headgear by slot. */
  headgearTop?: string;
  headgearMid?: string;
  headgearBottom?: string;
  weapon?: string;
  shield?: string;
  garment?: string;
  bodyPalette?: string;
  hairPalette?: string;
  /** A companion drawn beside the character, such as a falcon. */
  companion?: string;
  /** A cart drawn behind the character. */
  cart?: string;
}

/** RO actions whose frames are head turns instead of an animation. */
const IDLE = 0;
const WALK = 1;
const SIT = 2;

/** Where the companion is drawn from the body, in pixels on screen. */
const COMPANION_OFFSET: [number, number] = [PIXELS_PER_CELL / 2, -PIXELS_PER_CELL / 2];

/** Cell steps by facing, 0 south, turning clockwise. North is +y, and +y is -z in the scene. */
const FACING_STEPS: [number, number][] = [
  [0, -1],
  [-1, -1],
  [-1, 0],
  [-1, 1],
  [0, 1],
  [1, 1],
  [1, 0],
  [1, -1],
];

type Part =
  | 'body'
  | 'head'
  | 'headgearTop'
  | 'headgearMid'
  | 'headgearBottom'
  | 'weapon'
  | 'shield'
  | 'garment';

/**
 * Parts in draw order, first to last, for a direction the character is
 * shown facing, following roBrowser's EntityRender. The shield goes behind
 * the body when facing west through north-east. The garment goes behind
 * the body unless facing north-west through north-east, when it covers it.
 */
function drawOrder(dir: number): Part[] {
  const behind = dir >= 2 && dir <= 5;
  const away = dir >= 3 && dir <= 5;
  const order: Part[] = [];
  if (behind) {
    order.push('shield');
  }
  if (!away) {
    order.push('garment');
  }
  order.push('body', 'head', 'headgearMid', 'headgearTop', 'weapon', 'headgearBottom');
  if (away) {
    order.push('garment');
  }
  if (!behind) {
    order.push('shield');
  }
  return order;
}

const ATTACHED: ReadonlySet<Part> = new Set([
  'head',
  'headgearTop',
  'headgearMid',
  'headgearBottom',
]);

/** How far apart parts sit toward the camera, in cells, so they sort in order. */
const LAYER_GAP = 0.002;

export class Character {
  readonly group = new Group();
  /** RO action number. */
  action = 0;
  /** Facing, 0 to 7: 0 is south, turning clockwise. */
  direction = 0;
  /** Head turn while idle or sitting: 0 ahead, 1 and 2 to the sides. */
  headDirection = 0;
  private time = 0;

  private constructor(
    private readonly body: SpriteView,
    private readonly parts: Map<Part, SpriteView[]>,
    private readonly companion: SpriteView | undefined,
    private readonly cart: SpriteView | undefined,
  ) {
    for (const v of this.views()) {
      this.group.add(v.group);
    }
  }

  /** Every sprite the character draws. */
  private *views(): Generator<SpriteView> {
    for (const views of this.parts.values()) {
      yield* views;
    }
    if (this.companion !== undefined) {
      yield this.companion;
    }
    if (this.cart !== undefined) {
      yield this.cart;
    }
  }

  static async load(sprites: SpriteSource, look: Look): Promise<Character | undefined> {
    const optional = async (
      name: string | undefined,
      placeholder = false,
    ): Promise<SpriteView[]> => {
      const asset = name === undefined ? undefined : await sprites.get(name, placeholder);
      return asset === undefined ? [] : [new SpriteView(asset)];
    };
    const palette = async (name: string | undefined): Promise<Texture | undefined> =>
      name === undefined ? undefined : sprites.palette(name);
    const [
      bodyAsset,
      head,
      top,
      mid,
      bottom,
      weapon,
      shield,
      garment,
      bodyPalette,
      hairPalette,
      companion,
      cart,
    ] = await Promise.all([
      sprites.get(look.body),
      optional(look.head),
      optional(look.headgearTop),
      optional(look.headgearMid),
      optional(look.headgearBottom),
      optional(look.weapon),
      optional(look.shield),
      optional(look.garment),
      palette(look.bodyPalette),
      palette(look.hairPalette),
      // Companions and carts are not named by the importer yet, so a
      // placeholder build shows the placeholder sprite.
      optional(look.companion, true),
      optional(look.cart, true),
    ]);
    if (bodyAsset === undefined) {
      return undefined;
    }
    const body = new SpriteView(bodyAsset);
    body.setPalette(bodyPalette);
    for (const h of head) {
      h.setPalette(hairPalette);
    }
    const parts = new Map<Part, SpriteView[]>([
      ['body', [body]],
      ['head', head],
      ['headgearTop', top],
      ['headgearMid', mid],
      ['headgearBottom', bottom],
      ['weapon', weapon],
      ['shield', shield],
      ['garment', garment],
    ]);
    return new Character(body, parts, companion[0], cart[0]);
  }

  /** Whether an object is one of the character's own sprites, not something added to its group. */
  isPart(o: Object3D): boolean {
    for (const v of this.views()) {
      if (v.group === o) {
        return true;
      }
    }
    return false;
  }

  /** Multiplies every part's colors and opacity, as SpriteView.setTint. */
  setTint(r: number, g: number, b: number, a: number): void {
    for (const v of this.views()) {
      v.setTint(r, g, b, a);
    }
  }

  /** Called when the body starts a frame with a sound event. */
  get onSound(): ((name: string) => void) | undefined {
    return this.body.onSound;
  }

  set onSound(f: ((name: string) => void) | undefined) {
    this.body.onSound = f;
  }

  /** Changes the action and starts it from its first frame. */
  play(action: number): void {
    if (action !== this.action) {
      this.action = action;
      this.time = 0;
    }
  }

  /**
   * Draws the current frame. cameraSteps is the camera's turn in eighths,
   * and towardCamera a unit vector from the character to the camera.
   */
  update(deltaMs: number, cameraSteps: number, towardCamera: Vector3): void {
    this.time += deltaMs;
    const actionIndex = this.body.actionIndex(this.action, this.direction, cameraSteps);
    const a = this.body.asset.sprite.actions[actionIndex];
    if (a === undefined || a.frames.length === 0) {
      return;
    }
    let frame: number;
    if (this.action === IDLE || this.action === SIT) {
      frame = Math.min(this.headDirection, a.frames.length - 1);
    } else {
      frame = Math.floor(this.time / (a.intervalMs > 0 ? a.intervalMs : 100));
    }
    const bodyFrame = this.body.draw(actionIndex, frame, 0, 0);
    const anchor = bodyFrame?.anchors[0];

    let depth = 0;
    for (const part of drawOrder(actionIndex % 8)) {
      for (const v of this.parts.get(part) ?? []) {
        v.group.position.copy(towardCamera).multiplyScalar(depth++ * LAYER_GAP);
        if (v === this.body) {
          continue;
        }
        const index = v.actionIndex(this.action, this.direction, cameraSteps);
        const frames = v.asset.sprite.actions[index]?.frames ?? [];
        const f = frames[((frame % frames.length) + frames.length) % frames.length];
        // Heads and headgear attach at the body's anchor.
        const [dx, dy] = attach(anchor, ATTACHED.has(part) ? f : undefined);
        v.draw(index, frame, dx, dy);
      }
    }
    // The companion is drawn in front of the body's parts.
    this.companion?.group.position.copy(towardCamera).multiplyScalar(depth * LAYER_GAP);
    this.drawFollower(this.companion, cameraSteps, COMPANION_OFFSET);
    if (this.cart !== undefined) {
      const [sx, sy] = FACING_STEPS[this.direction & 7]!;
      const length = Math.hypot(sx, sy);
      this.cart.group.position.set(-sx / length, 0, sy / length);
      this.drawFollower(this.cart, cameraSteps, [0, 0]);
    }
  }

  /** Draws a companion or cart with the body's facing, walking or standing. */
  private drawFollower(v: SpriteView | undefined, cameraSteps: number, at: [number, number]): void {
    if (v === undefined) {
      return;
    }
    const action = this.action === WALK ? WALK : IDLE;
    const index = v.actionIndex(action, this.direction, cameraSteps);
    const a = v.asset.sprite.actions[index];
    const interval = a !== undefined && a.intervalMs > 0 ? a.intervalMs : 100;
    v.draw(index, Math.floor(this.time / interval), at[0], at[1]);
  }
}

/** The move that puts a part's anchor on the body's anchor. */
function attach(body: [number, number] | undefined, part: Frame | undefined): [number, number] {
  const p = part?.anchors[0];
  if (body === undefined || p === undefined) {
    return [0, 0];
  }
  return [body[0] - p[0], body[1] - p[1]];
}

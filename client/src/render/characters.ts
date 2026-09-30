import { Group, type Texture, type Vector3 } from 'three';
import type { Frame } from '../formats/wspr';
import { type SpriteAsset, SpriteView } from './sprites';

/** Where characters get their sprites and palettes, such as a SpriteCache. */
export interface SpriteSource {
  get(name: string, placeholder?: boolean): Promise<SpriteAsset | undefined>;
  palette(name: string): Promise<Texture | undefined>;
}

// A player character drawn from separate sprites: body, head, headgear,
// weapon, shield, and garment. Heads and headgear attach to the body at
// the body frame's anchor. The order the parts are drawn in depends on
// whether the character faces the camera.

/** Asset names for a character's parts. Only body is required. */
export interface Look {
  body: string;
  head?: string;
  /** Headgear from the bottom slot to the top. */
  headgear?: string[];
  weapon?: string;
  shield?: string;
  garment?: string;
  bodyPalette?: string;
  hairPalette?: string;
}

/** RO actions whose frames are head turns instead of an animation. */
const IDLE = 0;
const SIT = 2;

/**
 * Parts in a character's draw order, first to last. Facing away, the
 * shield goes behind the body and the garment in front of it.
 */
type Part = 'body' | 'head' | 'headgear' | 'weapon' | 'shield' | 'garment';
const FRONT: Part[] = ['garment', 'body', 'head', 'headgear', 'weapon', 'shield'];
const BACK: Part[] = ['shield', 'body', 'head', 'headgear', 'weapon', 'garment'];

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
  ) {
    for (const views of parts.values()) {
      for (const v of views) {
        this.group.add(v.group);
      }
    }
  }

  static async load(sprites: SpriteSource, look: Look): Promise<Character | undefined> {
    const optional = async (name: string | undefined): Promise<SpriteView[]> => {
      const asset = name === undefined ? undefined : await sprites.get(name, false);
      return asset === undefined ? [] : [new SpriteView(asset)];
    };
    const palette = async (name: string | undefined): Promise<Texture | undefined> =>
      name === undefined ? undefined : sprites.palette(name);
    const [bodyAsset, head, headgear, weapon, shield, garment, bodyPalette, hairPalette] =
      await Promise.all([
        sprites.get(look.body),
        optional(look.head),
        Promise.all((look.headgear ?? []).map(optional)).then((v) => v.flat()),
        optional(look.weapon),
        optional(look.shield),
        optional(look.garment),
        palette(look.bodyPalette),
        palette(look.hairPalette),
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
      ['headgear', headgear],
      ['weapon', weapon],
      ['shield', shield],
      ['garment', garment],
    ]);
    return new Character(body, parts);
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

    const dir = actionIndex % 8;
    const order = dir >= 2 && dir <= 5 ? BACK : FRONT;
    let depth = 0;
    for (const part of order) {
      for (const v of this.parts.get(part) ?? []) {
        v.group.position.copy(towardCamera).multiplyScalar(depth++ * LAYER_GAP);
        if (v === this.body) {
          continue;
        }
        const index = v.actionIndex(this.action, this.direction, cameraSteps);
        const frames = v.asset.sprite.actions[index]?.frames ?? [];
        const f = frames[((frame % frames.length) + frames.length) % frames.length];
        // Heads and headgear attach at the body's anchor.
        const [dx, dy] = attach(anchor, part === 'head' || part === 'headgear' ? f : undefined);
        v.draw(index, frame, dx, dy);
      }
    }
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

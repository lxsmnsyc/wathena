import {
  DataTexture,
  Group,
  Mesh,
  NearestFilter,
  NoColorSpace,
  PlaneGeometry,
  RGBAFormat,
  RedFormat,
  ShaderMaterial,
  type Texture,
  Vector2,
  Vector4,
} from 'three';
import type { AssetStore } from '../assets/store';
import decodeWPAL from '../formats/wpal';
import { type Frame, INDEXED, type Sprite, decodeWSPR } from '../formats/wspr';

// Draws WSPR sprites as quads that face the camera. Indexed pages stay as
// palette indexes on the GPU, and the shader looks colors up in a palette
// texture, so dyes only swap that texture.

/** Sprite pixels per cell, as RO draws them. */
export const PIXELS_PER_CELL = 35;

const VERTEX = /* glsl */ `
uniform vec4 uvRect;
uniform vec2 pageSize;
uniform vec2 offset;
uniform vec2 size;
uniform float angle;
uniform float mirror;
varying vec2 vUv;

void main() {
  vec2 c = position.xy;
  vec2 t = vec2(c.x + 0.5, 0.5 - c.y);
  if (mirror > 0.5) t.x = 1.0 - t.x;
  vUv = (uvRect.xy + t * uvRect.zw) / pageSize;
  vec2 p = c * size;
  // Clockwise, with Y up.
  float s = sin(angle);
  float k = cos(angle);
  p = vec2(p.x * k + p.y * s, p.y * k - p.x * s);
  p += vec2(offset.x, -offset.y);
  vec4 view = modelViewMatrix * vec4(0.0, 0.0, 0.0, 1.0);
  view.xy += p / ${PIXELS_PER_CELL.toFixed(1)};
  vec4 clip = projectionMatrix * view;
  // Test depth as if the sprite stood one cell closer to the camera, so its
  // lower half does not sink into the ground behind it.
  vec4 near = projectionMatrix * vec4(view.xy, view.z + 1.0, 1.0);
  gl_Position = vec4(clip.xy, near.z / near.w * clip.w, clip.w);
}
`;

const FRAGMENT = /* glsl */ `
uniform sampler2D page;
uniform sampler2D palette;
uniform float indexed;
uniform vec4 color;
uniform vec4 tint;
varying vec2 vUv;

void main() {
  vec4 c;
  if (indexed > 0.5) {
    float i = floor(texture2D(page, vUv).r * 255.0 + 0.5);
    // Index 0 is transparent.
    if (i < 0.5) discard;
    c = texture2D(palette, vec2((i + 0.5) / 256.0, 0.5));
  } else {
    c = texture2D(page, vUv);
  }
  c *= color * tint;
  if (c.a < 0.004) discard;
  gl_FragColor = c;
}
`;

const quad = new PlaneGeometry(1, 1);

function dataTexture(
  pixels: Uint8Array,
  width: number,
  height: number,
  indexed: boolean,
): DataTexture {
  const t = new DataTexture(pixels, width, height, indexed ? RedFormat : RGBAFormat);
  t.colorSpace = NoColorSpace;
  t.magFilter = NearestFilter;
  t.minFilter = NearestFilter;
  t.generateMipmaps = false;
  t.unpackAlignment = 1;
  t.needsUpdate = true;
  return t;
}

/** A decoded sprite with its pages uploaded as textures. */
export class SpriteAsset {
  readonly pages: DataTexture[];
  readonly palette: DataTexture;

  constructor(readonly sprite: Sprite) {
    this.pages = sprite.pages.map((p) =>
      dataTexture(p.pixels, p.width, p.height, p.kind === INDEXED),
    );
    this.palette = dataTexture(sprite.palette ?? new Uint8Array(1024), 256, 1, false);
  }
}

/** Sprites and palettes by asset name. */
export class SpriteCache {
  private readonly sprites = new Map<string, Promise<SpriteAsset | undefined>>();
  private readonly palettes = new Map<string, Promise<Texture | undefined>>();

  constructor(private readonly store: AssetStore) {}

  /**
   * A sprite. A missing sprite uses placeholder/sprite, unless placeholder
   * is false: missing equipment draws nothing.
   */
  async get(name: string, placeholder = true): Promise<SpriteAsset | undefined> {
    const s = await this.load(name);
    if (s === undefined && placeholder && name !== 'placeholder/sprite') {
      return this.load('placeholder/sprite');
    }
    return s;
  }

  /** A palette as a 256 × 1 texture. A missing palette is undefined, and the sprite keeps its own. */
  async palette(name: string): Promise<Texture | undefined> {
    let p = this.palettes.get(name);
    if (p === undefined) {
      p = this.decodePalette(name);
      this.palettes.set(name, p);
    }
    return p;
  }

  private async decodePalette(name: string): Promise<Texture | undefined> {
    try {
      const data = await this.store.bytes(name, 'palette');
      return data === undefined ? undefined : dataTexture(decodeWPAL(data), 256, 1, false);
    } catch (error) {
      console.error(`palette ${name}:`, error);
      return undefined;
    }
  }

  private async load(name: string): Promise<SpriteAsset | undefined> {
    let s = this.sprites.get(name);
    if (s === undefined) {
      s = this.decode(name);
      this.sprites.set(name, s);
    }
    return s;
  }

  private async decode(name: string): Promise<SpriteAsset | undefined> {
    try {
      const data = await this.store.bytes(name, 'sprite');
      return data === undefined ? undefined : new SpriteAsset(decodeWSPR(data));
    } catch (error) {
      console.error(`sprite ${name}:`, error);
      return undefined;
    }
  }
}

interface LayerUniforms {
  uvRect: { value: Vector4 };
  pageSize: { value: Vector2 };
  offset: { value: Vector2 };
  size: { value: Vector2 };
  angle: { value: number };
  mirror: { value: number };
  page: { value: Texture };
  palette: { value: Texture };
  indexed: { value: number };
  color: { value: Vector4 };
}

/**
 * One drawn sprite. Set its position, action, and direction, and call
 * update every frame.
 */
export class SpriteView {
  readonly group = new Group();
  /** RO action number, such as 0 for idle and 1 for walking. */
  action = 0;
  /** Facing, 0 to 7: 0 is south, turning clockwise. */
  direction = 0;
  /**
   * An action index to show as it is, without a facing, for sprites
   * whose actions are not by direction, such as the emotion icons.
   */
  fixed: number | undefined;
  private time = 0;
  private shown = '';
  private palette: Texture;
  /**
   * Called when a frame with a sound event starts, with the event's
   * sound name, such as sound/effect/hit_sword.
   */
  onSound: ((name: string) => void) | undefined;
  private readonly layers: { mesh: Mesh; u: LayerUniforms }[] = [];
  /** A color and opacity that multiply every layer, such as for a status. */
  private readonly tint = new Vector4(1, 1, 1, 1);

  constructor(readonly asset: SpriteAsset) {
    this.palette = asset.palette;
  }

  /** Changes the action and starts it from its first frame. */
  play(action: number): void {
    if (action !== this.action) {
      this.action = action;
      this.time = 0;
    }
  }

  /**
   * Advances the animation. cameraSteps is the camera's turn from facing
   * north, in eighths of a turn counterclockwise.
   */
  update(deltaMs: number, cameraSteps: number): void {
    this.time += deltaMs;
    const index = this.fixed ?? this.actionIndex(this.action, this.direction, cameraSteps);
    const a = this.asset.sprite.actions[index];
    if (a === undefined || a.frames.length === 0) {
      this.hide(0);
      return;
    }
    const interval = a.intervalMs > 0 ? a.intervalMs : 100;
    this.draw(index, Math.floor(this.time / interval), 0, 0);
  }

  /** How long an action index plays once, in milliseconds. */
  duration(index: number): number {
    const a = this.asset.sprite.actions[index];
    if (a === undefined) {
      return 0;
    }
    return a.frames.length * (a.intervalMs > 0 ? a.intervalMs : 100);
  }

  /**
   * The action to show for an RO action number and facing, seen by a
   * camera turned cameraSteps eighths. Falls back to the first action for
   * sprites that lack it.
   */
  actionIndex(action: number, direction: number, cameraSteps: number): number {
    const dir = (((direction + cameraSteps) % 8) + 8) % 8;
    const n = this.asset.sprite.actions.length;
    if (action * 8 + dir < n) {
      return action * 8 + dir;
    }
    return dir < n ? dir : 0;
  }

  /** Sets the palette for indexed pages, such as a dye. Undefined restores the sprite's own. */
  setPalette(palette: Texture | undefined): void {
    this.palette = palette ?? this.asset.palette;
    this.shown = '';
  }

  /**
   * Multiplies the sprite's colors by r, g and b, and its opacity by a,
   * all from 0 to 1. 1, 1, 1, 1 draws the sprite as it is.
   */
  setTint(r: number, g: number, b: number, a: number): void {
    // Every layer's material shares this vector.
    this.tint.set(r, g, b, a);
  }

  /**
   * Draws a frame of an action, moved by (dx, dy) pixels with Y down.
   * The frame number wraps around the action's frames. Returns the frame,
   * or undefined if the action has none.
   */
  draw(action: number, frame: number, dx: number, dy: number): Frame | undefined {
    const s = this.asset.sprite;
    const a = s.actions[action];
    if (a === undefined || a.frames.length === 0) {
      this.hide(0);
      return undefined;
    }
    const index = ((frame % a.frames.length) + a.frames.length) % a.frames.length;
    const f = a.frames[index]!;
    const key = `${action},${index},${dx},${dy}`;
    if (key === this.shown) {
      return f;
    }
    const newFrame = !this.shown.startsWith(`${action},${index},`);
    this.shown = key;
    // Events with a slash name sounds. Others are markers, such as the
    // moment a hit lands.
    const event = f.event >= 0 ? s.events[f.event] : undefined;
    if (newFrame && event?.includes('/') === true) {
      this.onSound?.(event);
    }
    let n = 0;
    for (const l of f.layers) {
      const im = l.image >= 0 ? s.images[l.image] : undefined;
      const page = im === undefined ? undefined : this.asset.pages[im.page];
      if (im === undefined || page === undefined || im.width === 0 || im.height === 0) {
        continue;
      }
      const u = this.layer(n++).u;
      u.uvRect.value.set(im.x, im.y, im.width, im.height);
      u.pageSize.value.set(page.image.width, page.image.height);
      u.offset.value.set(l.x + dx, l.y + dy);
      u.size.value.set(im.width * l.scaleX, im.height * l.scaleY);
      u.angle.value = (l.rotation * Math.PI) / 180;
      u.mirror.value = l.mirror ? 1 : 0;
      u.page.value = page;
      u.palette.value = this.palette;
      u.indexed.value = s.pages[im.page]?.kind === INDEXED ? 1 : 0;
      u.color.value.set(l.color[0] / 255, l.color[1] / 255, l.color[2] / 255, l.color[3] / 255);
    }
    this.hide(n);
    return f;
  }

  /** Hides the sprite until the next draw. */
  clear(): void {
    this.shown = '';
    this.hide(0);
  }

  private hide(from: number): void {
    for (let i = 0; i < this.layers.length; i++) {
      this.layers[i]!.mesh.visible = i < from;
    }
  }

  private layer(i: number): { mesh: Mesh; u: LayerUniforms } {
    let l = this.layers[i];
    if (l === undefined) {
      const u: LayerUniforms = {
        uvRect: { value: new Vector4() },
        pageSize: { value: new Vector2(1, 1) },
        offset: { value: new Vector2() },
        size: { value: new Vector2() },
        angle: { value: 0 },
        mirror: { value: 0 },
        page: { value: this.asset.palette },
        palette: { value: this.asset.palette },
        indexed: { value: 0 },
        color: { value: new Vector4(1, 1, 1, 1) },
      };
      const mesh = new Mesh(
        quad,
        new ShaderMaterial({
          vertexShader: VERTEX,
          fragmentShader: FRAGMENT,
          uniforms: { ...u, tint: { value: this.tint } },
          transparent: true,
          depthWrite: false,
        }),
      );
      // The quad is placed in the shader, so its bounds say nothing.
      mesh.frustumCulled = false;
      this.group.add(mesh);
      l = { mesh, u };
      this.layers.push(l);
    }
    return l;
  }
}

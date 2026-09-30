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
import { INDEXED, type Sprite, decodeWSPR } from '../formats/wspr';

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
  c *= color;
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

/** Sprites by asset name. A missing sprite uses placeholder/sprite. */
export class SpriteCache {
  private readonly sprites = new Map<string, Promise<SpriteAsset | undefined>>();

  constructor(private readonly store: AssetStore) {}

  async get(name: string): Promise<SpriteAsset | undefined> {
    const s = await this.load(name);
    if (s === undefined && name !== 'placeholder/sprite') {
      return this.load('placeholder/sprite');
    }
    return s;
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
  private time = 0;
  private shown = -1;
  private readonly layers: { mesh: Mesh; u: LayerUniforms }[] = [];

  constructor(readonly asset: SpriteAsset) {}

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
    const s = this.asset.sprite;
    const dir = (((this.direction + cameraSteps) % 8) + 8) % 8;
    const a = s.actions[this.action * 8 + dir] ?? s.actions[dir] ?? s.actions[0];
    if (a === undefined || a.frames.length === 0) {
      this.hide(0);
      return;
    }
    const interval = a.intervalMs > 0 ? a.intervalMs : 100;
    const index = Math.floor(this.time / interval) % a.frames.length;
    const key = (this.action * 8 + dir) * 65536 + index;
    if (key === this.shown) {
      return;
    }
    this.shown = key;
    const frame = a.frames[index]!;
    let n = 0;
    for (const l of frame.layers) {
      const im = l.image >= 0 ? s.images[l.image] : undefined;
      const page = im === undefined ? undefined : this.asset.pages[im.page];
      if (im === undefined || page === undefined || im.width === 0 || im.height === 0) {
        continue;
      }
      const u = this.layer(n++).u;
      u.uvRect.value.set(im.x, im.y, im.width, im.height);
      u.pageSize.value.set(page.image.width, page.image.height);
      u.offset.value.set(l.x, l.y);
      u.size.value.set(im.width * l.scaleX, im.height * l.scaleY);
      u.angle.value = (l.rotation * Math.PI) / 180;
      u.mirror.value = l.mirror ? 1 : 0;
      u.page.value = page;
      u.palette.value = this.asset.palette;
      u.indexed.value = s.pages[im.page]?.kind === INDEXED ? 1 : 0;
      u.color.value.set(l.color[0] / 255, l.color[1] / 255, l.color[2] / 255, l.color[3] / 255);
    }
    this.hide(n);
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
          uniforms: { ...u },
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

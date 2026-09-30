import {
  AddEquation,
  type BlendingDstFactor,
  type BlendingSrcFactor,
  BufferGeometry,
  CustomBlending,
  DoubleSide,
  DstAlphaFactor,
  DstColorFactor,
  Float32BufferAttribute,
  Group,
  Mesh,
  OneFactor,
  OneMinusDstAlphaFactor,
  OneMinusDstColorFactor,
  OneMinusSrcAlphaFactor,
  OneMinusSrcColorFactor,
  ShaderMaterial,
  SrcAlphaFactor,
  SrcAlphaSaturateFactor,
  SrcColorFactor,
  type Texture,
  Vector2,
  Vector4,
  ZeroFactor,
} from 'three';
import type { AssetStore } from '../assets/store';
import decodeWEFF, { type Effect } from '../formats/weff';
import type { TextureSource, TextureUniform } from './textures';

// Plays WEFF effects: layers of textured quads facing the camera, with
// each layer's state baked per frame. Between frames, color, offset,
// angle, and corners are interpolated, as assets-v1 says.

const VERTEX = /* glsl */ `
attribute float corner;
uniform vec2 corners[4];
uniform vec2 offset;
uniform float angle;
varying vec2 vUv;

void main() {
  int i = int(corner + 0.5);
  vec2 p = corners[i];
  vUv = vec2(i == 1 || i == 2 ? 1.0 : 0.0, i >= 2 ? 1.0 : 0.0);
  // Clockwise, with Y up.
  float s = sin(angle);
  float k = cos(angle);
  p = vec2(p.x * k + p.y * s, p.y * k - p.x * s) + offset;
  vec4 view = modelViewMatrix * vec4(0.0, 0.0, 0.0, 1.0);
  view.xy += p;
  gl_Position = projectionMatrix * view;
}
`;

const FRAGMENT = /* glsl */ `
uniform sampler2D map;
uniform vec4 color;
varying vec2 vUv;

void main() {
  vec4 t = texture2D(map, vUv);
  // As in RO, texels that are fully transparent or pure black are not
  // drawn.
  if (t.a == 0.0 || t.rgb == vec3(0.0)) discard;
  gl_FragColor = t * color;
}
`;

/** Direct3D blend codes, 1 to 11. Other codes blend as one. */
const FACTORS: BlendingDstFactor[] = [
  OneFactor,
  ZeroFactor,
  OneFactor,
  SrcColorFactor,
  OneMinusSrcColorFactor,
  SrcAlphaFactor,
  OneMinusSrcAlphaFactor,
  DstAlphaFactor,
  OneMinusDstAlphaFactor,
  DstColorFactor,
  OneMinusDstColorFactor,
];

export function srcFactor(code: number): BlendingSrcFactor {
  return code === 11 ? SrcAlphaSaturateFactor : (FACTORS[code] ?? OneFactor);
}

// Source alpha saturate only works as a source factor.
export function dstFactor(code: number): BlendingDstFactor {
  return FACTORS[code] ?? OneFactor;
}

/** A quad whose corners the shader places. */
function quad(): BufferGeometry {
  const g = new BufferGeometry();
  g.setAttribute('position', new Float32BufferAttribute(new Float32Array(12), 3));
  g.setAttribute('corner', new Float32BufferAttribute([0, 1, 2, 3], 1));
  g.setIndex([0, 1, 2, 0, 2, 3]);
  return g;
}

const QUAD = quad();

/** Effects by asset name. */
export class EffectCache {
  private readonly effects = new Map<string, Promise<Effect | undefined>>();

  constructor(private readonly store: AssetStore) {}

  /** An effect, or undefined if the build lacks it: missing effects show nothing. */
  async get(name: string): Promise<Effect | undefined> {
    let e = this.effects.get(name);
    if (e === undefined) {
      e = this.decode(name);
      this.effects.set(name, e);
    }
    return e;
  }

  private async decode(name: string): Promise<Effect | undefined> {
    try {
      const data = await this.store.bytes(name, 'effect');
      return data === undefined ? undefined : decodeWEFF(data);
    } catch (error) {
      console.error(`effect ${name}:`, error);
      return undefined;
    }
  }
}

interface LayerView {
  mesh: Mesh;
  material: ShaderMaterial;
  corners: Vector2[];
  offset: Vector2;
  angle: { value: number };
  color: Vector4;
  map: { value: Texture };
}

/** One playing effect. Place its group at the effect's anchor. */
export class EffectView {
  readonly group = new Group();
  /** Loops when true. Otherwise the effect stops after its last frame. */
  loop = true;
  private time = 0;
  private readonly textures: TextureUniform[];
  private readonly layers: LayerView[] = [];

  constructor(
    readonly effect: Effect,
    textures: TextureSource,
  ) {
    this.textures = effect.textures.map((t) => textures.uniform(t));
    const first = this.textures[0];
    // Without textures, no frame can be visible.
    const count = first === undefined ? 0 : effect.layers;
    for (let l = 0; l < count; l++) {
      const corners = [new Vector2(), new Vector2(), new Vector2(), new Vector2()];
      const offset = new Vector2();
      const angle = { value: 0 };
      const color = new Vector4();
      const map = { value: first!.value };
      const material = new ShaderMaterial({
        vertexShader: VERTEX,
        fragmentShader: FRAGMENT,
        uniforms: {
          corners: { value: corners },
          offset: { value: offset },
          angle,
          color: { value: color },
          map,
        },
        // Corners run clockwise, and mirrored quads turn over.
        side: DoubleSide,
        transparent: true,
        depthWrite: false,
        blending: CustomBlending,
        blendEquation: AddEquation,
      });
      const mesh = new Mesh(QUAD, material);
      // Layers draw in order, and the quad is placed in the shader.
      mesh.renderOrder = 2;
      mesh.frustumCulled = false;
      mesh.visible = false;
      this.group.add(mesh);
      this.layers.push({ mesh, material, corners, offset, angle, color, map });
    }
  }

  /** Whether a one-shot effect has played to its end. */
  get done(): boolean {
    return !this.loop && this.time * this.effect.fps >= this.effect.frames;
  }

  update(deltaMs: number): void {
    const e = this.effect;
    this.time += deltaMs / 1000;
    let t = this.time * e.fps;
    if (this.loop) {
      t %= e.frames;
    }
    const f0 = Math.floor(t);
    const mix = t - f0;
    // The last frame does not move toward the first when looping.
    const f1 = Math.min(f0 + 1, e.frames - 1);
    for (let l = 0; l < this.layers.length; l++) {
      const v = this.layers[l]!;
      const a = l * e.frames + f0;
      if (f0 >= e.frames || e.visible[a] !== 1) {
        v.mesh.visible = false;
        continue;
      }
      // A hidden next frame has no state to move toward.
      const b = e.visible[l * e.frames + f1] === 1 ? l * e.frames + f1 : a;
      const lerp = (arr: Float32Array | Uint8Array, i: number, j: number): number =>
        arr[i]! + (arr[j]! - arr[i]!) * mix;
      v.mesh.visible = true;
      v.map.value = this.textures[e.texture[a]!]?.value ?? v.map.value;
      v.material.blendSrc = srcFactor(e.srcBlend[a]!);
      v.material.blendDst = dstFactor(e.dstBlend[a]!);
      v.color.set(
        lerp(e.color, a * 4, b * 4) / 255,
        lerp(e.color, a * 4 + 1, b * 4 + 1) / 255,
        lerp(e.color, a * 4 + 2, b * 4 + 2) / 255,
        lerp(e.color, a * 4 + 3, b * 4 + 3) / 255,
      );
      v.offset.set(lerp(e.offset, a * 2, b * 2), lerp(e.offset, a * 2 + 1, b * 2 + 1));
      v.angle.value = (lerp(e.angle, a, b) * Math.PI) / 180;
      for (let c = 0; c < 4; c++) {
        v.corners[c]!.set(
          lerp(e.corners, a * 8 + c * 2, b * 8 + c * 2),
          lerp(e.corners, a * 8 + c * 2 + 1, b * 8 + c * 2 + 1),
        );
      }
    }
  }
}

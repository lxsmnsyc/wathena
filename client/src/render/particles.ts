import {
  CustomBlending,
  DoubleSide,
  Float32BufferAttribute,
  InstancedBufferAttribute,
  InstancedBufferGeometry,
  Mesh,
  ShaderMaterial,
  Vector2,
  Vector4,
} from 'three';
import { DEPTH_TEST, ETERNAL, type Emitter } from '../formats/wmap';
import { dstFactor, srcFactor } from './effects';
import type { TextureSource } from './textures';

// Draws map particle emitters, as assets-v1 describes them in WMAP's EMIT
// section. Each emitter is one instanced draw: a quad per live particle,
// facing the camera.

const VERTEX = /* glsl */ `
attribute vec2 corner;
attribute vec3 offset;
attribute float size;
attribute float fade;
uniform vec2 scale;
varying vec2 vUv;
varying float vFade;

void main() {
  vUv = vec2(corner.x * 0.5 + 0.5, 0.5 - corner.y * 0.5);
  vFade = fade;
  vec4 view = modelViewMatrix * vec4(offset, 1.0);
  view.xy += corner * size * scale;
  gl_Position = projectionMatrix * view;
}
`;

const FRAGMENT = /* glsl */ `
uniform sampler2D map;
uniform vec4 color;
varying vec2 vUv;
varying float vFade;

void main() {
  vec4 c = texture2D(map, vUv) * color;
  // Fading dims color too, since additive blends ignore alpha.
  gl_FragColor = vec4(c.rgb * vFade, c.a * vFade);
}
`;

/** Seconds over which particles fade in and out. */
const FADE = 1;

/** Seconds of simulation before an emitter first shows, so it starts full. */
const PREWARM_STEP = 0.1;

type Random = () => number;

function between(a: number, b: number, r: Random): number {
  return a + (b - a) * r();
}

export default class ParticleEmitter {
  readonly mesh: Mesh;
  private readonly geometry: InstancedBufferGeometry;
  private readonly offsets: Float32Array;
  private readonly sizes: Float32Array;
  private readonly fades: Float32Array;
  /** Per particle: start x, y, z, direction x, y, z, age, life, size. */
  private readonly state: Float32Array;
  private count = 0;
  private nextEmit = 0;
  private time = 0;
  private static readonly STRIDE = 9;

  constructor(
    readonly emitter: Emitter,
    textures: TextureSource,
    private readonly random: Random = Math.random,
  ) {
    const n = Math.max(emitter.maxCount, 0);
    this.state = new Float32Array(n * ParticleEmitter.STRIDE);
    this.offsets = new Float32Array(n * 3);
    this.sizes = new Float32Array(n);
    this.fades = new Float32Array(n);
    const g = new InstancedBufferGeometry();
    g.setAttribute('position', new Float32BufferAttribute(new Float32Array(12), 3));
    g.setAttribute('corner', new Float32BufferAttribute([-1, -1, 1, -1, 1, 1, -1, 1], 2));
    g.setIndex([0, 1, 2, 0, 2, 3]);
    g.setAttribute('offset', new InstancedBufferAttribute(this.offsets, 3));
    g.setAttribute('size', new InstancedBufferAttribute(this.sizes, 1));
    g.setAttribute('fade', new InstancedBufferAttribute(this.fades, 1));
    g.instanceCount = 0;
    this.geometry = g;
    const [r, gr, b, a] = emitter.color;
    const material = new ShaderMaterial({
      vertexShader: VERTEX,
      fragmentShader: FRAGMENT,
      uniforms: {
        map: textures.uniform(emitter.texture),
        color: { value: new Vector4(r / 255, gr / 255, b / 255, a / 255) },
        scale: { value: new Vector2(...emitter.scale) },
      },
      side: DoubleSide,
      transparent: true,
      depthWrite: false,
      depthTest: (emitter.flags & DEPTH_TEST) !== 0,
      blending: CustomBlending,
      blendSrc: srcFactor(emitter.srcBlend),
      blendDst: dstFactor(emitter.dstBlend),
    });
    this.mesh = new Mesh(g, material);
    this.mesh.position.set(...emitter.position);
    // Particles are placed in the shader and move, so bounds say nothing.
    this.mesh.frustumCulled = false;
    this.mesh.renderOrder = 2;
    // Simulate a full lifetime first.
    for (let t = 0; t < emitter.life[1]; t += PREWARM_STEP) {
      this.update(PREWARM_STEP);
    }
  }

  /** Advances the simulation by seconds. */
  update(seconds: number): void {
    const e = this.emitter;
    const S = ParticleEmitter.STRIDE;
    const eternal = (e.flags & ETERNAL) !== 0;
    this.time += seconds;
    // Age particles and drop the dead, keeping the rest packed.
    let alive = 0;
    for (let i = 0; i < this.count; i++) {
      const p = i * S;
      const age = this.state[p + 6]! + seconds;
      if (!eternal && age >= this.state[p + 7]!) {
        continue;
      }
      if (alive !== i) {
        this.state.copyWithin(alive * S, p, p + S);
      }
      this.state[alive * S + 6] = age;
      alive++;
    }
    this.count = alive;
    // Emit at the emitter's rate, up to its maximum.
    while (this.time >= this.nextEmit && this.count < e.maxCount) {
      const p = this.count * S;
      for (let k = 0; k < 3; k++) {
        this.state[p + k] = between(-e.radius[k]!, e.radius[k]!, this.random);
        this.state[p + 3 + k] = between(e.dir1[k]!, e.dir2[k]!, this.random);
      }
      this.state[p + 6] = 0;
      this.state[p + 7] = between(e.life[0], e.life[1], this.random);
      this.state[p + 8] = between(e.size[0], e.size[1], this.random);
      this.count++;
      const rate = between(e.rate[0], e.rate[1], this.random);
      this.nextEmit =
        (this.nextEmit > 0 ? this.nextEmit : this.time) + (rate > 0 ? 1 / rate : Infinity);
    }
    // Place live particles.
    for (let i = 0; i < this.count; i++) {
      const p = i * S;
      const t = this.state[p + 6]!;
      const life = this.state[p + 7]!;
      for (let k = 0; k < 3; k++) {
        this.offsets[i * 3 + k] =
          this.state[p + k]! +
          this.state[p + 3 + k]! * e.speed * t +
          e.gravity[k]! * e.speed * t * t * 0.5;
      }
      this.sizes[i] = this.state[p + 8]!;
      const fade = Math.min(FADE, life / 2);
      this.fades[i] =
        eternal || fade <= 0 ? 1 : Math.max(0, Math.min(1, t / fade, (life - t) / fade));
    }
    this.geometry.instanceCount = this.count;
    for (const name of ['offset', 'size', 'fade']) {
      const a = this.geometry.getAttribute(name);
      a.needsUpdate = true;
    }
  }
}

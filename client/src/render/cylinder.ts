import {
  AddEquation,
  BufferGeometry,
  CustomBlending,
  DoubleSide,
  Float32BufferAttribute,
  Group,
  Mesh,
  OneFactor,
  ShaderMaterial,
  SrcAlphaFactor,
  Vector4,
} from 'three';
import type { Cylinder } from '../data/tables';
import type { TextureSource, TextureUniform } from './textures';

// Draws cylinder effect parts, as roBrowser's Renderer/Effects/Cylinder.js
// (GPL-3.0) at commit 58759ab22d1d56da7f6719ee45cb475f7e851227: a cone of
// 20 sides from a bottom circle on the ground to a top circle above it,
// with the texture wrapped once around. Warp portals are two of them.

/** Sides of the cone, as roBrowser's default totalCircleSides. */
const SIDES = 20;

const VERTEX = /* glsl */ `
attribute float side;
attribute float top;
uniform float bottomSize;
uniform float topSize;
uniform float height;
varying vec2 vUv;

void main() {
  float a = side / ${SIDES.toFixed(1)} * 6.283185307179586;
  float size = top > 0.5 ? topSize : bottomSize;
  vec3 p = vec3(sin(a) * size, top * height, cos(a) * size);
  vUv = vec2(side / ${SIDES.toFixed(1)}, 1.0 - top);
  gl_Position = projectionMatrix * modelViewMatrix * vec4(p, 1.0);
}
`;

const FRAGMENT = /* glsl */ `
uniform sampler2D map;
uniform vec4 color;
varying vec2 vUv;

void main() {
  if (color.a == 0.0) discard;
  vec4 t = texture2D(map, vUv);
  // As roBrowser, transparent and black texels are not drawn.
  if (t.a == 0.0 || (t.r < 0.01 && t.g < 0.01 && t.b < 0.01)) discard;
  gl_FragColor = t * color;
}
`;

/** The cone's sides: two triangles per side, from bottom to top. */
function cone(): BufferGeometry {
  const side: number[] = [];
  const top: number[] = [];
  for (let i = 0; i < SIDES; i++) {
    for (const [s, t] of [
      [i, 0],
      [i, 1],
      [i + 1, 0],
      [i, 1],
      [i + 1, 1],
      [i + 1, 0],
    ] as const) {
      side.push(s);
      top.push(t);
    }
  }
  const g = new BufferGeometry();
  // The shader places the vertices. position only sets the count.
  g.setAttribute('position', new Float32BufferAttribute(new Float32Array(side.length * 3), 3));
  g.setAttribute('side', new Float32BufferAttribute(side, 1));
  g.setAttribute('top', new Float32BufferAttribute(top, 1));
  return g;
}

const CONE = cone();

/** A cylinder's sizes and alpha at one moment. */
export interface CylinderState {
  bottom: number;
  top: number;
  height: number;
  alpha: number;
}

/**
 * A cylinder's shape and alpha t milliseconds into its duration, as
 * roBrowser's Cylinder.render for each animation type.
 */
export function cylinderState(c: Cylinder, t: number): CylinderState {
  const d = c.duration_ms;
  let { bottom, top, height } = c;
  switch (c.animation) {
    case 1:
      // The height grows over the first second.
      height = d > 1000 ? (Math.min(t, 1000) / 1000) * c.height : (t / d) * c.height;
      break;
    case 2:
      // The top grows over the first second.
      top = d > 1000 ? (Math.min(t, 1000) / 1000) * c.top : (t / d) * c.top;
      break;
    case 3:
      // Both circles shrink, and the height rises and falls.
      bottom = (1 - t / d) * c.bottom;
      top = (1 - t / d) * c.top;
      if (t < d / 2) {
        height = (t * c.height) / (d / 2);
      } else if (t > d / 2) {
        height = ((d - t) * c.height) / (d / 2);
      }
      break;
    case 4:
      // Both circles grow.
      bottom = Math.max((t / d) * c.bottom, 0);
      top = Math.max((t / d) * c.top, 0);
      break;
    case 5:
      height = t < d / 2 ? ((t * 2) / d) * c.height : ((d - t) * c.height) / (d / 2);
      break;
    default:
      break;
  }
  let alpha = c.alpha;
  if (c.fade) {
    if (t < d / 4) {
      alpha = (t * c.alpha) / (d / 4);
    } else if (t > d / 2 + d / 4) {
      alpha = ((d - t) * c.alpha) / (d / 4);
    }
  }
  return { bottom, top, height, alpha };
}

interface Copy {
  mesh: Mesh;
  uniforms: {
    bottomSize: { value: number };
    topSize: { value: number };
    height: { value: number };
    color: { value: Vector4 };
    map: TextureUniform;
  };
}

/**
 * One playing cylinder part, with its copies. Place its group at the
 * center of the cell it plays on.
 */
export class CylinderView {
  readonly group = new Group();
  private time = 0;
  private readonly copies: Copy[] = [];

  constructor(
    readonly part: Cylinder,
    textures: TextureSource,
  ) {
    const map = textures.uniform(part.texture);
    for (let i = 0; i < part.copies; i++) {
      const uniforms = {
        bottomSize: { value: 0 },
        topSize: { value: 0 },
        height: { value: 0 },
        color: { value: new Vector4(...part.color, 0) },
        map,
      };
      const material = new ShaderMaterial({
        vertexShader: VERTEX,
        fragmentShader: FRAGMENT,
        uniforms,
        side: DoubleSide,
        transparent: true,
        depthWrite: false,
        // roBrowser's blendMode 2: source alpha, added to the scene.
        blending: CustomBlending,
        blendEquation: AddEquation,
        blendSrc: SrcAlphaFactor,
        blendDst: OneFactor,
      });
      const mesh = new Mesh(CONE, material);
      // roBrowser draws it before entities, so sprites stay on top.
      mesh.renderOrder = 1;
      mesh.frustumCulled = false;
      mesh.visible = false;
      this.group.add(mesh);
      this.copies.push({ mesh, uniforms });
    }
  }

  /** Whether a part that does not repeat has ended. */
  get done(): boolean {
    const p = this.part;
    return !p.repeat && this.time >= (p.copies - 1) * p.copy_ms + p.duration_ms;
  }

  update(deltaMs: number): void {
    this.time += deltaMs;
    const p = this.part;
    for (let i = 0; i < this.copies.length; i++) {
      const c = this.copies[i]!;
      let t = this.time - i * p.copy_ms;
      if (t >= 0 && p.repeat) {
        t %= p.duration_ms;
      }
      if (t < 0 || t >= p.duration_ms) {
        c.mesh.visible = false;
        continue;
      }
      const s = cylinderState(p, t);
      c.mesh.visible = true;
      c.uniforms.bottomSize.value = s.bottom;
      c.uniforms.topSize.value = s.top;
      c.uniforms.height.value = s.height;
      c.uniforms.color.value.w = s.alpha;
    }
  }
}

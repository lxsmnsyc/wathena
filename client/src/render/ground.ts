import {
  BufferGeometry,
  DataTexture,
  DoubleSide,
  Float32BufferAttribute,
  Group,
  LinearFilter,
  Mesh,
  NoColorSpace,
  ShaderMaterial,
  Vector2,
} from 'three';
import type { WMap } from '../formats/wmap';
import { LIGHT_GLSL, type LightUniforms } from './light';
import type { TextureSource } from './textures';

// Builds the ground mesh as assets-v1, "Building the ground", describes.

const VERTEX = /* glsl */ `
attribute vec2 uv1;
attribute float top;
uniform vec2 groundSize;
varying vec2 vUv;
varying vec2 vLightmapUv;
varying vec2 vColorUv;
varying float vTop;
varying vec3 vNormal;

void main() {
  vUv = uv;
  vLightmapUv = uv1;
  // The tile color texture has one texel per surface, 2 × 2 cells, so
  // linear filtering blends colors between neighboring surfaces.
  vColorUv = vec2(position.x, -position.z) / (2.0 * groundSize);
  vTop = top;
  vNormal = normal;
  gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
}
`;

const FRAGMENT = /* glsl */ `
${LIGHT_GLSL}
uniform sampler2D map;
uniform sampler2D lightmap;
uniform sampler2D tileColors;
varying vec2 vUv;
varying vec2 vLightmapUv;
varying vec2 vColorUv;
varying float vTop;
varying vec3 vNormal;

void main() {
  vec4 tex = texture2D(map, vUv);
  if (tex.a < 0.5) discard;
  vec3 color = tex.rgb;
  if (vTop > 0.5) color *= texture2D(tileColors, vColorUv).rgb;
  vec4 lm = texture2D(lightmap, vLightmapUv);
  vec3 normal = gl_FrontFacing ? vNormal : -vNormal;
  color = color * roLight(normal) * lm.a + lm.rgb;
  gl_FragColor = vec4(color, 1.0);
}
`;

/** Vertex data for the tiles that use one texture. */
class Batch {
  readonly position: number[] = [];
  readonly normal: number[] = [];
  readonly uv: number[] = [];
  readonly uv1: number[] = [];
  readonly top: number[] = [];
}

export default function buildGround(m: WMap, textures: TextureSource, light: LightUniforms): Group {
  const g = m.ground;
  const t = g.tiles;
  const group = new Group();
  group.name = 'ground';
  if (g.width === 0 || g.height === 0) {
    return group;
  }

  const columns = Math.max(g.lightmapColumns, 1);
  const rows = Math.max(Math.ceil(g.lightmapCount / columns), 1);
  const atlasW = columns * 8;
  const atlasH = rows * 8;

  const batches = new Map<number, Batch>();
  const corner = (surface: number, k: number): [number, number, number] => {
    const i = surface % g.width;
    const j = Math.floor(surface / g.width);
    return [2 * i + 2 * (k & 1), g.surfaceHeights[surface * 4 + k] ?? 0, -(2 * j + 2 * (k >> 1))];
  };

  // Adds tile t on four points, given in tile corner order.
  const addTile = (tile: number, p: [number, number, number][], isTop: boolean): void => {
    const tex = t.texture[tile] ?? 0;
    let b = batches.get(tex);
    if (b === undefined) {
      b = new Batch();
      batches.set(tex, b);
    }
    const p0 = p[0]!;
    const p1 = p[1]!;
    const p3 = p[3]!;
    // The normal of the first triangle's winding. The shader flips it for
    // back faces, since walls are seen from both sides.
    const ax = p1[0] - p0[0];
    const ay = p1[1] - p0[1];
    const az = p1[2] - p0[2];
    const bx = p3[0] - p0[0];
    const by = p3[1] - p0[1];
    const bz = p3[2] - p0[2];
    const nx = ay * bz - az * by;
    const ny = az * bx - ax * bz;
    const nz = ax * by - ay * bx;
    const len = Math.hypot(nx, ny, nz) || 1;
    const block = t.lightmap[tile] ?? 0;
    const bx0 = (block % columns) * 8;
    const by0 = Math.floor(block / columns) * 8;
    // Triangles 0, 1, 3 and 0, 3, 2 face up for a top tile.
    for (const k of [0, 1, 3, 0, 3, 2]) {
      const q = p[k]!;
      b.position.push(q[0], q[1], q[2]);
      b.normal.push(nx / len, ny / len, nz / len);
      b.uv.push(t.u[tile * 4 + k] ?? 0, t.v[tile * 4 + k] ?? 0);
      b.uv1.push((bx0 + (k & 1 ? 7 : 1)) / atlasW, (by0 + (k >> 1 ? 7 : 1)) / atlasH);
      b.top.push(isTop ? 1 : 0);
    }
  };

  const colors = new Uint8Array(g.width * g.height * 4).fill(255);
  for (let s = 0; s < g.width * g.height; s++) {
    const i = s % g.width;
    const j = Math.floor(s / g.width);
    const top = g.top[s] ?? -1;
    if (top >= 0) {
      addTile(
        top,
        [0, 1, 2, 3].map((k) => corner(s, k)),
        true,
      );
      colors.set(t.color.subarray(top * 4, top * 4 + 4), s * 4);
    }
    const north = g.north[s] ?? -1;
    if (north >= 0 && j + 1 < g.height) {
      const n = s + g.width;
      addTile(north, [corner(s, 2), corner(s, 3), corner(n, 0), corner(n, 1)], false);
    }
    const east = g.east[s] ?? -1;
    if (east >= 0 && i + 1 < g.width) {
      const e = s + 1;
      addTile(east, [corner(s, 3), corner(s, 1), corner(e, 2), corner(e, 0)], false);
    }
  }

  const tileColors = new DataTexture(colors, g.width, g.height);
  tileColors.colorSpace = NoColorSpace;
  tileColors.magFilter = LinearFilter;
  tileColors.minFilter = LinearFilter;
  tileColors.needsUpdate = true;

  const lightmap = textures.uniform(g.lightmap, { repeat: false, mipmaps: false });
  const groundSize = { value: new Vector2(g.width, g.height) };
  for (const [tex, b] of batches) {
    const geo = new BufferGeometry();
    geo.setAttribute('position', new Float32BufferAttribute(b.position, 3));
    geo.setAttribute('normal', new Float32BufferAttribute(b.normal, 3));
    geo.setAttribute('uv', new Float32BufferAttribute(b.uv, 2));
    geo.setAttribute('uv1', new Float32BufferAttribute(b.uv1, 2));
    geo.setAttribute('top', new Float32BufferAttribute(b.top, 1));
    geo.computeBoundingSphere();
    const material = new ShaderMaterial({
      vertexShader: VERTEX,
      fragmentShader: FRAGMENT,
      side: DoubleSide,
      uniforms: {
        ...light,
        map: textures.uniform(g.textures[tex] ?? ''),
        lightmap,
        tileColors: { value: tileColors },
        groundSize,
      },
    });
    group.add(new Mesh(geo, material));
  }
  return group;
}

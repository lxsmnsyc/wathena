import {
  BufferGeometry,
  DoubleSide,
  Float32BufferAttribute,
  Group,
  Mesh,
  ShaderMaterial,
  type Texture,
} from 'three';
import type { WMap, WaterZone } from '../formats/wmap';
import type { TextureSource, TextureUniform } from './textures';

// Draws water as flat, animated sheets, one per water zone. Waves move
// the surface up and down with RO's wave height, speed, and pitch.

const VERTEX = /* glsl */ `
uniform float time;
uniform float waveHeight;
uniform float waveSpeed;
uniform float wavePitch;
varying vec2 vUv;

void main() {
  // One texture repeat covers a ground surface of 2 × 2 cells.
  vUv = vec2(position.x, -position.z) / 2.0;
  vec3 p = position;
  float phase = radians((p.x + p.z) * wavePitch + time * waveSpeed * 60.0);
  p.y += sin(phase) * waveHeight;
  gl_Position = projectionMatrix * modelViewMatrix * vec4(p, 1.0);
}
`;

const FRAGMENT = /* glsl */ `
uniform sampler2D map;
varying vec2 vUv;

void main() {
  // RO draws water at 144 / 255 opacity.
  gl_FragColor = vec4(texture2D(map, vUv).rgb, 0.565);
}
`;

/** Grid cells per water mesh edge, so waves have vertices to move. */
const STEP = 2;

interface Sheet {
  time: { value: number };
  map: { value: Texture };
  frames: TextureUniform[];
  animSpeed: number;
}

export default class Water {
  readonly group = new Group();
  private readonly sheets: Sheet[] = [];

  constructor(m: WMap, textures: TextureSource) {
    this.group.name = 'water';
    const w = m.water;
    if (w === undefined) {
      return;
    }
    const zones: [WaterZone, number, number, number, number][] = [];
    if (w.zones.length === 0) {
      zones.push([w, 0, 0, m.width, m.height]);
    } else {
      const zw = m.width / w.splitWidth;
      const zh = m.height / w.splitHeight;
      w.zones.forEach((z, i) => {
        const x = i % w.splitWidth;
        const y = Math.floor(i / w.splitWidth);
        zones.push([z, x * zw, y * zh, zw, zh]);
      });
    }
    for (const [z, x0, y0, zw, zh] of zones) {
      const frames = w.textures.get(z.type);
      if (frames === undefined) {
        continue;
      }
      const uniforms = frames.map((f) => textures.uniform(f));
      const sheet: Sheet = {
        time: { value: 0 },
        map: { value: uniforms[0]!.value },
        frames: uniforms,
        animSpeed: Math.max(z.animSpeed, 1),
      };
      const material = new ShaderMaterial({
        vertexShader: VERTEX,
        fragmentShader: FRAGMENT,
        side: DoubleSide,
        transparent: true,
        depthWrite: false,
        uniforms: {
          map: sheet.map,
          time: sheet.time,
          waveHeight: { value: z.waveHeight },
          waveSpeed: { value: z.waveSpeed },
          wavePitch: { value: z.wavePitch },
        },
      });
      const mesh = new Mesh(grid(x0, y0, zw, zh, z.level), material);
      mesh.renderOrder = 1;
      this.group.add(mesh);
      this.sheets.push(sheet);
    }
  }

  update(seconds: number): void {
    for (const s of this.sheets) {
      s.time.value = seconds;
      // A frame lasts animSpeed ticks of RO's 60 per second.
      const frame = Math.floor((seconds * 60) / s.animSpeed) % s.frames.length;
      s.map.value = s.frames[frame]!.value;
    }
  }
}

/** A flat grid over cells, at a height. */
function grid(x0: number, y0: number, w: number, h: number, level: number): BufferGeometry {
  const nx = Math.max(Math.ceil(w / STEP), 1);
  const ny = Math.max(Math.ceil(h / STEP), 1);
  const pos: number[] = [];
  for (let j = 0; j <= ny; j++) {
    for (let i = 0; i <= nx; i++) {
      pos.push(x0 + (w * i) / nx, level, -(y0 + (h * j) / ny));
    }
  }
  const index: number[] = [];
  for (let j = 0; j < ny; j++) {
    for (let i = 0; i < nx; i++) {
      const a = j * (nx + 1) + i;
      index.push(a, a + 1, a + nx + 2, a, a + nx + 2, a + nx + 1);
    }
  }
  const geo = new BufferGeometry();
  geo.setAttribute('position', new Float32BufferAttribute(pos, 3));
  geo.setIndex(index);
  geo.computeBoundingSphere();
  return geo;
}

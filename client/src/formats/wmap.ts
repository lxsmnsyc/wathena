// Reads WMAP maps, as defined in docs/specs/assets-v1.md, "Map (WMAP)".
//
// Map space is in cells: X points east, Y up, and -Z north. Cell (x, y)
// covers X from x to x + 1 and Z from -(y + 1) to -y.

import { FormatError, Reader } from './reader';

export const MAX_CELLS = 1024;
export const MAX_BYTES = 64 * 1024 * 1024;

export const WALKABLE = 1;
export const SHOOTABLE = 2;
export const WATER = 4;

export interface WMap {
  width: number;
  height: number;
  /** Cell flags, row by row from y = 0, the south edge. */
  cellFlags: Uint8Array;
  /** Four corner heights per cell, in cells, up positive. */
  cellHeights: Float32Array;
  ground: Ground;
  light: Light;
  water: Water | undefined;
  objects: MapObject[];
  sounds: MapSound[];
  effects: MapEffect[];
  emitters: Emitter[];
  /** The background music's asset name, or empty. */
  music: string;
  /** Every asset name the map uses. */
  deps: string[];
}

/** The ground as a grid of surfaces, each 2 × 2 cells. */
export interface Ground {
  width: number;
  height: number;
  textures: string[];
  tiles: Tiles;
  /** Four corner heights per surface. */
  surfaceHeights: Float32Array;
  /** Tile indexes per surface, or -1. */
  top: Int32Array;
  north: Int32Array;
  east: Int32Array;
  lightmap: string;
  lightmapCount: number;
  lightmapColumns: number;
}

export interface Tiles {
  count: number;
  /** Four values per tile, for the corners in surface order. */
  u: Float32Array;
  v: Float32Array;
  texture: Uint16Array;
  lightmap: Uint32Array;
  /** RGBA per tile. */
  color: Uint8Array;
}

export interface Light {
  longitude: number;
  latitude: number;
  diffuse: [number, number, number];
  ambient: [number, number, number];
  opacity: number;
  /** Unit vector toward the light, in map space. */
  direction: [number, number, number];
}

export interface WaterZone {
  level: number;
  type: number;
  waveHeight: number;
  waveSpeed: number;
  wavePitch: number;
  animSpeed: number;
}

export interface Water extends WaterZone {
  splitWidth: number;
  splitHeight: number;
  /** Zones row by row, overriding the default when the map is split. */
  zones: WaterZone[];
  /** Animation frames per water type. */
  textures: Map<number, string[]>;
}

export interface MapObject {
  model: string;
  /** Column-major, from model space to map space. */
  matrix: Float32Array;
  animType: number;
  animSpeed: number;
  blockType: number;
}

export interface MapSound {
  name: string;
  position: [number, number, number];
  volume: number;
  width: number;
  height: number;
  range: number;
  cycle: number;
}

export interface MapEffect {
  id: number;
  position: [number, number, number];
  delay: number;
  params: [number, number, number, number];
}

export const DEPTH_TEST = 1;
export const ETERNAL = 2;
export const FLAT = 4;

/**
 * A particle emitter. Distances are in cells and times in seconds. A
 * particle starts at position plus a random offset within ±radius, with
 * a direction between dir1 and dir2 on each axis, and is at start + dir ×
 * speed × t + gravity × speed × t² / 2 after t seconds.
 */
export interface Emitter {
  position: [number, number, number];
  texture: string;
  srcBlend: number;
  dstBlend: number;
  /** RGBA, 0 to 255. */
  color: [number, number, number, number];
  dir1: [number, number, number];
  dir2: [number, number, number];
  gravity: [number, number, number];
  radius: [number, number, number];
  /** Ranges: seconds, particles per second, and half the width. */
  life: [number, number];
  rate: [number, number];
  size: [number, number];
  speed: number;
  maxCount: number;
  /** DEPTH_TEST, ETERNAL, and FLAT. */
  flags: number;
  scale: [number, number];
  /** The plane of flat particles, in degrees about X, Y, and Z. */
  rotate: [number, number, number];
}

const REQUIRED = ['CELL', 'GRND', 'LMAP', 'LGHT', 'OBJS', 'SNDS', 'EFCT', 'DEPS'];

function vec3(r: Reader): [number, number, number] {
  return [r.f32(), r.f32(), r.f32()];
}

export function decodeWMap(data: Uint8Array): WMap {
  if (data.byteLength > MAX_BYTES) {
    throw new FormatError('wmap: too large');
  }
  const r = new Reader(data);
  if (r.tag() !== 'WMAP' || r.u16() !== 1) {
    throw new FormatError('wmap: header');
  }
  const width = r.u16();
  const height = r.u16();
  if (width < 1 || height < 1 || width > MAX_CELLS || height > MAX_CELLS) {
    throw new FormatError('wmap: size');
  }
  const m: WMap = {
    width,
    height,
    cellFlags: new Uint8Array(0),
    cellHeights: new Float32Array(0),
    ground: {
      width: 0,
      height: 0,
      textures: [],
      tiles: {
        count: 0,
        u: new Float32Array(0),
        v: new Float32Array(0),
        texture: new Uint16Array(0),
        lightmap: new Uint32Array(0),
        color: new Uint8Array(0),
      },
      surfaceHeights: new Float32Array(0),
      top: new Int32Array(0),
      north: new Int32Array(0),
      east: new Int32Array(0),
      lightmap: '',
      lightmapCount: 0,
      lightmapColumns: 0,
    },
    light: {
      longitude: 0,
      latitude: 0,
      diffuse: [1, 1, 1],
      ambient: [0, 0, 0],
      opacity: 1,
      direction: [0, 1, 0],
    },
    water: undefined,
    objects: [],
    sounds: [],
    effects: [],
    emitters: [],
    music: '',
    deps: [],
  };
  const seen = new Set<string>();
  while (!r.done()) {
    const tag = r.tag();
    const s = new Reader(r.take(r.u32()));
    if (seen.has(tag)) {
      throw new FormatError(`wmap: repeated section ${tag}`);
    }
    seen.add(tag);
    switch (tag) {
      case 'CELL':
        readCells(m, s);
        break;
      case 'GRND':
        readGround(m.ground, s);
        break;
      case 'LMAP':
        m.ground.lightmap = s.str();
        m.ground.lightmapCount = s.u32();
        m.ground.lightmapColumns = s.u16();
        break;
      case 'LGHT':
        m.light = {
          longitude: s.i32(),
          latitude: s.i32(),
          diffuse: vec3(s),
          ambient: vec3(s),
          opacity: s.f32(),
          direction: vec3(s),
        };
        break;
      case 'WATR':
        m.water = readWater(s);
        break;
      case 'OBJS':
        for (let i = s.count(77); i > 0; i--) {
          m.objects.push({
            model: s.str(),
            matrix: s.f32s(16),
            animType: s.i32(),
            animSpeed: s.f32(),
            blockType: s.i32(),
          });
        }
        break;
      case 'SNDS':
        for (let i = s.count(33); i > 0; i--) {
          m.sounds.push({
            name: s.str(),
            position: vec3(s),
            volume: s.f32(),
            width: s.f32(),
            height: s.f32(),
            range: s.f32(),
            cycle: s.f32(),
          });
        }
        break;
      case 'EFCT':
        for (let i = s.count(36); i > 0; i--) {
          m.effects.push({
            id: s.i32(),
            position: vec3(s),
            delay: s.f32(),
            params: [s.f32(), s.f32(), s.f32(), s.f32()],
          });
        }
        break;
      case 'MUSC':
        m.music = s.str();
        break;
      case 'EMIT':
        for (let i = s.count(130); i > 0; i--) {
          const position = vec3(s);
          const texture = s.str();
          const srcBlend = s.u8();
          const dstBlend = s.u8();
          const c = s.take(4);
          m.emitters.push({
            position,
            texture,
            srcBlend,
            dstBlend,
            color: [c[0]!, c[1]!, c[2]!, c[3]!],
            dir1: vec3(s),
            dir2: vec3(s),
            gravity: vec3(s),
            radius: vec3(s),
            life: [s.f32(), s.f32()],
            rate: [s.f32(), s.f32()],
            size: [s.f32(), s.f32()],
            speed: s.f32(),
            maxCount: s.u16(),
            flags: s.u8(),
            scale: [s.f32(), s.f32()],
            rotate: vec3(s),
          });
        }
        break;
      case 'DEPS':
        for (let i = s.count(1); i > 0; i--) {
          m.deps.push(s.str());
        }
        break;
      default:
        // Readers skip sections they do not know.
        break;
    }
  }
  for (const tag of REQUIRED) {
    if (!seen.has(tag)) {
      throw new FormatError(`wmap: no ${tag} section`);
    }
  }
  validate(m);
  return m;
}

function readCells(m: WMap, s: Reader): void {
  const n = m.width * m.height;
  if (s.remaining() !== n * 17) {
    throw new FormatError('wmap: cell section');
  }
  m.cellFlags = new Uint8Array(n);
  m.cellHeights = new Float32Array(n * 4);
  for (let i = 0; i < n; i++) {
    m.cellFlags[i] = s.u8();
    for (let k = 0; k < 4; k++) {
      m.cellHeights[i * 4 + k] = s.f32();
    }
  }
}

function readGround(g: Ground, s: Reader): void {
  g.width = s.u16();
  g.height = s.u16();
  for (let i = s.u16(); i > 0; i--) {
    g.textures.push(s.str());
  }
  const n = s.count(42);
  const t: Tiles = {
    count: n,
    u: new Float32Array(n * 4),
    v: new Float32Array(n * 4),
    texture: new Uint16Array(n),
    lightmap: new Uint32Array(n),
    color: new Uint8Array(n * 4),
  };
  for (let i = 0; i < n; i++) {
    for (let k = 0; k < 4; k++) {
      t.u[i * 4 + k] = s.f32();
    }
    for (let k = 0; k < 4; k++) {
      t.v[i * 4 + k] = s.f32();
    }
    t.texture[i] = s.u16();
    t.lightmap[i] = s.u32();
    t.color.set(s.take(4), i * 4);
  }
  g.tiles = t;
  // The surfaces fill the rest of the section, which bounds the
  // allocations below.
  const count = g.width * g.height;
  if (g.width > MAX_CELLS || g.height > MAX_CELLS || count * 28 !== s.remaining()) {
    throw new FormatError('wmap: ground section');
  }
  g.surfaceHeights = new Float32Array(count * 4);
  g.top = new Int32Array(count);
  g.north = new Int32Array(count);
  g.east = new Int32Array(count);
  for (let i = 0; i < count; i++) {
    for (let k = 0; k < 4; k++) {
      g.surfaceHeights[i * 4 + k] = s.f32();
    }
    g.top[i] = s.i32();
    g.north[i] = s.i32();
    g.east[i] = s.i32();
  }
}

function readZone(s: Reader): WaterZone {
  return {
    level: s.f32(),
    type: s.i32(),
    waveHeight: s.f32(),
    waveSpeed: s.f32(),
    wavePitch: s.f32(),
    animSpeed: s.i32(),
  };
}

function readWater(s: Reader): Water {
  const zone = readZone(s);
  const splitWidth = s.u16();
  const splitHeight = s.u16();
  if (splitWidth * splitHeight * 24 > s.remaining()) {
    throw new FormatError('wmap: water zones');
  }
  const zones: WaterZone[] = [];
  for (let i = splitWidth * splitHeight; i > 0; i--) {
    zones.push(readZone(s));
  }
  const textures = new Map<number, string[]>();
  for (let i = s.u8(); i > 0; i--) {
    const type = s.i32();
    const frames: string[] = [];
    for (let f = 0; f < 32; f++) {
      frames.push(s.str());
    }
    textures.set(type, frames);
  }
  return { ...zone, splitWidth, splitHeight, zones, textures };
}

function validate(m: WMap): void {
  for (const e of m.emitters) {
    if (e.maxCount > 1000) {
      throw new FormatError('wmap: emitter particles');
    }
  }
  const g = m.ground;
  const t = g.tiles;
  for (let i = 0; i < t.count; i++) {
    if ((t.texture[i] ?? 0) >= g.textures.length) {
      throw new FormatError('wmap: tile texture');
    }
  }
  for (const refs of [g.top, g.north, g.east]) {
    for (const v of refs) {
      if (v < -1 || v >= t.count) {
        throw new FormatError('wmap: surface tile');
      }
    }
  }
}

/** Whether a cell, in server coordinates, is walkable. */
export function walkable(m: WMap, x: number, y: number): boolean {
  if (x < 0 || y < 0 || x >= m.width || y >= m.height) {
    return false;
  }
  return ((m.cellFlags[y * m.width + x] ?? 0) & WALKABLE) !== 0;
}

/**
 * The ground height at a point, in cells, from the cell corner heights.
 * x and y are server coordinates, so the point is (x, height, -y) in map
 * space.
 */
export function cellHeight(m: WMap, x: number, y: number): number {
  const cx = Math.min(Math.max(Math.floor(x), 0), m.width - 1);
  const cy = Math.min(Math.max(Math.floor(y), 0), m.height - 1);
  const fx = Math.min(Math.max(x - cx, 0), 1);
  const fy = Math.min(Math.max(y - cy, 0), 1);
  const h = m.cellHeights;
  const i = (cy * m.width + cx) * 4;
  const south = (h[i] ?? 0) * (1 - fx) + (h[i + 1] ?? 0) * fx;
  const north = (h[i + 2] ?? 0) * (1 - fx) + (h[i + 3] ?? 0) * fx;
  return south * (1 - fy) + north * fy;
}

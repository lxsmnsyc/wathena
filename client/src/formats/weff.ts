// Reads WEFF effects, as defined in docs/specs/assets-v1.md, "Effect
// (WEFF)".

import { FormatError, Reader } from './reader';

export const MAX_BYTES = 8 * 1024 * 1024;
export const MAX_FRAMES = 1 << 14;
export const MAX_FRAME_RECORDS = 1 << 18;

const FRAME_SIZE = 53;

/**
 * A baked effect. Layer states are stored per record, layer × frames +
 * frame, so the arrays below hold one entry (or 2, 4, or 8 values) per
 * record.
 */
export interface Effect {
  fps: number;
  frames: number;
  textures: string[];
  layers: number;
  visible: Uint8Array;
  texture: Uint16Array;
  srcBlend: Uint8Array;
  dstBlend: Uint8Array;
  /** RGBA, 4 per record. */
  color: Uint8Array;
  /** The quad's center in cells, X right and Y up, 2 per record. */
  offset: Float32Array;
  /** Degrees, clockwise. */
  angle: Float32Array;
  /** Top-left, top-right, bottom-right, bottom-left, relative to the offset, 8 per record. */
  corners: Float32Array;
}

export default function decodeWEFF(data: Uint8Array): Effect {
  if (data.byteLength > MAX_BYTES) {
    throw new FormatError('weff: too large');
  }
  const r = new Reader(data);
  if (r.tag() !== 'WEFF' || r.u16() !== 1) {
    throw new FormatError('weff: header');
  }
  const fps = r.u16();
  const frames = r.u32();
  if (fps < 1 || frames < 1 || frames > MAX_FRAMES) {
    throw new FormatError('weff: frames');
  }
  const textures: string[] = [];
  for (let i = r.u16(); i > 0; i--) {
    textures.push(r.str());
  }
  const layers = r.u16();
  const n = layers * frames;
  if (n > MAX_FRAME_RECORDS || n * FRAME_SIZE !== r.remaining()) {
    throw new FormatError('weff: frame data');
  }
  const e: Effect = {
    fps,
    frames,
    textures,
    layers,
    visible: new Uint8Array(n),
    texture: new Uint16Array(n),
    srcBlend: new Uint8Array(n),
    dstBlend: new Uint8Array(n),
    color: new Uint8Array(n * 4),
    offset: new Float32Array(n * 2),
    angle: new Float32Array(n),
    corners: new Float32Array(n * 8),
  };
  for (let i = 0; i < n; i++) {
    const visible = r.u8() === 1;
    const texture = r.u16();
    const src = r.u8();
    const dst = r.u8();
    const color = r.take(4);
    const ox = r.f32();
    const oy = r.f32();
    const angle = r.f32();
    const corners = r.f32s(8);
    // Hidden frames carry no state.
    if (!visible) {
      continue;
    }
    if (texture >= textures.length) {
      throw new FormatError('weff: texture');
    }
    e.visible[i] = 1;
    e.texture[i] = texture;
    e.srcBlend[i] = src;
    e.dstBlend[i] = dst;
    e.color.set(color, i * 4);
    e.offset[i * 2] = ox;
    e.offset[i * 2 + 1] = oy;
    e.angle[i] = angle;
    e.corners.set(corners, i * 8);
  }
  return e;
}

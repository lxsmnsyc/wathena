// Reads WSPR sprites, as defined in docs/specs/assets-v1.md, "Sprite
// (WSPR)".

import { FormatError, Reader } from './reader';

export const MAX_BYTES = 32 * 1024 * 1024;
export const MAX_PAGES = 64;
export const MAX_PAGE_SIZE = 2048;

export const INDEXED = 0;
export const RGBA = 1;

export interface Page {
  kind: typeof INDEXED | typeof RGBA;
  width: number;
  height: number;
  /** One byte per pixel for indexed pages, four for RGBA pages. */
  pixels: Uint8Array;
}

export interface SpriteImage {
  page: number;
  x: number;
  y: number;
  width: number;
  height: number;
}

export interface Layer {
  /** Index into images, or -1 for none. */
  image: number;
  /** The image's center, in pixels from the anchor, with Y down. */
  x: number;
  y: number;
  mirror: boolean;
  /** RGBA multiplier, 0 to 255. */
  color: [number, number, number, number];
  scaleX: number;
  scaleY: number;
  /** Degrees, clockwise. */
  rotation: number;
}

export interface Frame {
  /** Index into events, or -1. */
  event: number;
  anchors: [number, number][];
  layers: Layer[];
}

export interface Action {
  intervalMs: number;
  frames: Frame[];
}

export interface Sprite {
  pages: Page[];
  /** 256 RGBA colors, present when a page is indexed. Index 0 is transparent. */
  palette: Uint8Array | undefined;
  images: SpriteImage[];
  /** RO action number × 8 + direction. */
  actions: Action[];
  events: string[];
}

export function decodeWSPR(data: Uint8Array): Sprite {
  if (data.byteLength > MAX_BYTES) {
    throw new FormatError('wspr: too large');
  }
  const r = new Reader(data);
  if (r.tag() !== 'WSPR' || r.u16() !== 1) {
    throw new FormatError('wspr: header');
  }
  const pageCount = r.u16();
  const imageCount = r.u32();
  const actionCount = r.u32();
  const eventCount = r.u16();
  // Each image takes 10 bytes and each action at least 4.
  if (pageCount > MAX_PAGES || imageCount * 10 > r.remaining() || actionCount * 4 > r.remaining()) {
    throw new FormatError('wspr: counts do not fit the data');
  }
  const s: Sprite = { pages: [], palette: undefined, images: [], actions: [], events: [] };
  for (let i = 0; i < pageCount; i++) {
    const kind = r.u8();
    const width = r.u16();
    const height = r.u16();
    if ((kind !== INDEXED && kind !== RGBA) || width > MAX_PAGE_SIZE || height > MAX_PAGE_SIZE) {
      throw new FormatError(`wspr: page ${i}`);
    }
    const pixels = r.take(width * height * (kind === RGBA ? 4 : 1));
    s.pages.push({ kind, width, height, pixels });
  }
  if (s.pages.some((p) => p.kind === INDEXED)) {
    s.palette = r.take(1024);
  }
  for (let i = 0; i < imageCount; i++) {
    const im = { page: r.u16(), x: r.u16(), y: r.u16(), width: r.u16(), height: r.u16() };
    if (im.width > 0 && im.height > 0) {
      const p = s.pages[im.page];
      if (p === undefined || im.x + im.width > p.width || im.y + im.height > p.height) {
        throw new FormatError(`wspr: image ${i} is outside its page`);
      }
    }
    s.images.push(im);
  }
  for (let a = 0; a < actionCount; a++) {
    const intervalMs = r.u16();
    const frameCount = r.u16();
    if (frameCount * 5 > r.remaining()) {
      throw new FormatError('wspr: frame count does not fit the data');
    }
    const frames: Frame[] = [];
    for (let f = 0; f < frameCount; f++) {
      const event = r.i16();
      const anchors: [number, number][] = [];
      for (let k = r.u8(); k > 0; k--) {
        anchors.push([r.i16(), r.i16()]);
      }
      const layerCount = r.u16();
      if (layerCount * 23 > r.remaining()) {
        throw new FormatError('wspr: layer count does not fit the data');
      }
      const layers: Layer[] = [];
      for (let l = 0; l < layerCount; l++) {
        const image = r.i32();
        const x = r.i16();
        const y = r.i16();
        const mirror = r.u8();
        if (mirror > 1) {
          throw new FormatError('wspr: mirror flag');
        }
        const c = r.take(4);
        const layer: Layer = {
          image,
          x,
          y,
          mirror: mirror === 1,
          color: [c[0]!, c[1]!, c[2]!, c[3]!],
          scaleX: r.f32(),
          scaleY: r.f32(),
          rotation: r.u16(),
        };
        if (image < -1 || image >= imageCount || layer.rotation >= 360) {
          throw new FormatError('wspr: layer');
        }
        layers.push(layer);
      }
      frames.push({ event, anchors, layers });
    }
    s.actions.push({ intervalMs, frames });
  }
  for (let i = 0; i < eventCount; i++) {
    s.events.push(r.str());
  }
  for (const a of s.actions) {
    for (const f of a.frames) {
      if (f.event < -1 || f.event >= s.events.length) {
        throw new FormatError('wspr: event');
      }
    }
  }
  return s;
}

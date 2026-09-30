import {
  ClampToEdgeWrapping,
  DataTexture,
  LinearFilter,
  LinearMipmapLinearFilter,
  NoColorSpace,
  RepeatWrapping,
  Texture,
} from 'three';
import type { AssetStore } from '../assets/store';

/** A uniform whose texture is replaced when the asset arrives. */
export interface TextureUniform {
  value: Texture;
}

export interface TextureOptions {
  /** Repeat outside 0 to 1. The default. */
  repeat?: boolean;
  /** Build mipmaps. The default. Lightmaps turn this off. */
  mipmaps?: boolean;
}

// Colors stay in RO's own space: the shaders do their math on the stored
// values, as the game does, and write the result unchanged.
function grey(): DataTexture {
  const t = new DataTexture(new Uint8Array([128, 128, 128, 255]), 1, 1);
  t.colorSpace = NoColorSpace;
  t.needsUpdate = true;
  return t;
}

/** Gives each texture name a uniform that holds its texture. */
export interface TextureSource {
  uniform(name: string, options?: TextureOptions): TextureUniform;
}

/**
 * Textures by asset name. Each name has one uniform, shared by every
 * material that uses it, so a texture that arrives late shows up
 * everywhere at once. Until then, materials draw a grey texel.
 */
export class TextureCache implements TextureSource {
  private readonly uniforms = new Map<string, TextureUniform>();
  private readonly pending = new WeakMap<TextureUniform, Promise<void>>();
  private readonly loading = new Set<Promise<void>>();
  private readonly fallback = grey();
  private placeholder: Promise<Texture | undefined> | undefined;

  constructor(private readonly store: AssetStore) {}

  uniform(name: string, options: TextureOptions = {}): TextureUniform {
    const key = `${name}|${options.repeat === false ? 'c' : 'r'}${options.mipmaps === false ? 'n' : 'm'}`;
    let u = this.uniforms.get(key);
    if (u === undefined) {
      u = { value: this.fallback };
      this.uniforms.set(key, u);
      const fill = this.fill(u, name, options);
      this.pending.set(u, fill);
      this.loading.add(fill);
    }
    return u;
  }

  /** Resolves when no texture is loading. */
  async idle(): Promise<void> {
    while (this.loading.size > 0) {
      const batch = [...this.loading];
      // Loads never reject. They log their own errors.
      await Promise.all(batch);
      for (const p of batch) {
        this.loading.delete(p);
      }
    }
  }

  /** Resolves when a texture has loaded, or is known to be missing. */
  async load(name: string, options: TextureOptions = {}): Promise<void> {
    const u = this.uniform(name, options);
    await this.pending.get(u);
  }

  private async fill(u: TextureUniform, name: string, options: TextureOptions): Promise<void> {
    try {
      let t = await this.decode(name, options);
      if (t === undefined && !name.startsWith('placeholder/')) {
        // A missing texture shows the placeholder, as assets-v1 says.
        this.placeholder ??= this.decode('placeholder/texture', {});
        t = await this.placeholder;
      }
      if (t !== undefined) {
        u.value = t;
      }
    } catch (error) {
      console.error(`texture ${name}:`, error);
    }
  }

  private async decode(name: string, options: TextureOptions): Promise<Texture | undefined> {
    const data = await this.store.bytes(name, 'texture');
    if (data === undefined) {
      return undefined;
    }
    const bitmap = await createImageBitmap(new Blob([data], { type: 'image/webp' }), {
      premultiplyAlpha: 'none',
      colorSpaceConversion: 'none',
    });
    if (bitmap.width > 4096 || bitmap.height > 4096) {
      bitmap.close();
      throw new Error('texture is over 4096 × 4096');
    }
    const t = new Texture(bitmap);
    // Image rows run top to bottom, as texture coordinates in glTF and RO
    // expect.
    t.flipY = false;
    t.colorSpace = NoColorSpace;
    t.premultiplyAlpha = false;
    const wrap = options.repeat === false ? ClampToEdgeWrapping : RepeatWrapping;
    t.wrapS = wrap;
    t.wrapT = wrap;
    t.magFilter = LinearFilter;
    if (options.mipmaps === false) {
      t.minFilter = LinearFilter;
      t.generateMipmaps = false;
    } else {
      t.minFilter = LinearMipmapLinearFilter;
    }
    t.anisotropy = 4;
    t.needsUpdate = true;
    return t;
  }
}

import {
  type AnimationClip,
  AnimationMixer,
  DataTexture,
  DoubleSide,
  FrontSide,
  type Group,
  Mesh,
  MeshBasicMaterial,
  MeshStandardMaterial,
  NoColorSpace,
  type Object3D,
  ShaderMaterial,
  Texture,
  Vector4,
} from 'three';
import { GLTFLoader, type GLTFParser } from 'three/addons/loaders/GLTFLoader.js';
import type { AssetStore } from '../assets/store';
import { LIGHT_GLSL, type LightUniforms } from './light';
import type { TextureSource, TextureUniform } from './textures';

// Loads models in the assets-v1 glTF profile. Images name textures with
// "asset:" URIs, which resolve through the texture cache, so a texture
// that several models share downloads once.

const VERTEX = /* glsl */ `
varying vec2 vUv;
varying vec3 vNormal;

void main() {
  vUv = uv;
  vNormal = transpose(inverse(mat3(modelMatrix))) * normal;
  gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
}
`;

const FRAGMENT = /* glsl */ `
${LIGHT_GLSL}
uniform sampler2D map;
uniform vec4 baseColor;
uniform float alphaTest;
varying vec2 vUv;
varying vec3 vNormal;

void main() {
  vec4 tex = texture2D(map, vUv) * baseColor;
  if (tex.a < alphaTest) discard;
  vec3 normal = gl_FrontFacing ? vNormal : -vNormal;
  gl_FragColor = vec4(tex.rgb * roLight(normal), tex.a);
}
`;

const ASSET_URI = 'asset:';

/** Reads a field of parsed JSON, or undefined. */
function field(o: unknown, key: string | number): unknown {
  if (typeof o !== 'object' || o === null) {
    return undefined;
  }
  const v: unknown = Reflect.get(o, key);
  return v;
}

/** The asset name in an "asset:" URI, whose segments are percent-encoded. */
export function assetURIName(uri: string): string | undefined {
  if (!uri.startsWith(ASSET_URI)) {
    return undefined;
  }
  try {
    const segments = uri.slice(ASSET_URI.length).split('/').map(decodeURIComponent);
    // An encoded slash would change the name's segments.
    return segments.some((s) => s.includes('/')) ? undefined : segments.join('/');
  } catch {
    return undefined;
  }
}

/**
 * Stands in for glTF textures. The loader gets a texture that only
 * carries the asset name, and materials are rebuilt with the cache's
 * uniform for that name.
 */
class AssetTexturePlugin {
  readonly name = 'wathena_asset_textures';

  constructor(private readonly parser: GLTFParser) {}

  loadTexture(index: number): Promise<Texture> | null {
    const json: unknown = this.parser.json;
    const source = field(field(field(json, 'textures'), index), 'source');
    const uri =
      typeof source === 'number' ? field(field(field(json, 'images'), source), 'uri') : undefined;
    const name = typeof uri === 'string' ? assetURIName(uri) : undefined;
    if (name === undefined) {
      return null;
    }
    const t = new Texture();
    t.userData.asset = name;
    return Promise.resolve(t);
  }
}

type GLTFMaterial = MeshStandardMaterial | MeshBasicMaterial;

interface Template {
  scene: Group;
  clips: AnimationClip[];
}

function white(): TextureUniform {
  const t = new DataTexture(new Uint8Array([255, 255, 255, 255]), 1, 1);
  t.colorSpace = NoColorSpace;
  t.needsUpdate = true;
  return { value: t };
}

/** Models by asset name, with one instance per placement. */
export class ModelCache {
  private readonly templates = new Map<string, Promise<Template | undefined>>();
  private readonly loader = new GLTFLoader();
  private readonly white = white();
  private readonly mixers: AnimationMixer[] = [];

  constructor(
    private readonly store: AssetStore,
    private readonly textures: TextureSource,
    private readonly light: LightUniforms,
  ) {
    this.loader.register((parser) => new AssetTexturePlugin(parser));
  }

  /**
   * A new instance of a model, or undefined if the build does not have it
   * and has no placeholder.
   */
  async instance(name: string): Promise<Object3D | undefined> {
    let t = this.templates.get(name);
    if (t === undefined) {
      t = this.load(name);
      this.templates.set(name, t);
    }
    let template = await t;
    if (template === undefined && name !== 'placeholder/model') {
      template = await this.placeholderTemplate();
    }
    if (template === undefined) {
      return undefined;
    }
    const obj = template.scene.clone();
    if (template.clips.length > 0) {
      const mixer = new AnimationMixer(obj);
      for (const clip of template.clips) {
        mixer.clipAction(clip).play();
      }
      this.mixers.push(mixer);
    }
    return obj;
  }

  /** Advances every animation. */
  update(seconds: number): void {
    for (const m of this.mixers) {
      m.update(seconds);
    }
  }

  private async placeholderTemplate(): Promise<Template | undefined> {
    let t = this.templates.get('placeholder/model');
    if (t === undefined) {
      t = this.load('placeholder/model');
      this.templates.set('placeholder/model', t);
    }
    return t;
  }

  private async load(name: string): Promise<Template | undefined> {
    try {
      const data = await this.store.bytes(name, 'model');
      if (data === undefined) {
        return undefined;
      }
      const gltf = await this.loader.parseAsync(data.buffer, '');
      const json: unknown = gltf.parser.json;
      gltf.animations.forEach((clip, i) => {
        // The loop can be longer than the last keyframe.
        const loop = field(field(field(field(json, 'animations'), i), 'extras'), 'loopSeconds');
        if (typeof loop === 'number' && loop > 0 && Number.isFinite(loop)) {
          clip.duration = loop;
        }
      });
      const materials = new Map<GLTFMaterial, ShaderMaterial>();
      gltf.scene.traverse((o) => {
        const m: unknown = o instanceof Mesh ? o.material : undefined;
        // The loader makes standard materials, and basic ones for unlit
        // materials.
        if (
          o instanceof Mesh &&
          (m instanceof MeshStandardMaterial || m instanceof MeshBasicMaterial)
        ) {
          let sm = materials.get(m);
          if (sm === undefined) {
            sm = this.material(m);
            materials.set(m, sm);
          }
          o.material = sm;
        }
      });
      return { scene: gltf.scene, clips: gltf.animations };
    } catch (error) {
      console.error(`model ${name}:`, error);
      return undefined;
    }
  }

  /** Rebuilds a glTF material with RO's lighting. */
  private material(m: GLTFMaterial): ShaderMaterial {
    const asset: unknown = m.map?.userData.asset;
    const c = m.color;
    const map = typeof asset === 'string' ? this.textures.uniform(asset) : this.white;
    const blend = m.transparent;
    // Masked materials cut at their alpha cutoff. Blended ones only skip
    // texels that are fully transparent.
    let alphaTest = Math.max(m.alphaTest, 0);
    if (blend) {
      alphaTest = 1 / 255;
    }
    return new ShaderMaterial({
      vertexShader: VERTEX,
      fragmentShader: FRAGMENT,
      side: m.side === DoubleSide ? DoubleSide : FrontSide,
      transparent: blend,
      depthWrite: !blend,
      uniforms: {
        ...this.light,
        map,
        baseColor: { value: new Vector4(c.r, c.g, c.b, m.opacity) },
        alphaTest: { value: alphaTest },
      },
    });
  }
}

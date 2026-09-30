import { Group } from 'three';
import type { AssetStore } from '../assets/store';
import { type WMap, decodeWMap } from '../formats/wmap';
import { EffectCache } from './effects';
import buildGround from './ground';
import { lightUniforms } from './light';
import MapEffects from './mapEffects';
import { ModelCache } from './models';
import { TextureCache } from './textures';
import Water from './water';

/** A map's scene: ground, water, and placed models. */
export default class MapScene {
  readonly group = new Group();

  private constructor(
    readonly map: WMap,
    /** The map's textures, which effects and other scenes can share. */
    readonly textures: TextureCache,
    private readonly models: ModelCache,
    private readonly water: Water,
    private readonly effects: MapEffects,
  ) {}

  /**
   * Loads a map. The ground shows as soon as the map file arrives, and
   * textures and models appear as they load. ready resolves when all
   * have loaded or failed.
   */
  static async load(
    store: AssetStore,
    name: string,
  ): Promise<{ scene: MapScene; ready: Promise<void> }> {
    const data = await store.bytes(name, 'map');
    if (data === undefined) {
      throw new Error(`map ${name} is not in the build`);
    }
    const map = decodeWMap(data);
    const textures = new TextureCache(store);
    const light = lightUniforms(map.light);
    const models = new ModelCache(store, textures, light);
    const water = new Water(map, textures);
    const effects = new MapEffects();
    const scene = new MapScene(map, textures, models, water, effects);
    scene.group.add(buildGround(map, textures, light));
    scene.group.add(water.group, effects.group);

    const objects = new Group();
    objects.name = 'objects';
    scene.group.add(objects);
    // Every dependency starts loading at once. The store shares downloads
    // between names with the same content.
    const pending: Promise<unknown>[] = map.deps
      .filter((d) => d.startsWith('texture/') || d.startsWith('lightmap/'))
      .map(async (d) =>
        textures.load(d, d.startsWith('lightmap/') ? { repeat: false, mipmaps: false } : {}),
      );
    for (const o of map.objects) {
      pending.push(
        models.instance(o.model).then((obj) => {
          if (obj === undefined) {
            return;
          }
          const holder = new Group();
          holder.matrixAutoUpdate = false;
          holder.matrix.fromArray(o.matrix);
          holder.add(obj);
          objects.add(holder);
        }),
      );
    }
    pending.push(effects.load(map, new EffectCache(store), textures));
    // Models ask for their textures as they load, so textures finish last.
    const ready = Promise.allSettled(pending).then(async () => textures.idle());
    return { scene, ready };
  }

  update(seconds: number, delta: number): void {
    this.models.update(delta);
    this.water.update(seconds);
    this.effects.update(delta * 1000);
  }
}

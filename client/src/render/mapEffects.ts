import { Group } from 'three';
import { effects as table } from '../data/tables';
import type { WMap } from '../formats/wmap';
import { type EffectCache, EffectView } from './effects';
import type { TextureSource } from './textures';

// The effects a map places, by RO effect ID, through effects.yml. IDs the
// table lacks show nothing.

export default class MapEffects {
  readonly group = new Group();
  private readonly views: EffectView[] = [];
  private seed = 1;

  private random(n: number): number {
    this.seed = (this.seed * 1103515245 + 12345) % 2 ** 31;
    return this.seed % n;
  }

  /** Loads and places the map's effects. Resolves when all have loaded. */
  async load(map: WMap, effects: EffectCache, textures: TextureSource): Promise<void> {
    this.group.name = 'effects';
    const placed = map.effects.flatMap((e) =>
      (table.get(e.id)?.parts ?? []).map((part) => {
        const name =
          'effect' in part ? part.effect : (part.variants[this.random(part.variants.length)] ?? '');
        return { name, position: e.position };
      }),
    );
    await Promise.all(
      placed.map(async ({ name, position }) => {
        const effect = await effects.get(name);
        if (effect === undefined) {
          return;
        }
        const v = new EffectView(effect, textures);
        v.group.position.set(...position);
        this.group.add(v.group);
        this.views.push(v);
      }),
    );
  }

  update(deltaMs: number): void {
    for (const v of this.views) {
      v.update(deltaMs);
    }
  }
}

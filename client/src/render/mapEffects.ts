import { Group } from 'three';
import { partEffect, effects as table } from '../data/tables';
import type { WMap } from '../formats/wmap';
import { type EffectCache, EffectView } from './effects';
import ParticleEmitter from './particles';
import type { TextureSource } from './textures';

// The effects a map places: effects by RO effect ID, through effects.yml,
// and particle emitters. IDs the table lacks show nothing.

export default class MapEffects {
  readonly group = new Group();
  private readonly views: EffectView[] = [];
  private readonly emitters: ParticleEmitter[] = [];
  private seed = 1;

  private random(n: number): number {
    this.seed = (this.seed * 1103515245 + 12345) % 2 ** 31;
    return this.seed % n;
  }

  /** Loads and places the map's effects. Resolves when all have loaded. */
  async load(map: WMap, effects: EffectCache, textures: TextureSource): Promise<void> {
    this.group.name = 'effects';
    for (const e of map.emitters) {
      const p = new ParticleEmitter(e, textures);
      this.group.add(p.mesh);
      this.emitters.push(p);
    }
    // Placed effects play no sounds. Map sounds come from the map's own list.
    const placed = map.effects.flatMap((e) =>
      (table.get(e.id)?.parts ?? []).flatMap((part) => {
        const name = partEffect(part, (n) => this.random(n));
        return name === undefined ? [] : [{ name, position: e.position }];
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
    for (const p of this.emitters) {
      p.update(deltaMs / 1000);
    }
  }
}

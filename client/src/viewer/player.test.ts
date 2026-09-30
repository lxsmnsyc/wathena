import { readFileSync } from 'node:fs';
import { Vector3 } from 'three';
import { describe, expect, it } from 'vitest';
import type { WMap } from '../formats/wmap';
import type { Character } from '../render/characters';
import { Pathfinder } from '../world/pathfinder';
import Player from './player';

const wasm = readFileSync(new URL('../world/path.wasm', import.meta.url));

/** A 10 × 10 open map, with rAthena's blocked last row and column. */
function map(): WMap {
  const n = 10;
  return {
    width: n,
    height: n,
    cellFlags: new Uint8Array(n * n).fill(3),
    cellHeights: new Float32Array(n * n * 4),
  } as unknown as WMap;
}

function character(): Character {
  return {
    group: { position: new Vector3() },
    direction: 0,
    play: () => undefined,
    update: () => undefined,
  } as unknown as Character;
}

describe('Player', () => {
  it('walks a path at rAthena step times', async () => {
    const p = await Pathfinder.fromBytes(wasm);
    const m = map();
    p.setMap(m);
    const c = character();
    const player = new Player(c, m, p, 1, 1);
    expect(player.moveTo(3, 1)).toBe(true);
    const toward = new Vector3(0, 0, 1);
    player.update(75, 0, toward);
    // Halfway through the first eastward step.
    expect(c.group.position.x).toBeCloseTo(2);
    expect(c.direction).toBe(6);
    player.update(225, 0, toward);
    expect([player.x, player.y]).toEqual([3, 1]);
    expect(c.group.position.x).toBeCloseTo(3.5);
  });

  it('rejects unreachable cells', async () => {
    const p = await Pathfinder.fromBytes(wasm);
    const m = map();
    p.setMap(m);
    const player = new Player(character(), m, p, 1, 1);
    // The last column is never walkable.
    expect(player.moveTo(9, 1)).toBe(false);
  });
});

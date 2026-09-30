import { Group, type Vector3 } from 'three';
import { jobs, looks } from '../data/tables';
import type { WMap } from '../formats/wmap';
import type { Event, Look as LookMessage } from '../gen/wathena/v1/event_pb';
import { unpackSteps } from '../net/connection';
import { Character, type Look } from '../render/characters';
import type { SpriteCache } from '../render/sprites';
import type { Pathfinder } from '../world/pathfinder';
import Player from './player';

// Shows the entities a server sends: players appear, walk the paths the
// server gives, and vanish.

/** A character's sprites from the protocol's table IDs. */
export function lookOf(m: LookMessage | undefined): Look {
  const job = jobs.get(m?.body ?? 0) ?? jobs.get(0);
  const name = job?.look ?? 'novice';
  const info = looks.get(name);
  const sex = info?.sex ?? (m?.sex === 1 ? 'm' : 'f');
  const race = info?.race ?? 'human';
  const hair = Math.max(m?.hair ?? 1, 1);
  return {
    body: `sprite/body/${name}/${sex}`,
    head: `sprite/head/${race}/${sex}/${hair}`,
    hairPalette: `palette/hair/${race}/${sex}/${hair}/${m?.hairColor ?? 0}`,
  };
}

export default class Online {
  readonly group = new Group();
  private readonly units = new Map<number, Player>();
  /** Events that arrived before the map loaded. */
  private waiting: Event[] = [];
  private map: WMap | undefined;

  constructor(
    /** The player's own entity ID. */
    public self: number,
    private readonly sprites: SpriteCache,
    private readonly pathfinder: Pathfinder,
  ) {
    this.group.name = 'online';
  }

  /** The player's own character, once it has appeared. */
  get me(): Player | undefined {
    return this.units.get(this.self);
  }

  setMap(map: WMap): void {
    this.map = map;
    const waiting = this.waiting;
    this.waiting = [];
    this.handle(waiting);
  }

  handle(events: Event[]): void {
    if (this.map === undefined) {
      this.waiting.push(...events);
      return;
    }
    for (const e of events) {
      switch (e.body.case) {
        case 'appear': {
          const a = e.body.value;
          this.remove(a.id);
          const map = this.map;
          // A placeholder holds the entity's place until its sprites load.
          const pending = {
            x: a.x,
            y: a.y,
            dir: a.dir,
            walk: undefined as Parameters<Player['follow']> | undefined,
          };
          this.pending.set(a.id, pending);
          Character.load(this.sprites, lookOf(a.look))
            .then((c) => {
              if (c === undefined || this.pending.get(a.id) !== pending) {
                return;
              }
              this.pending.delete(a.id);
              const unit = new Player(
                c,
                map,
                this.pathfinder,
                pending.x,
                pending.y,
                a.walkMs || undefined,
              );
              unit.stand(pending.x, pending.y, pending.dir);
              if (pending.walk !== undefined) {
                unit.follow(...pending.walk);
              }
              this.units.set(a.id, unit);
              this.group.add(c.group);
            })
            .catch(reportError);
          break;
        }
        case 'move': {
          const m = e.body.value;
          // Steps are facings. The pathfinder's directions mirror them.
          const steps = unpackSteps(m.steps).map((f) => (((4 - f) % 8) + 8) % 8);
          const walk: Parameters<Player['follow']> = [m.fromX, m.fromY, steps, m.startMs];
          const unit = this.units.get(m.id);
          if (unit === undefined) {
            const p = this.pending.get(m.id);
            if (p !== undefined) {
              p.walk = walk;
            }
          } else {
            unit.follow(...walk);
          }
          break;
        }
        case 'stop': {
          const s = e.body.value;
          this.units.get(s.id)?.stand(s.x, s.y, this.units.get(s.id)?.character.direction ?? 0);
          break;
        }
        case 'vanish':
          this.remove(e.body.value.id);
          break;
        default:
          // Events this client does not show yet.
          break;
      }
    }
  }

  private readonly pending = new Map<
    number,
    { x: number; y: number; dir: number; walk: Parameters<Player['follow']> | undefined }
  >();

  private remove(id: number): void {
    this.pending.delete(id);
    const unit = this.units.get(id);
    if (unit !== undefined) {
      this.group.remove(unit.character.group);
      this.units.delete(id);
    }
  }

  update(deltaMs: number, cameraSteps: number, towardCamera: Vector3): void {
    for (const unit of this.units.values()) {
      unit.update(deltaMs, cameraSteps, towardCamera);
    }
  }
}

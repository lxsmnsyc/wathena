import { type Camera, Group, Vector3 } from 'three';
import { jobs, looks } from '../data/tables';
import type { WMap } from '../formats/wmap';
import {
  DamageKind,
  EntityKind,
  type Event,
  type Look as LookMessage,
  StatId,
  VanishReason,
} from '../gen/wathena/v1/event_pb';
import { unpackSteps } from '../net/connection';
import { Character, type Look } from '../render/characters';
import { type SpriteCache, SpriteView } from '../render/sprites';
import type { Pathfinder } from '../world/pathfinder';
import Player, { type Body } from './player';

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

/** RO action numbers by entity kind: attack, hurt, and death. */
const ACTIONS = {
  player: { attack: 10, hurt: 6, dead: 8 },
  monster: { attack: 2, hurt: 3, dead: 4 },
} as const;

/** A hit to show: where, how much, and what kind. */
export interface Hit {
  position: Vector3;
  amount: number;
  kind: DamageKind;
  /** Whether the player's own character was hit. */
  mine: boolean;
}

export default class Online {
  readonly group = new Group();
  private readonly units = new Map<number, Player>();
  private readonly kinds = new Map<number, keyof typeof ACTIONS>();
  /** Called for every hit, at the moment it lands. */
  onHit: ((hit: Hit) => void) | undefined;
  /** The player's own HP and max HP, once the server sends them. */
  hp: [number, number] | undefined;
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
          this.kinds.set(a.id, a.kind === EntityKind.MONSTER ? 'monster' : 'player');
          const map = this.map;
          // A placeholder holds the entity's place until its sprites load.
          const pending = {
            x: a.x,
            y: a.y,
            dir: a.dir,
            walk: undefined as Parameters<Player['follow']> | undefined,
          };
          this.pending.set(a.id, pending);
          // Monsters are one sprite, named by monster ID. Players are
          // characters built from their look.
          const body: Promise<Body | undefined> =
            a.kind === EntityKind.MONSTER
              ? this.sprites
                  .get(`sprite/mob/${a.look?.body ?? 0}`)
                  .then((s) => (s === undefined ? undefined : new SpriteView(s)))
              : Character.load(this.sprites, lookOf(a.look));
          body
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
        case 'vanish': {
          const v = e.body.value;
          const unit = this.units.get(v.id);
          if (v.reason === VanishReason.DIED && unit !== undefined) {
            // The body plays its death, then goes.
            unit.act(this.actions(v.id).dead, Infinity);
            this.units.delete(v.id);
            setTimeout(() => {
              this.group.remove(unit.character.group);
            }, 1500);
            this.dying.push(unit);
            break;
          }
          this.remove(v.id);
          break;
        }
        case 'damage': {
          const d = e.body.value;
          const source = this.units.get(d.source);
          const target = this.units.get(d.target);
          if (source !== undefined && target !== undefined) {
            source.face(target.x, target.y);
            source.act(this.actions(d.source).attack, Math.max(d.delayMs, 300));
          }
          setTimeout(() => {
            if (target === undefined) {
              return;
            }
            if (d.targetHpPermille === 0 && this.kinds.get(d.target) === 'player') {
              target.act(this.actions(d.target).dead, Infinity);
            } else if (d.kind !== DamageKind.MISS && d.kind !== DamageKind.LUCKY_DODGE) {
              target.act(this.actions(d.target).hurt, 300);
            }
            this.onHit?.({
              position: target.character.group.position.clone(),
              amount: Number(d.amount),
              kind: d.kind,
              mine: d.target === this.self,
            });
          }, d.delayMs);
          break;
        }
        case 'stats': {
          const hp = e.body.value.stats.find((s) => s.id === StatId.HP)?.value;
          const max = e.body.value.stats.find((s) => s.id === StatId.MAX_HP)?.value;
          if (hp !== undefined && max !== undefined) {
            this.hp = [Number(hp), Number(max)];
          }
          break;
        }
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

  /** Units playing their death before they go. */
  private dying: Player[] = [];

  private actions(id: number): (typeof ACTIONS)[keyof typeof ACTIONS] {
    return ACTIONS[this.kinds.get(id) ?? 'player'];
  }

  /**
   * The entity nearest a point on the screen, within 40 pixels, other than
   * the player. Sprites face the camera, so this compares screen positions
   * a cell above each unit's feet.
   */
  pick(
    screenX: number,
    screenY: number,
    camera: Camera,
    width: number,
    height: number,
  ): number | undefined {
    let best: number | undefined;
    let bestDistance = 40;
    const p = new Vector3();
    for (const [id, unit] of this.units) {
      if (id === this.self) {
        continue;
      }
      p.copy(unit.character.group.position);
      p.y += 1;
      p.project(camera);
      const d = Math.hypot(((p.x + 1) / 2) * width - screenX, ((1 - p.y) / 2) * height - screenY);
      if (d < bestDistance) {
        best = id;
        bestDistance = d;
      }
    }
    return best;
  }

  update(deltaMs: number, cameraSteps: number, towardCamera: Vector3): void {
    for (const unit of this.units.values()) {
      unit.update(deltaMs, cameraSteps, towardCamera);
    }
    this.dying = this.dying.filter((u) => u.character.group.parent !== null);
    for (const unit of this.dying) {
      unit.update(deltaMs, cameraSteps, towardCamera);
    }
  }
}

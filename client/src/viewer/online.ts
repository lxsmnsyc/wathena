import { type Camera, Group, Vector3 } from 'three';
import { effects as effectTable, jobs, looks, partEffect } from '../data/tables';
import { type WMap, cellHeight } from '../formats/wmap';
import {
  DamageKind,
  type EffectPlay,
  EntityKind,
  type Event,
  type Look as LookMessage,
  type PickUpFailure,
  StatId,
  VanishReason,
} from '../gen/wathena/v1/event_pb';
import { ChatChannel } from '../gen/wathena/v1/command_pb';
import { unpackSteps } from '../net/connection';
import { Character, type Look } from '../render/characters';
import { type EffectCache, EffectView } from '../render/effects';
import { type SpriteCache, SpriteView } from '../render/sprites';
import type { TextureSource } from '../render/textures';
import type { Pathfinder } from '../world/pathfinder';
import GroundItems from './groundItems';
import nearest, { type Target } from './pick';
import Player, { type Body } from './player';

// Shows the entities a server sends: players appear, walk the paths the
// server gives, and vanish. Items lie on the ground until someone picks
// them up. Effects play on entities or cells.

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

interface Actions {
  attack: number;
  hurt: number;
  dead: number;
  /** Undefined for sprites without a pick up action. */
  pickUp: number | undefined;
}

/** RO action numbers by entity kind. */
const ACTIONS: Record<'player' | 'monster', Actions> = {
  player: { attack: 10, hurt: 6, dead: 8, pickUp: 3 },
  monster: { attack: 2, hurt: 3, dead: 4, pickUp: undefined },
};

/** How long the pick up action plays, in milliseconds. */
const PICK_UP_MS = 500;

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
  private readonly items: GroundItems;
  /** Called for every hit, at the moment it lands. */
  onHit: ((hit: Hit) => void) | undefined;
  /** Called when an item enters the player's inventory. */
  onInventoryAdd: ((itemId: number, amount: number) => void) | undefined;
  /** Called when the player's pick up fails. */
  onPickUpFailed: ((reason: PickUpFailure) => void) | undefined;
  /** Called for every local chat line, with the speaker's entity ID. */
  onChat: ((from: number, name: string, text: string) => void) | undefined;
  /** Called when an effect plays a sound, with where it plays. */
  onSound: ((name: string, position: Vector3) => void) | undefined;
  /** Where effects come from. Set it when a map loads. */
  effects: { cache: EffectCache; textures: TextureSource } | undefined;
  /** Effects playing once, until they end. */
  private playing: EffectView[] = [];
  /** The player's own stats, as the server last sent them. */
  readonly stats = new Map<StatId, bigint>();
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
    this.items = new GroundItems(sprites);
    this.group.add(this.items.group);
  }

  /** The player's own HP and max HP, once the server sends them. */
  get hp(): [number, number] | undefined {
    const hp = this.stats.get(StatId.HP);
    const max = this.stats.get(StatId.MAX_HP);
    return hp === undefined || max === undefined ? undefined : [Number(hp), Number(max)];
  }

  /** The player's own character, once it has appeared. */
  get me(): Player | undefined {
    return this.units.get(this.self);
  }

  /** Where a unit's feet are, while it is shown. */
  position(id: number): Vector3 | undefined {
    return this.units.get(id)?.character.group.position;
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
          if (a.kind === EntityKind.ITEM) {
            if (a.item !== undefined) {
              this.items.add(a.id, this.map, a.x, a.y, a.item);
            }
            break;
          }
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
          if (this.items.has(v.id)) {
            this.items.remove(v.id);
            break;
          }
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
          // Updates can carry some stats only, so they merge.
          for (const stat of e.body.value.stats) {
            this.stats.set(stat.id, stat.value);
          }
          break;
        }
        case 'itemTaken': {
          const t = e.body.value;
          const source = this.units.get(t.source);
          const cell = this.items.cell(t.item);
          const action = this.actions(t.source).pickUp;
          if (source !== undefined && action !== undefined) {
            if (cell !== undefined) {
              source.face(...cell);
            }
            source.act(action, PICK_UP_MS);
          }
          break;
        }
        case 'inventoryAdd':
          this.onInventoryAdd?.(e.body.value.itemId, e.body.value.amount);
          break;
        case 'pickUpFailed':
          this.onPickUpFailed?.(e.body.value.reason);
          break;
        case 'effect':
          this.playEffect(e.body.value, this.map);
          break;
        case 'chat': {
          const c = e.body.value;
          if (c.channel === ChatChannel.LOCAL) {
            this.onChat?.(c.from, c.fromName, c.text);
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
    this.items.remove(id);
    const unit = this.units.get(id);
    if (unit !== undefined) {
      this.group.remove(unit.character.group);
      this.units.delete(id);
    }
  }

  /** Units playing their death before they go. */
  private dying: Player[] = [];

  private actions(id: number): Actions {
    return ACTIONS[this.kinds.get(id) ?? 'player'];
  }

  /**
   * Plays an effect from effects.yml. On an entity, it follows the entity.
   * Effects the table lacks, and entities not shown, show nothing.
   */
  private playEffect(e: EffectPlay, map: WMap): void {
    const entry = effectTable.get(e.effectId);
    let parent: Group;
    const at = new Vector3();
    // An entity whose sprites are still loading gets the effect on its cell.
    let cell: { x: number; y: number } | undefined;
    const unit = e.at.case === 'entity' ? this.units.get(e.at.value) : undefined;
    if (e.at.case === 'cell') {
      cell = e.at.value;
    } else if (e.at.case === 'entity') {
      cell = this.pending.get(e.at.value);
    }
    if (unit !== undefined) {
      parent = unit.character.group;
    } else if (cell === undefined) {
      return;
    } else {
      const x = cell.x + 0.5;
      const y = cell.y + 0.5;
      at.set(x, cellHeight(map, x, y), -y);
      parent = this.group;
    }
    for (const part of entry?.parts ?? []) {
      if ('sound' in part) {
        this.onSound?.(part.sound, parent === this.group ? at.clone() : parent.position.clone());
        continue;
      }
      const name = partEffect(part, (n) => Math.floor(Math.random() * n));
      const source = this.effects;
      if (name === undefined || source === undefined) {
        continue;
      }
      source.cache
        .get(name)
        .then((effect) => {
          if (effect === undefined) {
            return;
          }
          const v = new EffectView(effect, source.textures);
          v.loop = false;
          v.group.position.copy(at);
          parent.add(v.group);
          this.playing.push(v);
        })
        .catch(reportError);
    }
  }

  /**
   * The unit or ground item nearest a point on the screen, other than the
   * player. Sprites face the camera, so this compares screen positions: a
   * cell above each unit's feet, within 40 pixels, and each item's center,
   * within 20 pixels.
   */
  pick(
    screenX: number,
    screenY: number,
    camera: Camera,
    width: number,
    height: number,
  ): { id: number; item: boolean } | undefined {
    const targets: Target[] = [];
    const p = new Vector3();
    const add = (
      id: number,
      position: Vector3,
      lift: number,
      item: boolean,
      radius: number,
    ): void => {
      p.copy(position);
      p.y += lift;
      p.project(camera);
      targets.push({
        id,
        item,
        x: ((p.x + 1) / 2) * width,
        y: ((1 - p.y) / 2) * height,
        radius,
      });
    };
    for (const [id, unit] of this.units) {
      if (id !== this.self) {
        add(id, unit.character.group.position, 1, false, 40);
      }
    }
    for (const [id, position] of this.items.shown()) {
      add(id, position, 0, true, 20);
    }
    const t = nearest(targets, screenX, screenY);
    return t === undefined ? undefined : { id: t.id, item: t.item };
  }

  update(deltaMs: number, cameraSteps: number, towardCamera: Vector3): void {
    for (const unit of this.units.values()) {
      unit.update(deltaMs, cameraSteps, towardCamera);
    }
    this.dying = this.dying.filter((u) => u.character.group.parent !== null);
    for (const unit of this.dying) {
      unit.update(deltaMs, cameraSteps, towardCamera);
    }
    this.items.update(deltaMs);
    for (const v of this.playing) {
      v.update(deltaMs);
      if (v.done) {
        v.group.removeFromParent();
      }
    }
    this.playing = this.playing.filter((v) => !v.done);
  }
}

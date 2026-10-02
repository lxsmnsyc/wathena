import { create } from '@bufbuild/protobuf';
import { type Camera, Group, Vector3 } from 'three';
import { effects as effectTable, jobs, looks, partEffect, weapons } from '../data/tables';
import { type WMap, cellHeight } from '../formats/wmap';
import {
  DamageKind,
  type DropItemFailure,
  type EffectPlay,
  EffectPlaySchema,
  EntityKind,
  type EquipFailure,
  type Event,
  type ImagePosition,
  type InventoryRemoveReason,
  type Look as LookMessage,
  type MinimapMark,
  type NpcDialog,
  type PickUpFailure,
  type ShopOpen,
  type ShopOutcome,
  StatId,
  type UseItemFailure,
  VanishReason,
} from '../gen/wathena/v1/event_pb';
import { ChatChannel } from '../gen/wathena/v1/command_pb';
import { unpackSteps } from '../net/connection';
import { Character, type Look } from '../render/characters';
import { CylinderView } from '../render/cylinder';
import { type EffectCache, EffectView } from '../render/effects';
import { type SpriteCache, SpriteView } from '../render/sprites';
import type { TextureSource } from '../render/textures';
import type { Pathfinder } from '../world/pathfinder';
import GroundItems from './groundItems';
import { type StackDetails, type StackInput, USE_EFFECTS } from './inventory';
import nearest, { type Target } from './pick';
import Player, { type Body } from './player';

// Shows the entities a server sends: players appear, walk the paths the
// server gives, and vanish. Items lie on the ground until someone picks
// them up. Effects play on entities or cells. Warp portals are NPCs with
// the warp sprite, drawn as the warp effect. Other NPCs are one sprite,
// sprite/npc/<sprite ID>. A map change clears everything, and events
// wait until the new map has loaded.

/** JT_WARPNPC, the NPC sprite of warp portals. */
export const WARP_SPRITE = 45;
/** EF_WARPZONE2, the effect roBrowser plays on warp portals. */
export const WARP_EFFECT = 321;

/** Something that plays until it is done. */
interface Playing {
  group: Group;
  update(deltaMs: number): void;
  readonly done: boolean;
}

/**
 * A character's sprites from the protocol's table IDs. Equipment shows as
 * looks.ts names it:
 * - The weapon type's view from data/weapons.yml, sprite/weapon/<job>/<sex>/<view>.
 * - The shield, sprite/shield/<job>/<sex>/<view>.
 * - Headgear, sprite/headgear/<sex>/<view>.
 * - The garment, sprite/garment/<job>/<sex>/<view>.
 *
 * A view of 0 draws nothing.
 */
export function lookOf(m: LookMessage | undefined): Look {
  const job = jobs.get(m?.body ?? 0) ?? jobs.get(0);
  const name = job?.look ?? 'novice';
  const info = looks.get(name);
  const sex = info?.sex ?? (m?.sex === 1 ? 'm' : 'f');
  const race = info?.race ?? 'human';
  const hair = Math.max(m?.hair ?? 1, 1);
  const look: Look = {
    body: `sprite/body/${name}/${sex}`,
    head: `sprite/head/${race}/${sex}/${hair}`,
    hairPalette: `palette/hair/${race}/${sex}/${hair}/${m?.hairColor ?? 0}`,
  };
  if (m === undefined) {
    return look;
  }
  const weapon = m.weapon === 0 ? 0 : (weapons.get(m.weapon)?.view ?? 0);
  if (weapon !== 0) {
    look.weapon = `sprite/weapon/${name}/${sex}/${weapon}`;
  }
  if (m.shield !== 0) {
    look.shield = `sprite/shield/${name}/${sex}/${m.shield}`;
  }
  if (m.headTop !== 0) {
    look.headgearTop = `sprite/headgear/${sex}/${m.headTop}`;
  }
  if (m.headMid !== 0) {
    look.headgearMid = `sprite/headgear/${sex}/${m.headMid}`;
  }
  if (m.headBottom !== 0) {
    look.headgearBottom = `sprite/headgear/${sex}/${m.headBottom}`;
  }
  if (m.robe !== 0) {
    look.garment = `sprite/garment/${name}/${sex}/${m.robe}`;
  }
  return look;
}

interface Actions {
  attack: number;
  hurt: number;
  dead: number;
  /** Undefined for sprites without a pick up action. */
  pickUp: number | undefined;
}

/** RO action numbers by entity kind. NPCs only stand. */
const ACTIONS: Record<'player' | 'monster' | 'npc', Actions> = {
  player: { attack: 10, hurt: 6, dead: 8, pickUp: 3 },
  monster: { attack: 2, hurt: 3, dead: 4, pickUp: undefined },
  npc: { attack: 0, hurt: 0, dead: 0, pickUp: undefined },
};

/** The actions an entity kind uses. */
function kindOf(kind: EntityKind): keyof typeof ACTIONS {
  if (kind === EntityKind.MONSTER) {
    return 'monster';
  }
  return kind === EntityKind.NPC ? 'npc' : 'player';
}

/** The asset an entity's single sprite comes from: monsters and NPCs by ID. */
function spriteOf(kind: EntityKind, body: number): string {
  return kind === EntityKind.MONSTER ? `sprite/mob/${body}` : `sprite/npc/${body}`;
}

/** An entity whose sprites are still loading, and what happened to it meanwhile. */
interface Pending {
  x: number;
  y: number;
  dir: number;
  walk: Parameters<Player['follow']> | undefined;
  /** The newest look, for a player. */
  look: LookMessage | undefined;
}

/** How long the pick up action plays, in milliseconds. */
const PICK_UP_MS = 500;

/** The emotion icons, one action per emotion (docs/specs/assets-v1.md). */
export const EMOTION_SPRITE = 'sprite/effect/emotion';

/** How high an emotion shows over an entity's feet, in cells. */
const EMOTION_HEIGHT = 3;

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
  /** Called with the player's whole inventory. */
  onInventory: ((items: StackInput[], size: number) => void) | undefined;
  /** Called when an item enters a slot of the player's inventory. */
  onInventoryAdd:
    | ((slot: number, itemId: number, amount: number, details: StackDetails) => void)
    | undefined;
  /** Called when an item goes on, with the EQP_ bits it now takes. */
  onItemEquipped: ((slot: number, position: number) => void) | undefined;
  /** Called when an item comes off. */
  onItemUnequipped: ((slot: number, position: number) => void) | undefined;
  /** Called when putting an item on fails. */
  onEquipFailed: ((slot: number, reason: EquipFailure) => void) | undefined;
  /** Called when a card went into an item. */
  onCardInserted: ((cardSlot: number, equipSlot: number, cardId: number) => void) | undefined;
  /** Called when a card did not go into an item. */
  onInsertCardFailed: ((cardSlot: number, equipSlot: number) => void) | undefined;
  /** Called when an amount leaves a slot of the player's inventory. */
  onInventoryRemove:
    | ((slot: number, amount: number, reason: InventoryRemoveReason) => void)
    | undefined;
  /** Called when using an item fails. */
  onUseItemFailed: ((reason: UseItemFailure) => void) | undefined;
  /** Called when dropping an item fails. */
  onDropItemFailed: ((reason: DropItemFailure) => void) | undefined;
  /** Called when the player's pick up fails. */
  onPickUpFailed: ((reason: PickUpFailure) => void) | undefined;
  /** Called for every local chat line, with the speaker's entity ID. */
  onChat: ((from: number, name: string, text: string) => void) | undefined;
  /** Called for text the server sends to this player alone, such as a mod's message. */
  onServerText: ((text: string) => void) | undefined;
  /** Called for an announcement to the map or the world, with its color or 0. */
  onAnnouncement: ((text: string, color: number) => void) | undefined;
  /** Called for each NPC dialog step, with the NPC's name. */
  onNpcDialog: ((dialog: NpcDialog, name: string) => void) | undefined;
  /** Called to show or hide an NPC illustration. */
  onNpcImage: ((name: string, position: ImagePosition) => void) | undefined;
  /** Called when a progress bar starts over the player. */
  onProgress: ((durationMs: number, color: number) => void) | undefined;
  /** Called for a minimap mark. */
  onMinimapMark: ((mark: MinimapMark) => void) | undefined;
  /** Called when a shop window opens. */
  onShopOpen: ((shop: ShopOpen) => void) | undefined;
  /** Called with a shop's answer. */
  onShopResult: ((outcome: ShopOutcome) => void) | undefined;
  /** Called when an effect plays a sound, with where it plays. */
  onSound: ((name: string, position: Vector3) => void) | undefined;
  /**
   * Called when the server moves the player to another map, with the
   * map's asset name and the player's cell. Events wait until setMap.
   */
  onMapChange: ((map: string, x: number, y: number) => void) | undefined;
  /** Where effects come from. Set it when a map loads. */
  effects: { cache: EffectCache; textures: TextureSource } | undefined;
  /** Effects playing once, until they end. */
  private playing: Playing[] = [];
  /** Warp portals by entity ID, with the effect parts they show. */
  private readonly portals = new Map<number, { group: Group; parts: CylinderView[] }>();
  /** The player's own stats, as the server last sent them. */
  readonly stats = new Map<StatId, bigint>();
  /** Events that arrived before the map loaded. */
  private waiting: Event[] = [];
  /** Entity names, by ID, from EntityAppear. */
  private readonly names = new Map<number, string>();
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

  /** The player's own cell, once it has appeared, even while its sprites load. */
  get selfCell(): [number, number] | undefined {
    const me = this.units.get(this.self);
    if (me !== undefined) {
      return [me.x, me.y];
    }
    const p = this.pending.get(this.self);
    return p === undefined ? undefined : [p.x, p.y];
  }

  /** A ground item's item ID, amount, and position, while it is shown. */
  groundItem(id: number): { itemId: number; amount: number; position: Vector3 } | undefined {
    return this.items.info(id);
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
    for (let n = 0; n < events.length; n++) {
      if (this.map === undefined) {
        this.waiting.push(...events.slice(n));
        return;
      }
      const e = events[n]!;
      switch (e.body.case) {
        case 'mapChange': {
          // Everything shown belongs to the old map. Entity IDs start
          // again on the new one.
          const c = e.body.value;
          this.clear();
          this.self = c.netId;
          this.map = undefined;
          this.onMapChange?.(c.map, c.x, c.y);
          break;
        }
        case 'appear': {
          const a = e.body.value;
          this.remove(a.id);
          if (a.kind === EntityKind.ITEM) {
            if (a.item !== undefined) {
              this.items.add(a.id, this.map, a.x, a.y, a.item);
            }
            break;
          }
          if (a.kind === EntityKind.NPC && a.look?.body === WARP_SPRITE) {
            this.addPortal(a.id, a.x, a.y, this.map);
            break;
          }
          this.kinds.set(a.id, kindOf(a.kind));
          this.names.set(a.id, a.name);
          const map = this.map;
          // A placeholder holds the entity's place until its sprites load.
          const pending: Pending = {
            x: a.x,
            y: a.y,
            dir: a.dir,
            walk: undefined,
            look: a.look,
          };
          this.pending.set(a.id, pending);
          // Monsters and NPCs are one sprite. Players are characters
          // built from their look.
          const single = a.kind === EntityKind.MONSTER || a.kind === EntityKind.NPC;
          const body: Promise<Body | undefined> = single
            ? this.sprites
                .get(spriteOf(a.kind, a.look?.body ?? 0))
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
              // The look changed while the sprites loaded.
              if (pending.look !== a.look) {
                this.changeLook(a.id, pending.look);
              }
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
          if (d.kind === DamageKind.HEAL) {
            // A heal shows its number and nothing else.
            if (target !== undefined) {
              this.onHit?.({
                position: target.character.group.position.clone(),
                amount: Number(d.amount),
                kind: d.kind,
                mine: d.target === this.self,
              });
            }
            break;
          }
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
        case 'inventory':
          this.onInventory?.(
            e.body.value.items.map((i) => ({
              slot: i.slot,
              itemId: i.itemId,
              amount: i.amount,
              equip: i.equip,
              refine: i.refine,
              cards: [...i.cards],
              location: i.location,
              card: i.card,
            })),
            e.body.value.size,
          );
          break;
        case 'inventoryAdd': {
          const a = e.body.value;
          this.onInventoryAdd?.(a.slot, a.itemId, a.amount, {
            refine: a.refine,
            cards: [...a.cards],
            location: a.location,
            card: a.card,
          });
          break;
        }
        case 'itemEquipped':
          this.onItemEquipped?.(e.body.value.slot, e.body.value.position);
          break;
        case 'itemUnequipped':
          this.onItemUnequipped?.(e.body.value.slot, e.body.value.position);
          break;
        case 'equipFailed':
          this.onEquipFailed?.(e.body.value.slot, e.body.value.reason);
          break;
        case 'cardInserted': {
          const c = e.body.value;
          this.onCardInserted?.(c.cardSlot, c.equipSlot, c.cardId);
          break;
        }
        case 'insertCardFailed':
          this.onInsertCardFailed?.(e.body.value.cardSlot, e.body.value.equipSlot);
          break;
        case 'lookChange':
          this.changeLook(e.body.value.id, e.body.value.look);
          break;
        case 'inventoryRemove':
          this.onInventoryRemove?.(e.body.value.slot, e.body.value.amount, e.body.value.reason);
          break;
        case 'itemUsed': {
          // The potion effects play on the user, as the game client
          // does for clif_useitemack.
          const effectId = USE_EFFECTS.get(e.body.value.itemId);
          if (effectId !== undefined) {
            this.playEffect(
              create(EffectPlaySchema, {
                effectId,
                at: { case: 'entity', value: e.body.value.source },
              }),
              this.map,
            );
          }
          break;
        }
        case 'useItemFailed':
          this.onUseItemFailed?.(e.body.value.reason);
          break;
        case 'dropItemFailed':
          this.onDropItemFailed?.(e.body.value.reason);
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
          } else if (c.channel === ChatChannel.SYSTEM) {
            this.onServerText?.(c.text);
          } else if (c.channel === ChatChannel.ANNOUNCEMENT) {
            this.onAnnouncement?.(c.text, c.color);
          }
          break;
        }
        case 'npcDialog': {
          const d = e.body.value;
          this.onNpcDialog?.(d, this.names.get(d.npc) ?? '');
          break;
        }
        case 'npcImage':
          this.onNpcImage?.(e.body.value.name, e.body.value.position);
          break;
        case 'npcProgress':
          this.onProgress?.(e.body.value.durationMs, e.body.value.color);
          break;
        case 'emotion':
          this.showEmotion(e.body.value.entity, e.body.value.emotion);
          break;
        case 'minimapMark':
          this.onMinimapMark?.(e.body.value);
          break;
        case 'shopOpen':
          this.onShopOpen?.(e.body.value);
          break;
        case 'shopResult':
          this.onShopResult?.(e.body.value.outcome);
          break;
        default:
          // Events this client does not show yet.
          break;
      }
    }
  }

  private readonly pending = new Map<number, Pending>();

  /** The look each player's newest LookChange loads, until it shows. */
  private readonly lookLoads = new Map<number, LookMessage | undefined>();

  /**
   * Shows a player's new look, such as after it put on a hat. The new
   * character takes the old one's place, direction and action once its
   * sprites load. A player still loading takes the new look after it has
   * loaded. Monsters and NPCs keep their sprite.
   */
  private changeLook(id: number, look: LookMessage | undefined): void {
    if (this.kinds.get(id) !== 'player') {
      return;
    }
    const pending = this.pending.get(id);
    if (pending !== undefined) {
      pending.look = look;
      return;
    }
    const unit = this.units.get(id);
    if (unit === undefined) {
      return;
    }
    this.lookLoads.set(id, look);
    Character.load(this.sprites, lookOf(look))
      .then((c) => {
        // A later change, a vanish, or a death replaces this one.
        if (c === undefined || this.units.get(id) !== unit || this.lookLoads.get(id) !== look) {
          return;
        }
        this.lookLoads.delete(id);
        const old = unit.character;
        unit.replace(c);
        // Effects and emotions on the old character move to the new one.
        if (old instanceof Character) {
          // Adding a child takes it from the old group, so the list is taken first.
          const added = old.group.children.filter((child) => !old.isPart(child));
          for (const child of added) {
            c.group.add(child);
          }
        }
        this.group.remove(old.group);
        this.group.add(c.group);
      })
      .catch(reportError);
  }

  /**
   * Shows an emotion icon over a unit: the action of the emotion sprite
   * with the emotion's number, played once.
   */
  private showEmotion(id: number, emotion: number): void {
    const unit = this.units.get(id);
    if (unit === undefined) {
      return;
    }
    this.sprites
      .get(EMOTION_SPRITE)
      .then((asset) => {
        if (asset === undefined || this.units.get(id) !== unit) {
          return;
        }
        const v = new SpriteView(asset);
        v.fixed = emotion;
        v.group.position.y = EMOTION_HEIGHT;
        unit.character.group.add(v.group);
        let left = Math.max(v.duration(emotion), 1000);
        this.playing.push({
          group: v.group,
          update: (deltaMs: number) => {
            v.update(deltaMs, 0);
            left -= deltaMs;
          },
          get done() {
            return left <= 0;
          },
        });
      })
      .catch(reportError);
  }

  /** Whether an entity is an NPC, which a click talks to. */
  isNpc(id: number): boolean {
    return this.kinds.get(id) === 'npc';
  }

  private remove(id: number): void {
    this.pending.delete(id);
    this.lookLoads.delete(id);
    this.names.delete(id);
    this.items.remove(id);
    const portal = this.portals.get(id);
    if (portal !== undefined) {
      this.group.remove(portal.group);
      this.portals.delete(id);
    }
    const unit = this.units.get(id);
    if (unit !== undefined) {
      this.group.remove(unit.character.group);
      this.units.delete(id);
    }
  }

  /**
   * Shows a warp portal on a cell: the parts of the warp effect that the
   * client draws, repeating while the portal is in view.
   */
  private addPortal(id: number, x: number, y: number, map: WMap): void {
    const g = new Group();
    g.name = 'portal';
    const cx = x + 0.5;
    const cy = y + 0.5;
    g.position.set(cx, cellHeight(map, cx, cy), -cy);
    const textures = this.effects?.textures;
    const parts: CylinderView[] = [];
    for (const part of effectTable.get(WARP_EFFECT)?.parts ?? []) {
      if ('cylinder' in part && textures !== undefined) {
        const v = new CylinderView(part.cylinder, textures);
        g.add(v.group);
        parts.push(v);
      }
    }
    this.portals.set(id, { group: g, parts });
    this.group.add(g);
  }

  /** Removes everything shown, for a map change. */
  private clear(): void {
    for (const id of [...this.units.keys(), ...this.pending.keys(), ...this.portals.keys()]) {
      this.remove(id);
    }
    for (const [id] of this.items.shown()) {
      this.items.remove(id);
    }
    for (const unit of this.dying) {
      this.group.remove(unit.character.group);
    }
    this.dying = [];
    for (const v of this.playing) {
      v.group.removeFromParent();
    }
    this.playing = [];
    this.kinds.clear();
    this.waiting = [];
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
      if ('cylinder' in part) {
        const textures = this.effects?.textures;
        if (textures !== undefined) {
          const v = new CylinderView({ ...part.cylinder, repeat: false }, textures);
          v.group.position.copy(at);
          parent.add(v.group);
          this.playing.push(v);
        }
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
    for (const portal of this.portals.values()) {
      for (const v of portal.parts) {
        v.update(deltaMs);
      }
    }
    for (const v of this.playing) {
      v.update(deltaMs);
      if (v.done) {
        v.group.removeFromParent();
      }
    }
    this.playing = this.playing.filter((v) => !v.done);
  }
}

import {
  companionSprite,
  jobBody,
  lookJob,
  modMonsterName,
  monsterSound,
  monsterSprite,
  mountSprite,
  npcSprite,
  skillArtEffects,
  viewSprite,
} from '../assets/art';
import { create } from '@bufbuild/protobuf';
import { type Camera, Group, Vector3 } from 'three';
import { effects as effectTable, jobs, looks, partEffect, weapons } from '../data/tables';
import { type WMap, cellHeight } from '../formats/wmap';
import {
  type BuyingStoreBought,
  type BuyingStoreList,
  type BuyingStoreOpened,
  type BuyingStoreResult,
  type ChannelFailure,
  type ChannelInfo,
  type ChannelJoined,
  type ChatRefused,
  DamageKind,
  type DropItemFailure,
  type EffectPlay,
  EffectPlaySchema,
  EntityKind,
  type EquipFailure,
  type Event,
  type Friend,
  type FriendOutcome,
  type GuildAllyRequest,
  type GuildBadge,
  type GuildInfo,
  type GuildInvite,
  type GuildMember,
  type GuildOutcome,
  type GuildStorageLogEntry,
  type ImagePosition,
  type InventoryItem,
  type InventoryRemoveReason,
  type Look as LookMessage,
  type MailNew,
  type MailResult,
  type MailSummary,
  type MinimapMark,
  type NpcDialog,
  type Options,
  type PartyBookingList,
  type PartyInfo,
  type PartyInvite,
  type PartyMember,
  type PartyOutcome,
  type PickUpFailure,
  type ReportOutcome,
  type SanctionNotice,
  type ShopOpen,
  type ShopOutcome,
  type SkillFailure,
  type SkillInfo,
  StatId,
  type StorageKind,
  type StorageOutcome,
  type StoreSearchOpen,
  type StoreSearchResult,
  type TradeOutcome,
  type UseItemFailure,
  VanishReason,
  type VendingResult,
  type VendingSold,
  type WhisperResult,
  type WorldEventInfo,
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
import {
  type CurrencyBalance,
  type Stack,
  type StackDetails,
  type StackInput,
  type StackRolls,
  USE_EFFECTS,
  stack,
  stackRolls,
} from './inventory';
import { playerLimitsOf } from './limits';
import { bodyLook, cartLook, companionLook, mountedBody, withoutMount } from './mounts';
import { appearance } from './options';
import nearest, { type Target } from './pick';
import Player, { type Body } from './player';
import type { WorldEventRun } from './worldEvents';
import { type QuestView, questOf } from './quests';
import { type WorldMapData, worldMapOf } from './worldMap';
import {
  HEAL_SKILLS,
  HIT_GAP_MS,
  Recent,
  type SkillPhase,
  castAura,
  skillEffectIds,
  splitHits,
  unitEffect,
} from './skillEffects';

// Shows the entities a server sends: players appear, walk the paths the
// server gives, and vanish. Items lie on the ground until someone picks
// them up. Effects play on entities or cells. Warp portals are NPCs with
// the warp sprite, drawn as the warp effect. Other NPCs are one sprite,
// sprite/npc/<sprite ID>. Status options tint units, and hide units that
// hide (options.ts). A map change clears everything, and events wait
// until the new map has loaded.

/** JT_WARPNPC, the NPC sprite of warp portals. */
/**
 * modLine marks a line that a mod's script sent, such as a whisper or an
 * announcement, with the mod's ID, so it cannot pass for a player's or
 * staff's line.
 */
export function modLine(mod: string, text: string): string {
  return `[Mod ${mod}] ${text}`;
}

export const WARP_SPRITE = 45;
/** EF_WARPZONE2, the effect roBrowser plays on warp portals. */
export const WARP_EFFECT = 321;

/** A run of a world event as the world events view keeps it. */
export function worldEventRun(i: WorldEventInfo): WorldEventRun {
  return {
    name: i.name,
    displayName: i.displayName,
    type: i.type,
    map: i.map,
    endsAtMs: Number(i.endsAtMs),
  };
}

/** A slot as InventoryItem gives it, for the inventory and storage windows. */
export function stackInput(i: InventoryItem): StackInput {
  return {
    slot: i.slot,
    itemId: i.itemId,
    amount: i.amount,
    equip: i.equip,
    refine: i.refine,
    cards: [...i.cards],
    location: i.location,
    card: i.card,
    ...stackRolls(i),
    bound: i.bound,
    restrictions: i.restrictions,
  };
}

/** A storage window as StorageOpen opens it. */
export interface StorageOpenInfo {
  kind: StorageKind;
  /** How many slots the storage has. */
  size: number;
  /** The first page of the slots that hold items. */
  items: StackInput[];
  /** The extra storage's record name, empty for the others. */
  name: string;
  /** The storage's name as players see it. */
  displayName: string;
  /** The cart's weight and its limit, in tenths. 0 for storage. */
  weight: number;
  maxWeight: number;
  /** How many StorageItems pages follow. */
  pages: number;
}

/** A mail as MailRead shows it, with its attachments as stacks. */
export interface MailView {
  mail: MailSummary;
  body: string;
  /** The attachments. Each slot is the attachment's index. */
  items: Stack[];
}

/** A line of a vending shop, as VendingListItem. */
export interface VendingItem {
  index: number;
  /** The copy for sale, with its amount and its cart slot as slot. */
  item: Stack;
  price: number;
}

/** A vending shop of another player, as VendingList. */
export interface VendingShop {
  vendor: number;
  shop: number;
  title: string;
  seller: string;
  items: VendingItem[];
}

/** A store search result, as StoreSearchHit. */
export interface StoreSearchHitView {
  index: number;
  title: string;
  owner: string;
  map: string;
  x: number;
  y: number;
  item: Stack;
  price: number;
  buying: boolean;
}

/** The shop title over a player, as EntityShop. */
export interface ShopBoard {
  title: string;
  /** Set for a buying store; a vending shop otherwise. */
  buying: boolean;
}

/** An InventoryItem as a stack, or an empty stack when it is missing. */
function itemStack(i: InventoryItem | undefined): Stack {
  return stack(i === undefined ? { slot: 0, itemId: 0, amount: 0 } : stackInput(i));
}

/** An offer in the trade window, as TradeOffer. */
export type TradeOfferValue = { case: 'item'; value: StackInput } | { case: 'zeny'; value: number };

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
 * A view of 0 draws nothing. A view the server gave an item with a
 * view_sprite draws that sprite (art.ts): headgear, garments, shields, and
 * a weapon by its weapon_view. The option bits add a mount, a companion
 * and a cart (mounts.ts):
 * - A mounted player's body, sprite/body/<mounted look>/<sex>.
 * - A companion, sprite/companion/<look>.
 * - A cart, sprite/cart/<n>.
 *
 * A companion or mount record with art draws its art by the record ID in
 * the look (decision 0036). A mount of art keeps the body on foot.
 */
export function lookOf(m: LookMessage | undefined, option = 0): Look {
  const mount = mountSprite(m?.mount ?? 0);
  const bits = mount === undefined ? option : withoutMount(option);
  // A mod's job comes by its own ID: its body art by that ID, and the
  // rest from the job it looks like (review decision 40).
  const own = m?.body ?? 0;
  const body = lookJob(own);
  const job = jobs.get(mountedBody(body, bits)) ?? jobs.get(0);
  const name = job?.look ?? 'novice';
  const info = looks.get(name);
  const sex = info?.sex ?? (m?.sex === 1 ? 'm' : 'f');
  const race = info?.race ?? 'human';
  const hair = Math.max(m?.hair ?? 1, 1);
  const look: Look = {
    // A mod's job may name body sprites of its art.
    body:
      jobBody(own, sex) ??
      jobBody(mountedBody(body, bits), sex) ??
      `sprite/body/${bodyLook(body, bits)}/${sex}`,
    head: `sprite/head/${race}/${sex}/${hair}`,
    hairPalette: `palette/hair/${race}/${sex}/${hair}/${m?.hairColor ?? 0}`,
  };
  const companion = companionLook(bits);
  const companionArt = companionSprite(m?.companion ?? 0);
  if (companionArt !== undefined) {
    look.companion = companionArt;
  } else if (companion !== undefined) {
    look.companion = `sprite/companion/${companion}`;
  }
  if (mount !== undefined) {
    look.mount = mount;
  }
  const cart = cartLook(bits);
  if (cart !== undefined) {
    look.cart = `sprite/cart/${cart}`;
  }
  if (m === undefined) {
    return look;
  }
  const weapon = m.weapon === 0 ? 0 : (weapons.get(m.weapon)?.view ?? 0);
  const weaponArt = m.weaponView === 0 ? undefined : viewSprite(m.weaponView);
  if (weaponArt !== undefined) {
    look.weapon = weaponArt;
  } else if (weapon !== 0) {
    look.weapon = `sprite/weapon/${name}/${sex}/${weapon}`;
  }
  if (m.shield !== 0) {
    look.shield = viewSprite(m.shield) ?? `sprite/shield/${name}/${sex}/${m.shield}`;
  }
  if (m.headTop !== 0) {
    look.headgearTop = viewSprite(m.headTop) ?? `sprite/headgear/${sex}/${m.headTop}`;
  }
  if (m.headMid !== 0) {
    look.headgearMid = viewSprite(m.headMid) ?? `sprite/headgear/${sex}/${m.headMid}`;
  }
  if (m.headBottom !== 0) {
    look.headgearBottom = viewSprite(m.headBottom) ?? `sprite/headgear/${sex}/${m.headBottom}`;
  }
  if (m.robe !== 0) {
    look.garment = viewSprite(m.robe) ?? `sprite/garment/${name}/${sex}/${m.robe}`;
  }
  return look;
}

interface Actions {
  attack: number;
  /** The action of a unit that casts or uses a skill. */
  skill: number;
  hurt: number;
  dead: number;
  /** Undefined for sprites without a pick up action. */
  pickUp: number | undefined;
}

/** RO action numbers by entity kind. NPCs only stand. */
const ACTIONS: Record<'player' | 'monster' | 'npc', Actions> = {
  player: { attack: 10, skill: 12, hurt: 6, dead: 8, pickUp: 3 },
  monster: { attack: 2, skill: 2, hurt: 3, dead: 4, pickUp: undefined },
  npc: { attack: 0, skill: 0, hurt: 0, dead: 0, pickUp: undefined },
};

/** The actions an entity kind uses. */
function kindOf(kind: EntityKind): keyof typeof ACTIONS {
  if (kind === EntityKind.MONSTER) {
    return 'monster';
  }
  return kind === EntityKind.NPC ? 'npc' : 'player';
}

/**
 * The asset an entity's single sprite comes from: monsters and NPCs by ID,
 * or the art the server's client data names for a mod's record (art.ts).
 */
function spriteOf(kind: EntityKind, body: number): string {
  return kind === EntityKind.MONSTER ? monsterSprite(body) : npcSprite(body);
}

/** An entity whose sprites are still loading, and what happened to it meanwhile. */
interface Pending {
  x: number;
  y: number;
  dir: number;
  walk: Parameters<Player['follow']> | undefined;
  /** The newest look, for a player. */
  look: LookMessage | undefined;
  /** The newest status options. */
  options: Options | undefined;
}

/** A player's look and the option bits it is drawn with. */
interface PlayerLook {
  look: LookMessage | undefined;
  option: number;
}

/** Whether two looks draw the same sprites. */
function sameLook(a: PlayerLook, b: PlayerLook): boolean {
  if (a.look !== b.look) {
    return false;
  }
  const x = lookOf(a.look, a.option);
  const y = lookOf(b.look, b.option);
  return x.body === y.body && x.companion === y.companion && x.cart === y.cart;
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
  /** Set on the total that follows the numbers of a multi-hit attack. */
  combo?: true;
}

/** Where an effect plays: on an entity, or on a cell. */
type EffectAt =
  | { case: 'entity'; value: number }
  | { case: 'cell'; value: { x: number; y: number } };

/** How long the caster effect of one use stays one effect, in milliseconds. */
const CASTER_WINDOW_MS = 150;

/** How long a ground effect at one cell stays one effect, in milliseconds. */
const GROUND_WINDOW_MS = 1000;

/** How long a unit plays its skill action after a skill without a cast. */
const SKILL_ACTION_MS = 400;

export default class Online {
  readonly group = new Group();
  private readonly units = new Map<number, Player>();
  private readonly kinds = new Map<number, keyof typeof ACTIONS>();
  private readonly items: GroundItems;
  /** Called for every hit, at the moment it lands. */
  onHit: ((hit: Hit) => void) | undefined;
  /** Called with the player's inventory, or with a later page of it when more is set. */
  onInventory: ((items: StackInput[], size: number, more: boolean) => void) | undefined;
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
  /** Called when the rolls of a slot changed. */
  onItemRollsChange: ((slot: number, rolls: StackRolls) => void) | undefined;
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
  /**
   * Called for every local chat line, with the speaker's entity ID, and
   * the line's color as 0xrrggbb, or 0 for the log's color. A monster's
   * line has a color. lineId identifies a player's line for Report, and
   * is empty for other lines.
   */
  onChat:
    | ((from: number, name: string, text: string, color: number, lineId: Uint8Array) => void)
    | undefined;
  /** Called for a whisper to the player, with the sender's name. */
  onWhisper: ((from: string, text: string, lineId: Uint8Array) => void) | undefined;
  /**
   * Called for a line in a chat channel, such as #trade. name is the
   * speaker's name, or empty for the channel's own line.
   */
  onChannelChat:
    | ((channel: string, name: string, text: string, color: number, lineId: Uint8Array) => void)
    | undefined;
  /** Called for a line said to the player's party. */
  onPartyChat: ((name: string, text: string, lineId: Uint8Array) => void) | undefined;
  /** Called with the player's party, after login and after each change. An id of 0 means none. */
  onPartyInfo: ((info: PartyInfo) => void) | undefined;
  /** Called when one party member's values change, such as its HP. */
  onPartyMember: ((member: PartyMember) => void) | undefined;
  /** Called when a leader invites the player to a party. */
  onPartyInvite: ((invite: PartyInvite) => void) | undefined;
  /** Called with how a party action went, or what happened to the party. */
  onPartyResult: ((outcome: PartyOutcome, name: string) => void) | undefined;
  /** Called when a party member got an item that even item share gave. */
  onPartyItemShared: ((name: string, itemId: number, amount: number) => void) | undefined;
  /** Called with the player's friend list, after login and after each change. */
  onFriendList: ((friends: Friend[]) => void) | undefined;
  /** Called when a friend logs in or out. */
  onFriendStatus: ((name: string, online: boolean) => void) | undefined;
  /** Called when a player asks to be friends. */
  onFriendRequest: ((from: string) => void) | undefined;
  /** Called with how a friend action went. */
  onFriendResult: ((outcome: FriendOutcome, name: string) => void) | undefined;
  /** Called with party booking ads, or the player's own ad. */
  onPartyBookingList: ((list: PartyBookingList) => void) | undefined;
  /** Called for a line said to the player's guild. */
  onGuildChat: ((name: string, text: string, lineId: Uint8Array) => void) | undefined;
  /** Called with the player's guild, after login and after each change. An id of 0 means none. */
  onGuildInfo: ((info: GuildInfo) => void) | undefined;
  /** Called when one guild member's values change, such as its online state. */
  onGuildMember: ((member: GuildMember) => void) | undefined;
  /** Called when a guild member invites the player to a guild. */
  onGuildInvite: ((invite: GuildInvite) => void) | undefined;
  /** Called when another guild asks the player's guild to be allies. */
  onGuildAllyRequest: ((request: GuildAllyRequest) => void) | undefined;
  /** Called with how a guild action went, or what happened to the guild. */
  onGuildResult: ((outcome: GuildOutcome, name: string) => void) | undefined;
  /** Called when a storage window opens, or the player gets a cart, with the first page. */
  onStorageOpen: ((info: StorageOpenInfo) => void) | undefined;
  /** Called with a later page of a storage's slots. page counts from 1. */
  onStorageItems: ((kind: StorageKind, items: StackInput[], page: number) => void) | undefined;
  /**
   * Called with a storage slot after an amount entered it. weight is the
   * cart's weight after the change, in tenths, and 0 for storage.
   */
  onStorageAdd: ((kind: StorageKind, item: StackInput, weight: number) => void) | undefined;
  /** Called when an amount left a storage slot, with weight as onStorageAdd. */
  onStorageRemove:
    | ((kind: StorageKind, slot: number, amount: number, weight: number) => void)
    | undefined;
  /** Called when the server closes the storage window. */
  onStorageClose: ((kind: StorageKind) => void) | undefined;
  /** Called when a storage action failed. */
  onStorageResult: ((outcome: StorageOutcome) => void) | undefined;
  /** Called with the guild storage log, newest first. */
  onGuildStorageLog: ((entries: GuildStorageLogEntry[]) => void) | undefined;
  /** Called when a player asks to trade. */
  onTradeRequest: ((from: string, baseLevel: number) => void) | undefined;
  /** Called when the trade window opens. */
  onTradeOpen: ((partner: string, baseLevel: number) => void) | undefined;
  /** Called with an item or zeny a player offered. mine is set for the player's own offer. */
  onTradeOffer: ((mine: boolean, offer: TradeOfferValue) => void) | undefined;
  /** Called when a player locked its offer. */
  onTradeLock: ((mine: boolean) => void) | undefined;
  /** Called when the trade window closes. */
  onTradeClose: ((outcome: TradeOutcome) => void) | undefined;
  /** Called with how a trade step went. */
  onTradeResult:
    | ((outcome: TradeOutcome, name: string, slot: number, message: string) => void)
    | undefined;
  /** Called with the player's currencies, which replace the ones before. */
  onCurrencies: ((currencies: CurrencyBalance[]) => void) | undefined;
  /**
   * Called with the player's values of the limits it holds, such as
   * inventory_slots, at login and after a script changes one.
   */
  onPlayerLimits: ((limits: ReadonlyMap<string, number>) => void) | undefined;
  /** Called with the player's mails, newest first, as MailBox. */
  onMailBox:
    | ((mails: MailSummary[], size: number, sentToday: number, daily: number) => void)
    | undefined;
  /** Called with a mail the player reads. */
  onMailRead: ((mail: MailView) => void) | undefined;
  /** Called when a mail arrives. */
  onMailNew: ((mail: MailNew) => void) | undefined;
  /** Called when a run of a world event starts or ends. */
  onWorldEventNotice: ((run: WorldEventRun, started: boolean, result: string) => void) | undefined;
  /** Called at login with the runs of world events that last. */
  onWorldEventList: ((runs: WorldEventRun[]) => void) | undefined;
  /**
   * Called with a procedural world's discovered map graph, after login
   * and when the character enters a map for the first time. It replaces
   * the one before. A classic world sends none.
   */
  onWorldMap: ((map: WorldMapData) => void) | undefined;
  /**
   * Called with the player's quest log, after login and after each map
   * change. It replaces the log before.
   */
  onQuestList: ((quests: QuestView[]) => void) | undefined;
  /** Called with a quest to add to the log, or to replace the quest with its ID. */
  onQuestUpdate: ((quest: QuestView) => void) | undefined;
  /** Called with the ID of a quest that left the log. */
  onQuestRemove: ((id: number) => void) | undefined;
  /** Called with how a mail action went. */
  onMailResult: ((result: MailResult) => void) | undefined;
  /** Called when the vending setup window opens, with the most lines. */
  onVendingSetup: ((maxItems: number) => void) | undefined;
  /** Called with the player's own open shop. */
  onVendingOpened: ((title: string, items: VendingItem[]) => void) | undefined;
  /** Called with another player's shop. */
  onVendingList: ((shop: VendingShop) => void) | undefined;
  /** Called when a line of the player's shop sold. */
  onVendingSold: ((sold: VendingSold) => void) | undefined;
  /** Called with how a vending step went. */
  onVendingResult: ((result: VendingResult) => void) | undefined;
  /** Called when the buying store setup window opens, with its lines. */
  onBuyingStoreSetup: ((slots: number) => void) | undefined;
  /** Called with the player's own open buying store. */
  onBuyingStoreOpened: ((store: BuyingStoreOpened) => void) | undefined;
  /** Called with another player's buying store. */
  onBuyingStoreList: ((store: BuyingStoreList) => void) | undefined;
  /** Called when the player's buying store bought items. */
  onBuyingStoreBought: ((bought: BuyingStoreBought) => void) | undefined;
  /** Called with how a buying store step went. */
  onBuyingStoreResult: ((result: BuyingStoreResult) => void) | undefined;
  /** Called when the store search window opens. */
  onStoreSearchOpen: ((open: StoreSearchOpen) => void) | undefined;
  /** Called with a page of store search results. */
  onStoreSearchResults:
    | ((results: StoreSearchHitView[], uses: number, more: boolean) => void)
    | undefined;
  /** Called with what a store search click or query did. */
  onStoreSearchResult: ((result: StoreSearchResult) => void) | undefined;
  /** Called with the server's answer to the player's whisper. */
  onWhisperResult: ((result: WhisperResult) => void) | undefined;
  /** Called when a line the player said was not sent. */
  onChatRefused: ((refused: ChatRefused) => void) | undefined;
  /** Called when the player is in a chat channel. */
  onChannelJoined: ((channel: ChannelJoined) => void) | undefined;
  /** Called when the player is no longer in a chat channel. */
  onChannelLeft: ((name: string) => void) | undefined;
  /** Called with the chat channels the player may join. */
  onChannelList: ((channels: ChannelInfo[]) => void) | undefined;
  /** Called with the members of a chat channel. */
  onChannelMembers: ((name: string, names: string[]) => void) | undefined;
  /** Called when a channel command did nothing. */
  onChannelFailed: ((name: string, reason: ChannelFailure) => void) | undefined;
  /** Called with the player's block list, after login and after each change. */
  onBlockList: ((names: string[], full: boolean) => void) | undefined;
  /** Called when a mute or a jail starts, lasts, or ends. */
  onSanction: ((notice: SanctionNotice) => void) | undefined;
  /** Called with the server's answer to a report. */
  onReportResult: ((outcome: ReportOutcome) => void) | undefined;
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
  /**
   * Called when one of the player's status icons shows or goes, with its
   * EFST_ ID and how long it lasts in milliseconds, or -1 when it does not
   * end by itself.
   */
  onStatusIcon: ((icon: number, on: boolean, remainingMs: number) => void) | undefined;
  /** Called for a minimap mark. */
  onMinimapMark: ((mark: MinimapMark) => void) | undefined;
  /** Called when a shop window opens. */
  onShopOpen: ((shop: ShopOpen) => void) | undefined;
  /** Called with a shop's answer, and the item it is about or 0. */
  onShopResult: ((outcome: ShopOutcome, itemId: number) => void) | undefined;
  /** Called with the player's skill list. */
  onSkillList: ((skills: SkillInfo[]) => void) | undefined;
  /** Called when the player's skill fails. */
  onSkillFailed: ((skillId: number, reason: SkillFailure) => void) | undefined;
  /** Called when a skill, or every skill for 0, cannot be used for ms. */
  onSkillCooldown: ((skillId: number, ms: number) => void) | undefined;
  /** Called when an entity starts a cast of castMs. */
  onSkillCast: ((source: number, skillId: number, castMs: number) => void) | undefined;
  /** Called when an entity's cast stops before its end. */
  onSkillCastCancel: ((source: number) => void) | undefined;
  /**
   * The skill whose effects a skill plays. A mod's skill names a core
   * skill in the skill list. Others play their own.
   */
  skillIcon: ((skillId: number) => number) | undefined;
  /** The clock for effects that two events ask for at once. Defaults to performance.now. */
  clock: (() => number) | undefined;
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
  /** Skill units by entity ID, with the effects they show while they stay. */
  private readonly skillUnits = new Map<number, { group: Group; parts: Playing[] }>();
  /** The cast auras that play under casting entities, by entity ID. */
  private readonly auras = new Map<number, Playing[]>();
  /** Effects that played a moment ago, so a second ask plays nothing. */
  private readonly recent = new Recent();
  /** The player's own stats, as the server last sent them. */
  readonly stats = new Map<StatId, bigint>();
  /** Job names from the server's JobName events, by job ID. */
  readonly jobNames = new Map<number, string>();
  /** Events that arrived before the map loaded. */
  private waiting: Event[] = [];
  /** Entity names, by ID, from EntityAppear. */
  private readonly names = new Map<number, string>();
  /** The look ID of each monster in sight, for its sounds. */
  private readonly monsterLooks = new Map<number, number>();
  /** The guild shown with each entity in sight, by ID, from EntityAppear and EntityGuild. */
  private readonly badges = new Map<number, GuildBadge>();
  /** The shop titles shown over players in sight, by ID, from EntityShop. */
  private readonly boards = new Map<number, ShopBoard>();
  /** The quest icons over NPCs, by entity ID, from QuestIcon. */
  private readonly icons = new Map<number, string>();
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

  /** The player's own name, once its character has appeared. */
  get selfName(): string | undefined {
    return this.names.get(this.self);
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

  /** The IDs of the items on the ground in sight. */
  groundItemIds(): number[] {
    return this.items.ids();
  }

  /** The IDs of the units in sight, the player's own too. */
  unitIds(): number[] {
    return [...this.units.keys()];
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
          // A quest icon that came first stays with the entity.
          const icon = this.icons.get(a.id);
          this.remove(a.id);
          if (icon !== undefined) {
            this.icons.set(a.id, icon);
          }
          if (a.kind === EntityKind.ITEM) {
            if (a.item !== undefined) {
              this.items.add(a.id, this.map, a.x, a.y, a.item);
            }
            break;
          }
          if (a.kind === EntityKind.SKILL_UNIT) {
            const u = a.skillUnit;
            if (u !== undefined) {
              this.addSkillUnit(a.id, u.unitId, u.skillId, a.x, a.y, this.map);
            }
            break;
          }
          if (a.kind === EntityKind.NPC && a.look?.body === WARP_SPRITE) {
            this.addPortal(a.id, a.x, a.y, this.map);
            break;
          }
          this.kinds.set(a.id, kindOf(a.kind));
          // A mod's monster has the name the client data gives it.
          this.names.set(
            a.id,
            (a.kind === EntityKind.MONSTER ? modMonsterName(a.look?.body ?? 0) : undefined) ??
              a.name,
          );
          if (a.kind === EntityKind.MONSTER) {
            this.monsterLooks.set(a.id, a.look?.body ?? 0);
          }
          if (a.guild !== undefined && a.guild.guildId !== 0n) {
            this.badges.set(a.id, a.guild);
          }
          const map = this.map;
          // A placeholder holds the entity's place until its sprites load.
          const pending: Pending = {
            x: a.x,
            y: a.y,
            dir: a.dir,
            walk: undefined,
            look: a.look,
            options: a.options,
          };
          this.pending.set(a.id, pending);
          // Monsters and NPCs are one sprite. Players are characters
          // built from their look.
          const single = a.kind === EntityKind.MONSTER || a.kind === EntityKind.NPC;
          const body: Promise<Body | undefined> = single
            ? this.sprites
                .get(spriteOf(a.kind, a.look?.body ?? 0))
                .then((s) => (s === undefined ? undefined : new SpriteView(s)))
            : Character.load(this.sprites, lookOf(a.look, a.options?.option));
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
              unit.appearance = appearance(pending.options, a.id === this.self);
              if (pending.walk !== undefined) {
                unit.follow(...pending.walk);
              }
              this.units.set(a.id, unit);
              this.group.add(c.group);
              if (single) {
                return;
              }
              const shown = { look: a.look, option: a.options?.option ?? 0 };
              this.looksShown.set(a.id, shown);
              // The look or options changed while the sprites loaded.
              const option = pending.options?.option ?? 0;
              if (pending.look !== a.look || !sameLook(shown, { look: pending.look, option })) {
                this.loadLook(a.id, pending.look, option);
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
          this.monsterSound(m.id, 'move');
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
            this.monsterSound(v.id, 'die');
            this.monsterLooks.delete(v.id);
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
          const skill = d.skillId;
          const on: EffectAt = { case: 'entity', value: d.target };
          if (skill !== 0) {
            this.endAura(d.source);
            this.playSkill(skill, 'caster', { case: 'entity', value: d.source });
          }
          const missed = d.kind === DamageKind.MISS || d.kind === DamageKind.LUCKY_DODGE;
          this.monsterSound(d.source, 'attack');
          const total = Number(d.amount);
          const hits = missed ? [total] : splitHits(total, d.hits);
          setTimeout(() => {
            if (skill !== 0) {
              this.playSkill(skill, 'effect', on);
            }
            if (target === undefined) {
              return;
            }
            if (d.targetHpPermille === 0 && this.kinds.get(d.target) === 'player') {
              target.act(this.actions(d.target).dead, Infinity);
            } else if (!missed) {
              target.act(this.actions(d.target).hurt, 300);
              this.monsterSound(d.target, 'damage');
            }
          }, d.delayMs);
          // A multi-hit attack shows each hit's number, then the total.
          hits.forEach((amount, i) => {
            setTimeout(
              () => {
                if (skill !== 0 && amount > 0) {
                  this.playSkill(skill, 'hit', on);
                }
                if (target !== undefined) {
                  this.onHit?.({
                    position: target.character.group.position.clone(),
                    amount,
                    kind: d.kind,
                    mine: d.target === this.self,
                  });
                }
              },
              d.delayMs + i * HIT_GAP_MS,
            );
          });
          if (hits.length > 1 && target !== undefined) {
            setTimeout(
              () => {
                this.onHit?.({
                  position: target.character.group.position.clone(),
                  amount: total,
                  kind: d.kind,
                  mine: d.target === this.self,
                  combo: true,
                });
              },
              d.delayMs + hits.length * HIT_GAP_MS,
            );
          }
          break;
        }
        case 'skillCast': {
          const c = e.body.value;
          const source = this.units.get(c.source);
          const cell = this.targetCell(c.target);
          if (source !== undefined) {
            if (cell !== undefined && c.target.value !== c.source) {
              source.face(cell.x, cell.y);
            }
            if (c.castMs > 0) {
              source.act(this.actions(c.source).skill, c.castMs);
            }
          }
          this.endAura(c.source);
          const aura = castAura(this.iconOf(c.skillId), c.element);
          if (aura !== undefined && c.castMs > 0) {
            this.auras.set(
              c.source,
              this.play(aura, { case: 'entity', value: c.source }, this.map, c.castMs),
            );
          }
          this.onSkillCast?.(c.source, c.skillId, c.castMs);
          break;
        }
        case 'skillCastCancel': {
          const source = e.body.value.source;
          this.endAura(source);
          // The cast action stops.
          this.units.get(source)?.act(0, 0);
          this.onSkillCastCancel?.(source);
          break;
        }
        case 'skillEffect': {
          const f = e.body.value;
          const skill = f.skillId;
          const source = this.units.get(f.source);
          let on: EffectAt = { case: 'entity', value: f.source };
          if (f.target.case === 'entity') {
            on = { case: 'entity', value: f.target.value };
          } else if (f.target.case === 'cell') {
            on = { case: 'cell', value: { x: f.target.value.x, y: f.target.value.y } };
          }
          this.endAura(f.source);
          if (source !== undefined) {
            const cell = this.targetCell(f.target);
            if (cell !== undefined && f.target.value !== f.source) {
              source.face(cell.x, cell.y);
            }
            source.act(this.actions(f.source).skill, SKILL_ACTION_MS);
          }
          this.playSkill(skill, 'caster', { case: 'entity', value: f.source });
          if (f.ok) {
            this.playSkill(skill, 'effect', on);
          }
          // Heal and First Aid show the HP healed.
          const healed = on.case === 'entity' ? this.units.get(on.value) : undefined;
          if (HEAL_SKILLS.has(this.iconOf(skill)) && f.value > 0n && healed !== undefined) {
            this.onHit?.({
              position: healed.character.group.position.clone(),
              amount: Number(f.value),
              kind: DamageKind.HEAL,
              mine: on.value === this.self,
            });
          }
          break;
        }
        case 'skillList':
          this.onSkillList?.(e.body.value.skills);
          break;
        case 'skillFailed':
          this.onSkillFailed?.(e.body.value.skillId, e.body.value.reason);
          break;
        case 'skillCooldown':
          this.onSkillCooldown?.(e.body.value.skillId, e.body.value.ms);
          break;
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
            e.body.value.items.map(stackInput),
            e.body.value.size,
            e.body.value.more,
          );
          break;
        case 'inventoryAdd': {
          const a = e.body.value;
          this.onInventoryAdd?.(a.slot, a.itemId, a.amount, {
            refine: a.refine,
            cards: [...a.cards],
            location: a.location,
            card: a.card,
            ...stackRolls(a),
            bound: a.bound,
            restrictions: a.restrictions,
          });
          break;
        }
        case 'jobName':
          this.jobNames.set(e.body.value.job, e.body.value.name);
          break;
        case 'itemRollsChange':
          this.onItemRollsChange?.(e.body.value.slot, stackRolls(e.body.value));
          break;
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
        case 'optionChange':
          this.changeOptions(e.body.value.id, e.body.value.options);
          break;
        case 'statusIcon': {
          const s = e.body.value;
          this.onStatusIcon?.(s.icon, s.on, Number(s.remainingMs));
          break;
        }
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
            this.onChat?.(c.from, c.fromName, c.text, c.color, c.lineId);
          } else if (c.channel === ChatChannel.WHISPER && c.fromMod !== '') {
            // A mod's whisper is not a player's: it shows as a server line
            // with the mod, and opens no whisper tab to answer.
            this.onServerText?.(modLine(c.fromMod, `${c.fromName}: ${c.text}`));
          } else if (c.channel === ChatChannel.WHISPER) {
            this.onWhisper?.(c.fromName, c.text, c.lineId);
          } else if (c.channel === ChatChannel.PARTY) {
            this.onPartyChat?.(c.fromName, c.text, c.lineId);
          } else if (c.channel === ChatChannel.GUILD) {
            this.onGuildChat?.(c.fromName, c.text, c.lineId);
          } else if (c.channel === ChatChannel.NAMED) {
            this.onChannelChat?.(c.channelName, c.fromName, c.text, c.color, c.lineId);
          } else if (c.channel === ChatChannel.SYSTEM) {
            this.onServerText?.(c.text);
          } else if (c.channel === ChatChannel.ANNOUNCEMENT) {
            this.onAnnouncement?.(c.fromMod === '' ? c.text : modLine(c.fromMod, c.text), c.color);
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
          this.onShopResult?.(e.body.value.outcome, e.body.value.itemId);
          break;
        case 'whisperResult':
          this.onWhisperResult?.(e.body.value);
          break;
        case 'chatRefused':
          this.onChatRefused?.(e.body.value);
          break;
        case 'channelJoined':
          this.onChannelJoined?.(e.body.value);
          break;
        case 'channelLeft':
          this.onChannelLeft?.(e.body.value.name);
          break;
        case 'channelList':
          this.onChannelList?.(e.body.value.channels);
          break;
        case 'channelMembers':
          this.onChannelMembers?.(e.body.value.name, [...e.body.value.names]);
          break;
        case 'channelFailed':
          this.onChannelFailed?.(e.body.value.name, e.body.value.reason);
          break;
        case 'blockList':
          this.onBlockList?.([...e.body.value.names], e.body.value.full);
          break;
        case 'sanction':
          this.onSanction?.(e.body.value);
          break;
        case 'reportResult':
          this.onReportResult?.(e.body.value.outcome);
          break;
        case 'partyInfo':
          this.onPartyInfo?.(e.body.value);
          break;
        case 'partyMember':
          if (e.body.value.member !== undefined) {
            this.onPartyMember?.(e.body.value.member);
          }
          break;
        case 'partyInvite':
          this.onPartyInvite?.(e.body.value);
          break;
        case 'partyResult':
          this.onPartyResult?.(e.body.value.outcome, e.body.value.name);
          break;
        case 'partyItemShared':
          this.onPartyItemShared?.(e.body.value.name, e.body.value.itemId, e.body.value.amount);
          break;
        case 'friendList':
          this.onFriendList?.(e.body.value.friends);
          break;
        case 'friendStatus':
          this.onFriendStatus?.(e.body.value.name, e.body.value.online);
          break;
        case 'friendRequest':
          this.onFriendRequest?.(e.body.value.from);
          break;
        case 'friendResult':
          this.onFriendResult?.(e.body.value.outcome, e.body.value.name);
          break;
        case 'partyBookingList':
          this.onPartyBookingList?.(e.body.value);
          break;
        case 'guildInfo':
          this.onGuildInfo?.(e.body.value);
          break;
        case 'guildMember':
          if (e.body.value.member !== undefined) {
            this.onGuildMember?.(e.body.value.member);
          }
          break;
        case 'guildInvite':
          this.onGuildInvite?.(e.body.value);
          break;
        case 'guildAllyRequest':
          this.onGuildAllyRequest?.(e.body.value);
          break;
        case 'guildResult':
          this.onGuildResult?.(e.body.value.outcome, e.body.value.name);
          break;
        case 'entityGuild': {
          const g = e.body.value;
          if (g.guild === undefined || g.guild.guildId === 0n) {
            this.badges.delete(g.id);
          } else if (this.names.has(g.id)) {
            this.badges.set(g.id, g.guild);
          }
          break;
        }
        case 'storageOpen': {
          const o = e.body.value;
          this.onStorageOpen?.({
            kind: o.kind,
            size: o.size,
            items: o.items.map(stackInput),
            name: o.name,
            displayName: o.displayName,
            weight: o.weight,
            maxWeight: o.maxWeight,
            pages: o.pages,
          });
          break;
        }
        case 'storageItems':
          this.onStorageItems?.(
            e.body.value.kind,
            e.body.value.items.map(stackInput),
            e.body.value.page,
          );
          break;
        case 'storageAdd':
          if (e.body.value.item !== undefined) {
            this.onStorageAdd?.(
              e.body.value.kind,
              stackInput(e.body.value.item),
              e.body.value.weight,
            );
          }
          break;
        case 'storageRemove':
          this.onStorageRemove?.(
            e.body.value.kind,
            e.body.value.slot,
            e.body.value.amount,
            e.body.value.weight,
          );
          break;
        case 'storageClose':
          this.onStorageClose?.(e.body.value.kind);
          break;
        case 'storageResult':
          this.onStorageResult?.(e.body.value.outcome);
          break;
        case 'guildStorageLog':
          this.onGuildStorageLog?.(e.body.value.entries);
          break;
        case 'tradeRequest':
          this.onTradeRequest?.(e.body.value.from, e.body.value.baseLevel);
          break;
        case 'tradeOpen':
          this.onTradeOpen?.(e.body.value.partner, e.body.value.baseLevel);
          break;
        case 'tradeOffer': {
          const o = e.body.value;
          if (o.offer.case === 'item') {
            this.onTradeOffer?.(o.mine, { case: 'item', value: stackInput(o.offer.value) });
          } else if (o.offer.case === 'zeny') {
            this.onTradeOffer?.(o.mine, { case: 'zeny', value: o.offer.value });
          }
          break;
        }
        case 'tradeLock':
          this.onTradeLock?.(e.body.value.mine);
          break;
        case 'tradeClose':
          this.onTradeClose?.(e.body.value.outcome);
          break;
        case 'tradeResult': {
          const r = e.body.value;
          this.onTradeResult?.(r.outcome, r.name, r.slot, r.message);
          break;
        }
        case 'playerLimits':
          this.onPlayerLimits?.(playerLimitsOf(e.body.value.limits));
          break;
        case 'currencies':
          this.onCurrencies?.(
            e.body.value.currencies.map((c) => ({
              name: c.name,
              displayName: c.displayName,
              amount: c.amount,
            })),
          );
          break;
        case 'mailBox': {
          const b = e.body.value;
          this.onMailBox?.(b.mails, b.size, b.sentToday, b.daily);
          break;
        }
        case 'mailRead': {
          const r = e.body.value;
          if (r.mail !== undefined) {
            this.onMailRead?.({ mail: r.mail, body: r.body, items: r.items.map(itemStack) });
          }
          break;
        }
        case 'mailNew':
          this.onMailNew?.(e.body.value);
          break;
        case 'worldEventNotice': {
          const notice = e.body.value;
          if (notice.event !== undefined) {
            this.onWorldEventNotice?.(worldEventRun(notice.event), notice.started, notice.result);
          }
          break;
        }
        case 'worldEventList':
          this.onWorldEventList?.(e.body.value.events.map(worldEventRun));
          break;
        case 'worldMap':
          this.onWorldMap?.(worldMapOf(e.body.value));
          break;
        case 'questList': {
          // Timers count from when the list arrived.
          const now = this.clock?.() ?? performance.now();
          this.onQuestList?.(e.body.value.quests.map((q) => questOf(q, now)));
          break;
        }
        case 'questUpdate': {
          const q = e.body.value.quest;
          if (q !== undefined) {
            this.onQuestUpdate?.(questOf(q, this.clock?.() ?? performance.now()));
          }
          break;
        }
        case 'questRemove':
          this.onQuestRemove?.(e.body.value.id);
          break;
        case 'questIcon': {
          const i = e.body.value;
          if (i.icon === '') {
            this.icons.delete(i.npc);
          } else {
            this.icons.set(i.npc, i.icon);
          }
          break;
        }
        case 'mailResult':
          this.onMailResult?.(e.body.value);
          break;
        case 'vendingSetup':
          this.onVendingSetup?.(e.body.value.maxItems);
          break;
        case 'vendingOpened':
          this.onVendingOpened?.(
            e.body.value.title,
            e.body.value.items.map((i) => ({
              index: i.index,
              item: itemStack(i.item),
              price: i.price,
            })),
          );
          break;
        case 'vendingList': {
          const l = e.body.value;
          this.onVendingList?.({
            vendor: l.vendor,
            shop: l.shop,
            title: l.title,
            seller: l.seller,
            items: l.items.map((i) => ({
              index: i.index,
              item: itemStack(i.item),
              price: i.price,
            })),
          });
          break;
        }
        case 'vendingSold':
          this.onVendingSold?.(e.body.value);
          break;
        case 'vendingResult':
          this.onVendingResult?.(e.body.value);
          break;
        case 'entityShop': {
          const b = e.body.value;
          if (b.remove) {
            this.boards.delete(b.id);
          } else {
            this.boards.set(b.id, { title: b.title, buying: b.buying });
          }
          break;
        }
        case 'buyingStoreSetup':
          this.onBuyingStoreSetup?.(e.body.value.slots);
          break;
        case 'buyingStoreOpened':
          this.onBuyingStoreOpened?.(e.body.value);
          break;
        case 'buyingStoreList':
          this.onBuyingStoreList?.(e.body.value);
          break;
        case 'buyingStoreBought':
          this.onBuyingStoreBought?.(e.body.value);
          break;
        case 'buyingStoreResult':
          this.onBuyingStoreResult?.(e.body.value);
          break;
        case 'storeSearchOpen':
          this.onStoreSearchOpen?.(e.body.value);
          break;
        case 'storeSearchResults': {
          const r = e.body.value;
          this.onStoreSearchResults?.(
            r.results.map((h) => ({
              index: h.index,
              title: h.title,
              owner: h.owner,
              map: h.map,
              x: h.x,
              y: h.y,
              item: itemStack(h.item),
              price: h.price,
              buying: h.buying,
            })),
            r.uses,
            r.more,
          );
          break;
        }
        case 'storeSearchResult':
          this.onStoreSearchResult?.(e.body.value);
          break;
        default:
          // Events this client does not show yet.
          break;
      }
    }
  }

  private readonly pending = new Map<number, Pending>();

  /** The look and option bits each player's newest load uses, until it shows. */
  private readonly lookLoads = new Map<number, PlayerLook>();

  /** The newest look and option bits of each player shown. */
  private readonly looksShown = new Map<number, PlayerLook>();

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
    this.loadLook(id, look, this.looksShown.get(id)?.option ?? 0);
  }

  /**
   * Loads a player's character for a look and option bits. The new
   * character takes the old one's place, direction and action once its
   * sprites load.
   */
  private loadLook(id: number, look: LookMessage | undefined, option: number): void {
    const unit = this.units.get(id);
    if (unit === undefined) {
      return;
    }
    const want = { look, option };
    this.looksShown.set(id, want);
    this.lookLoads.set(id, want);
    Character.load(this.sprites, lookOf(look, option))
      .then((c) => {
        // A later change, a vanish, or a death replaces this one.
        if (c === undefined || this.units.get(id) !== unit || this.lookLoads.get(id) !== want) {
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
   * Shows a unit's new status options. A unit still loading takes them
   * once it has loaded. A player whose mount, companion or cart changes
   * loads its character again.
   */
  private changeOptions(id: number, options: Options | undefined): void {
    const pending = this.pending.get(id);
    if (pending !== undefined) {
      pending.options = options;
      return;
    }
    const unit = this.units.get(id);
    if (unit === undefined) {
      return;
    }
    unit.appearance = appearance(options, id === this.self);
    const shown = this.looksShown.get(id);
    if (shown === undefined) {
      return;
    }
    const next = { look: shown.look, option: options?.option ?? 0 };
    if (sameLook(shown, next)) {
      shown.option = next.option;
    } else {
      this.loadLook(id, next.look, next.option);
    }
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

  /** Whether an entity is a player character. */
  isPlayer(id: number): boolean {
    return this.kinds.get(id) === 'player';
  }

  /** Whether a unit in sight walks. */
  walking(id: number): boolean {
    return this.units.get(id)?.walking ?? false;
  }

  /** An entity's name, while it is in sight. */
  nameOf(id: number): string | undefined {
    return this.names.get(id);
  }

  /** The guild shown with an entity in sight, if it has one. */
  guildBadge(id: number): GuildBadge | undefined {
    return this.badges.get(id);
  }

  /** The entities in sight that show a guild, by ID. */
  guildBadges(): ReadonlyMap<number, GuildBadge> {
    return this.badges;
  }

  /** The shop title over a player in sight, if it has an open shop. */
  shopBoard(id: number): ShopBoard | undefined {
    return this.boards.get(id);
  }

  /** The players in sight that show a shop title, by ID. */
  shopBoards(): ReadonlyMap<number, ShopBoard> {
    return this.boards;
  }

  /** The quest icons over NPCs, by entity ID. */
  questIcons(): ReadonlyMap<number, string> {
    return this.icons;
  }

  /** Whether an entity is an NPC, which a click talks to. */
  isNpc(id: number): boolean {
    return this.kinds.get(id) === 'npc';
  }

  private remove(id: number): void {
    this.pending.delete(id);
    this.lookLoads.delete(id);
    this.looksShown.delete(id);
    this.names.delete(id);
    this.monsterLooks.delete(id);
    this.badges.delete(id);
    this.boards.delete(id);
    this.icons.delete(id);
    this.items.remove(id);
    const portal = this.portals.get(id);
    if (portal !== undefined) {
      this.group.remove(portal.group);
      this.portals.delete(id);
    }
    const skillUnit = this.skillUnits.get(id);
    if (skillUnit !== undefined) {
      this.group.remove(skillUnit.group);
      this.skillUnits.delete(id);
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
    for (const id of [
      ...this.units.keys(),
      ...this.pending.keys(),
      ...this.portals.keys(),
      ...this.skillUnits.keys(),
    ]) {
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
    this.auras.clear();
    this.kinds.clear();
    this.badges.clear();
    this.boards.clear();
    this.icons.clear();
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
    if (e.at.case !== undefined) {
      this.play(e.effectId, e.at, map);
    }
  }

  /** The cell of an entity or a cell target, while it is known. */
  private targetCell(
    t:
      | { case: 'entity'; value: number }
      | { case: 'cell'; value: { x: number; y: number } }
      | { case: undefined },
  ): { x: number; y: number } | undefined {
    if (t.case === 'cell') {
      return t.value;
    }
    if (t.case === 'entity') {
      return this.units.get(t.value) ?? this.pending.get(t.value);
    }
    return undefined;
  }

  /**
   * Plays a skill's effects for a moment: the effects its record names in
   * the client data, or else those of the skill whose effects it uses, by
   * the client tables. The caster effect of one use, and the effect of a
   * ground skill on one cell, play once when several events ask for them
   * together.
   */
  private playSkill(skill: number, phase: SkillPhase, at: EffectAt): void {
    const now = this.clock?.() ?? performance.now();
    if (at.case === 'entity' && phase === 'caster') {
      if (this.recent.check(`c:${at.value}:${skill}`, now, CASTER_WINDOW_MS)) {
        return;
      }
    } else if (at.case === 'cell' && phase === 'effect') {
      if (this.recent.check(`g:${skill}:${at.value.x}:${at.value.y}`, now, GROUND_WINDOW_MS)) {
        return;
      }
    }
    if (this.map === undefined) {
      return;
    }
    const named = skillArtEffects(skill, phase);
    if (named !== undefined) {
      for (const name of named) {
        this.playNamed(name, at, this.map);
      }
      return;
    }
    for (const id of skillEffectIds(this.iconOf(skill), phase)) {
      this.play(id, at, this.map);
    }
  }

  /** Plays a monster's sound for a moment, when its record names one. */
  private monsterSound(id: number, moment: string): void {
    const look = this.monsterLooks.get(id);
    if (look === undefined) {
      return;
    }
    const name = monsterSound(look, moment);
    const at = this.units.get(id)?.character.group.position;
    if (name !== undefined && at !== undefined) {
      this.onSound?.(name, at.clone());
    }
  }

  /** Plays an STR effect by its asset name on an entity or a cell, once. */
  private playNamed(name: string, on: EffectAt, map: WMap): void {
    const source = this.effects;
    const unit = on.case === 'entity' ? this.units.get(on.value) : undefined;
    const cell = on.case === 'cell' ? on.value : this.pending.get(on.value);
    if (source === undefined || (unit === undefined && cell === undefined)) {
      return;
    }
    const at = new Vector3();
    let parent: Group = this.group;
    if (unit !== undefined) {
      parent = unit.character.group;
    } else if (cell !== undefined) {
      const x = cell.x + 0.5;
      const y = cell.y + 0.5;
      at.set(x, cellHeight(map, x, y), -y);
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

  /** The skill whose effects a skill plays. */
  private iconOf(skillId: number): number {
    return this.skillIcon?.(skillId) ?? skillId;
  }

  /** Removes the cast aura under an entity, when its cast ends or stops. */
  private endAura(id: number): void {
    const views = this.auras.get(id);
    if (views === undefined) {
      return;
    }
    this.auras.delete(id);
    for (const v of views) {
      v.group.removeFromParent();
    }
    this.playing = this.playing.filter((p) => !views.includes(p));
  }

  /**
   * Plays an effect by ID on an entity or a cell, once. durationMs sets
   * how long its cylinders last, as for a cast aura. It returns the parts
   * that play, which grows as STR effects load.
   */
  private play(effectId: number, on: EffectAt, map: WMap, durationMs?: number): Playing[] {
    const out: Playing[] = [];
    const entry = effectTable.get(effectId);
    let parent: Group;
    const at = new Vector3();
    // An entity whose sprites are still loading gets the effect on its cell.
    let cell: { x: number; y: number } | undefined;
    const unit = on.case === 'entity' ? this.units.get(on.value) : undefined;
    if (on.case === 'cell') {
      cell = on.value;
    } else {
      cell = this.pending.get(on.value);
    }
    if (unit !== undefined) {
      parent = unit.character.group;
    } else if (cell === undefined) {
      return out;
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
          const c = { ...part.cylinder, repeat: false };
          if (durationMs !== undefined) {
            c.duration_ms = durationMs;
          }
          const v = new CylinderView(c, textures);
          v.group.position.copy(at);
          parent.add(v.group);
          this.playing.push(v);
          out.push(v);
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
          out.push(v);
        })
        .catch(reportError);
    }
    return out;
  }

  /**
   * Shows a skill unit on a cell. A unit with its own effect shows it,
   * repeating, until the unit goes. Others play their skill's effect
   * once, as Thunder Storm does.
   */
  private addSkillUnit(
    id: number,
    unitId: number,
    skillId: number,
    x: number,
    y: number,
    map: WMap,
  ): void {
    const skill = this.iconOf(skillId);
    const shown = unitEffect(unitId, skill);
    if (shown === undefined) {
      return;
    }
    if ('once' in shown) {
      this.playSkill(skillId, 'effect', { case: 'cell', value: { x, y } });
      return;
    }
    const g = new Group();
    g.name = 'skill-unit';
    const cx = x + 0.5;
    const cy = y + 0.5;
    g.position.set(cx, cellHeight(map, cx, cy), -cy);
    const parts: Playing[] = [];
    const unit = { group: g, parts };
    this.skillUnits.set(id, unit);
    this.group.add(g);
    for (const part of effectTable.get(shown.stay)?.parts ?? []) {
      if ('sound' in part) {
        this.onSound?.(part.sound, g.position.clone());
        continue;
      }
      if ('cylinder' in part) {
        const textures = this.effects?.textures;
        if (textures !== undefined) {
          const v = new CylinderView({ ...part.cylinder, repeat: true }, textures);
          g.add(v.group);
          parts.push(v);
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
          // The unit may have gone while the effect loaded.
          if (effect === undefined || this.skillUnits.get(id) !== unit) {
            return;
          }
          const v = new EffectView(effect, source.textures);
          g.add(v.group);
          parts.push(v);
        })
        .catch(reportError);
    }
  }

  /**
   * The unit or ground item nearest a point on the screen, other than the
   * player, unless self is set. Sprites face the camera, so this compares screen positions: a
   * cell above each unit's feet, within 40 pixels, and each item's center,
   * within 20 pixels.
   */
  pick(
    screenX: number,
    screenY: number,
    camera: Camera,
    width: number,
    height: number,
    self = false,
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
      // Hidden units cannot be clicked.
      if ((self || id !== this.self) && unit.appearance.visible) {
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
    for (const unit of this.skillUnits.values()) {
      for (const v of unit.parts) {
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

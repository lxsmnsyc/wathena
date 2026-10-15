// A map viewer for development. It loads a map from the build that
// `wactl assets serve` serves, and shows download progress.

import {
  Color,
  ColorManagement,
  Group,
  Mesh,
  MeshBasicMaterial,
  PerspectiveCamera,
  PlaneGeometry,
  Raycaster,
  Scene,
  Timer,
  Vector2,
  Vector3,
  WebGLRenderer,
} from 'three';
import { OrbitControls } from 'three/addons/controls/OrbitControls.js';
import { AssetStore } from '../assets/store';
import { setArt } from '../assets/art';
import { loadClientData } from '../data/clientData';
import { AudioSystem } from '../audio/audio';
import Soundscape from '../audio/soundscape';
import { cellHeight, walkable } from '../formats/wmap';
import MapScene from '../render/map';
import { SpriteCache, SpriteView } from '../render/sprites';
import { Character } from '../render/characters';
import { EffectCache, EffectView } from '../render/effects';
import {
  BuyingStoreOutcome,
  DamageKind,
  GuildOutcome,
  InventoryRemoveReason,
  MailOutcome,
  PartyOutcome,
  SanctionKind,
  StatId,
  StorageKind,
  VendingOutcome,
} from '../gen/wathena/v1/event_pb';
import { ChannelAction } from '../gen/wathena/v1/command_pb';
import { accountPage, accountURL, fetchTicket } from '../net/account';
import gameServerURL from '../net/server';
import { Connection, disconnectDelay, disconnectText } from '../net/connection';
import { Pathfinder } from '../world/pathfinder';
import { Bubbles, ChatLog, ChatRate, chatLine, isTyping } from './chat';
import { type ChatAction, parseChatInput } from './chatCommands';
import {
  type Sanction,
  channelFailureMessage,
  channelListMessage,
  membersMessage,
  refusalMessage,
  reportMessage,
  sanctionMessage,
  whisperMessage,
} from './chatNotices';
import { ChatTabBar, ChatTabs, GUILD_TAB, PARTY_TAB, placeholder, whisperKey } from './chatTabs';
import { BlockPanel, blockChanges } from './blockList';
import { BOOKING_REFUSED, BookingWindow, entryView } from './booking';
import { EmblemCache } from './emblems';
import { FriendPanel, friendsFromList, setFriendOnline } from './friends';
import {
  GuildWindow,
  NO_GUILD,
  badgeLine,
  guildAfterResult,
  guildFromInfo,
  mergeGuildMember,
  permissions,
} from './guild';
import { PlayerMenu } from './playerMenu';
import {
  CART_DRAG_TYPE,
  STORAGE_DRAG_TYPE,
  StorageModel,
  StorageWindow,
  storageMessage,
} from './storage';
import {
  NO_TRADE,
  type TradeEvent,
  TradeWindow,
  tradeMessage,
  tradeReducer,
  tradeRequestText,
} from './trade';
import {
  NO_PARTY,
  PartyWindow,
  leads,
  mergeMember,
  partyAfterResult,
  partyFromInfo,
} from './party';
import { Prompts } from './prompts';
import {
  friendMessage,
  friendRequestText,
  friendStatusMessage,
  guildAllyRequestText,
  guildInviteText,
  guildMessage,
  itemSharedMessage,
  partyInviteText,
  partyMessage,
} from './socialNotices';
import { expLine, hpLine, pickUpMessage } from './hud';
import {
  InventoryModel,
  InventoryWindow,
  ItemIcons,
  MAJOR_OVERWEIGHT,
  SLOT_DRAG_TYPE,
  dropItemMessage,
  gotItemMessage,
  useItemMessage,
  usedItemMessage,
  weightPercent,
} from './inventory';
import { applyPlayerLimits } from './limits';
import { ItemTexts } from './itemText';
import { EquipmentWindow, INSERT_CARD_FAILED, equipMessage } from './equipment';
import { DialogWindow } from './dialog';
import { ShopWindow, shopMessage } from './shop';
import { MailWindow, mailButtonText, mailMessage, mailNewText } from './mail';
import {
  VendingBrowseWindow,
  VendingOwnWindow,
  VendingSetupWindow,
  vendingMessage,
  vendingSoldText,
} from './vending';
import {
  BuyingBrowseWindow,
  BuyingOwnWindow,
  BuyingSetupWindow,
  buyingBoughtText,
  buyingStoreMessage,
  closesOwnStore,
} from './buyingStore';
import { StoreSearchWindow, storeSearchMessage } from './storeSearch';
import { groundItemLabel } from './groundItems';
import randomLook from './looks';
import { StatusIconBar } from './statusIcons';
import { StatusWindow } from './statusWindow';
import { CastBars, Cooldowns, cooldownText } from './cooldowns';
import { HotkeyBar, hotkeySlot, loadHotkeys, saveHotkeys } from './hotkeys';
import {
  SkillModel,
  SkillWindow,
  castLevel,
  castable,
  skillBadge,
  skillFailureMessage,
  skillOf,
} from './skills';
import { TargetMode } from './targeting';
import { type Gesture, GestureRecognizer } from './gestures';
import {
  type Ground,
  type Push,
  WalkRequests,
  cellDirection,
  joystickTarget,
  stopTarget,
} from './joystick';
import { loadSettings, saveSettings } from './settings';
import { InputMode, installTouchUI } from './touch';
import { TouchControls } from './touchControls';
import { makeMovable } from './windows';
import Online, { type ShopBoard } from './online';
import Player from './player';
import Walkers from './walkers';
import { WorldEventsView } from './worldEvents';

// Shaders work on stored color values directly, as RO does.
ColorManagement.enabled = false;

const base = import.meta.env.VITE_ASSETS_URL ?? '/assets/';

const canvas = document.querySelector<HTMLCanvasElement>('#view');
const form = document.querySelector<HTMLFormElement>('#controls');
const input = document.querySelector<HTMLInputElement>('#map');
const status = document.querySelector<HTMLOutputElement>('#status');
const soundButton = document.querySelector<HTMLButtonElement>('#sound');
if (canvas === null || form === null || input === null || status === null || soundButton === null) {
  throw new Error('viewer: page is missing elements');
}

const renderer = new WebGLRenderer({ canvas, antialias: true });
renderer.setPixelRatio(Math.min(devicePixelRatio, 2));
const scene = new Scene();
scene.background = new Color(0x3a4a5c);
const camera = new PerspectiveCamera(30, 1, 0.5, 4000);
const controls = new OrbitControls(camera, canvas);
controls.enableDamping = true;
controls.maxPolarAngle = Math.PI / 2 - 0.05;

function resize(): void {
  const w = canvas?.clientWidth ?? 1;
  const h = canvas?.clientHeight ?? 1;
  renderer.setSize(w, h, false);
  camera.aspect = w / h;
  camera.updateProjectionMatrix();
}
addEventListener('resize', resize);
resize();

let workerBytes = 0;

// The Service Worker caches assets across visits. ?nosw turns it off,
// for measuring downloads. Nothing waits for it: until it controls the
// page, assets come from the network as usual.
if ('serviceWorker' in navigator && !new URL(location.href).searchParams.has('nosw')) {
  const url = `/sw.js?assets=${encodeURIComponent(new URL(base, location.href).href)}`;
  // The worker reports the bytes it downloads, which the page's own
  // timing entries show as zero.
  navigator.serviceWorker.addEventListener('message', (event: MessageEvent<unknown>) => {
    const d = event.data;
    if (
      typeof d === 'object' &&
      d !== null &&
      'type' in d &&
      d.type === 'wathena-downloaded' &&
      'bytes' in d &&
      typeof d.bytes === 'number'
    ) {
      workerBytes += d.bytes;
    }
  });
  navigator.serviceWorker.register(url).catch((error: unknown) => {
    console.error('service worker:', error);
  });
  // Ask the browser to keep the cache when space runs low. Firefox asks
  // the user, so this can stay pending.
  navigator.storage.persist().catch(() => false);
}

const store = await AssetStore.open(base);
// Item names load on first need, and descriptions when first shown.
const itemTexts = new ItemTexts(store);
let current: MapScene | undefined;
let session: { connection: Connection; online: Online } | undefined;
const pathfinder = await Pathfinder.load();
let player: Player | undefined;
const playerGroup = new Group();
scene.add(playerGroup);
// The cell under the pointer while a ground skill waits for a target.
const cellMarker = new Mesh(
  new PlaneGeometry(1, 1),
  new MeshBasicMaterial({ color: 0x66ccff, transparent: true, opacity: 0.45, depthWrite: false }),
);
cellMarker.rotation.x = -Math.PI / 2;
cellMarker.visible = false;
cellMarker.renderOrder = 1;
scene.add(cellMarker);

const audio = new AudioSystem(store);
let soundscape: Soundscape | undefined;
let statusWindow: StatusWindow | undefined;
let statusIcons: StatusIconBar | undefined;
let inventoryWindow: InventoryWindow | undefined;
let equipmentWindow: EquipmentWindow | undefined;
let dialogWindow: DialogWindow | undefined;
let shopWindow: ShopWindow | undefined;
let mailWindow: MailWindow | undefined;
let vendingOwnWindow: VendingOwnWindow | undefined;
let buyingOwnWindow: BuyingOwnWindow | undefined;
/** The player's skills, and the skill waiting for a target. */
const skills = new SkillModel();
const targetMode = new TargetMode();
const cooldowns = new Cooldowns();
const castBars = new CastBars();
let skillWindow: SkillWindow | undefined;
let hotkeyBar: HotkeyBar | undefined;
/** The character whose hotkeys are loaded, once its name is known. */
let hotkeysOf: string | undefined;
/** Casts a skill from the skill window or a hotkey, at a level or the learned one. */
let castSkill: (skillId: number, level: number | undefined) => void = () => {};
/** A progress bar over the player: when it started and how long it lasts. */
let progressBar: { start: number; ms: number } | undefined;
/** Shows a short message to the player, such as why an action failed. */
let showMessage: (text: string) => void = () => {};
/** Shows the touch cancel button while a skill waits, once it exists. */
let showTargetCancel: (on: boolean) => void = () => {};
/** Opens or closes the chat of touch screens, once the chat exists. */
let openChat: () => void = () => {};
/** Adds a line from the game to the chat log, once the log exists. */
let logNotice: (text: string) => void = () => {};
/** The ground item under the pointer, whose name shows. */
let hoveredItem: number | undefined;
/** Guild emblems, once a game server is known. */
let emblems: EmblemCache | undefined;
/**
 * Opens the menu of another player at a point of the page, once the chat
 * exists. Returns false for an entity that is not another player.
 */
let openPlayerMenu: ((id: number, x: number, y: number) => boolean) | undefined;
const bubbles = new Bubbles();
const bubbleRoot = document.querySelector<HTMLDivElement>('#bubbles');
const bubbleViews = new Map<number, HTMLDivElement>();

// Browsers allow audio only after a click.
soundButton.addEventListener('click', () => {
  (audio.on ? audio.suspend() : audio.resume())
    .then(() => {
      soundButton.textContent = audio.on ? 'Sound on' : 'Sound off';
    })
    .catch(reportError);
});

/** Sounds from sprites carry this far, in cells. */
const SPRITE_SOUND_RANGE = 20;
let walkers: Walkers | undefined;
const sprites = new SpriteCache(store);

// Sprites to scatter on the map, as asset names without "sprite/".
// ?server= connects to a game server, such as ws://localhost:8300/ws, with
// a game ticket from the account service (?account=, by default the game
// server's /account/), or as ?name= on a --dev server. A production build
// without ?server= plays on its own origin's /ws (gameServerURL). The
// server then chooses the map and moves every character, and the demo
// sprites are off unless asked for.
const params = new URL(location.href).searchParams;
const serverURL = gameServerURL(params, location.href, import.meta.env.PROD);
const online = serverURL !== null;
const DEFAULT_SPRITES = online ? '' : 'mob/1002,mob/1063,mob/1007,mob/1113,mob/1008,npc/117';
// Effects to play in a row near the center, such as effect/firewall.
const effectNames = (new URL(location.href).searchParams.get('effect') ?? '')
  .split(',')
  .filter((n) => n !== '');
const effects = new EffectCache(store);
let playing: EffectView[] = [];
const effectGroup = new Group();
scene.add(effectGroup);

// Dropped items, as item IDs. Their sprites are sprite/item/<id>.
const DEFAULT_DROPS = online
  ? ''
  : '501,502,503,504,505,506,507,508,512,601,602,909,914,938,1201,2301,4001';
const dropIDs = (new URL(location.href).searchParams.get('drops') ?? DEFAULT_DROPS)
  .split(',')
  .filter((n) => /^\d+$/.test(n));

const characterCount = Number(params.get('characters') ?? (online ? 0 : 30));
const spriteNames = (new URL(location.href).searchParams.get('sprites') ?? DEFAULT_SPRITES)
  .split(',')
  .filter((n) => n !== '')
  .map((n) => `sprite/${n}`);
let loading = 0;

// The default buffer holds 250 entries, fewer than a map's files.
performance.setResourceTimingBufferSize(100_000);

/** Bytes received for asset files since a time, after compression. */
function transferred(since: number): number {
  let total = 0;
  for (const e of performance.getEntriesByType('resource')) {
    if (e instanceof PerformanceResourceTiming && e.startTime >= since && e.name.includes(base)) {
      total += e.transferSize;
    }
  }
  return total;
}

function mb(bytes: number): string {
  return `${(bytes / 1e6).toFixed(1)} MB`;
}

/**
 * Loads a map and shows it. at is the cell the camera starts on; without
 * it, the camera starts on the player or the map's center. It resolves
 * true once the map is ready, or false when a newer load replaced it or
 * it failed.
 */
async function show(name: string, at?: [number, number]): Promise<boolean> {
  const id = ++loading;
  const start = performance.now();
  const workerStart = workerBytes;
  const before = { ...store.stats };
  const progress = (label: string): void => {
    if (id !== loading) {
      return;
    }
    const files = store.stats.files - before.files;
    const bytes = store.stats.bytes - before.bytes;
    const seconds = ((performance.now() - start) / 1000).toFixed(1);
    status!.value = `${label}: ${files} files, ${mb(transferred(start) + workerBytes - workerStart)} sent (${mb(bytes)} decoded), ${seconds} s`;
  };
  progress('loading');
  const timer = setInterval(() => {
    progress('loading');
  }, 200);
  try {
    const { scene: next, ready } = await MapScene.load(
      store,
      name.includes('/') ? name : `map/${name}`,
    );
    if (id !== loading) {
      return false;
    }
    if (current !== undefined) {
      scene.remove(current.group);
    }
    if (walkers !== undefined) {
      scene.remove(walkers.group);
    }
    current = next;
    effectGroup.clear();
    playing = [];
    const views = await Promise.all(effectNames.map(async (n) => effects.get(n)));
    if (id !== loading) {
      return false;
    }
    views.forEach((effect, i) => {
      if (effect === undefined) {
        return;
      }
      const v = new EffectView(effect, next.textures);
      const x = next.map.width / 2 + (i - (views.length - 1) / 2) * 6;
      const y = next.map.height / 2;
      v.group.position.set(x, cellHeight(next.map, x, y), -y);
      effectGroup.add(v.group);
      playing.push(v);
    });
    walkers = new Walkers(next.map);
    walkers.onSound = (sound, position) => {
      const d = position.distanceTo(controls.target);
      if (d < SPRITE_SOUND_RANGE) {
        audio.play(sound, 1 - d / SPRITE_SOUND_RANGE, pan(position));
      }
    };
    soundscape = new Soundscape(next.map.sounds, audio);
    audio.playMusic(next.map.music).catch(reportError);
    scene.add(next.group, walkers.group);
    // The player starts at the walkable cell nearest the center, and the
    // camera looks at it from the south, as RO's camera does.
    pathfinder.setMap(next.map);
    player = undefined;
    playerGroup.clear();
    if (session !== undefined) {
      session.online.effects = { cache: effects, textures: next.textures };
      session.online.setMap(next.map);
    }
    // Online, the player's own cell comes in the first frame, which
    // setMap has just shown.
    const [px, py] = at ?? session?.online.selfCell ?? Player.start(next.map);
    const cx = px + 0.5;
    const cz = -(py + 0.5);
    const size = Math.max(next.map.width, next.map.height);
    // Online, the server's events place every character.
    (session === undefined
      ? Character.load(sprites, {
          body: 'sprite/body/novice/f',
          head: 'sprite/head/human/f/2',
          hairPalette: 'palette/hair/human/f/2/1',
        })
      : Promise.resolve(undefined)
    )
      .then((c) => {
        if (c !== undefined && current === next) {
          player = new Player(c, next.map, pathfinder, px, py);
          playerGroup.add(c.group);
        }
      })
      .catch(reportError);
    // ?distance= sets the camera's distance from the center, in cells.
    const distance = Number(new URL(location.href).searchParams.get('distance')) || size * 0.85;
    const y = cellHeight(next.map, cx, -cz);
    controls.target.set(cx, y, cz);
    camera.position.set(cx, y + distance * 0.7, cz + distance * 0.7);
    controls.update();
    // A fixed generator gives the same characters on every load.
    let seed = 7;
    const random = (n: number): number => {
      seed = (seed * 1103515245 + 12345) % 2 ** 31;
      return seed % n;
    };
    await Promise.all([
      ready,
      walkers.add(
        async (i) => {
          const asset = await sprites.get(
            spriteNames[i % spriteNames.length] ?? 'placeholder/sprite',
          );
          return asset === undefined ? undefined : new SpriteView(asset);
        },
        spriteNames.length > 0 ? 20 : 0,
      ),
      walkers.add(async () => Character.load(sprites, randomLook(random)), characterCount),
      walkers.add(
        async (i) => {
          const asset = await sprites.get(`sprite/item/${dropIDs[i % dropIDs.length] ?? 0}`);
          return asset === undefined ? undefined : new SpriteView(asset);
        },
        dropIDs.length > 0 ? 40 : 0,
        true,
      ),
    ]);
    progress('done');
    return id === loading;
  } catch (error) {
    status!.value = String(error);
    return false;
  } finally {
    clearInterval(timer);
  }
}

/** A map load after a warp, for the budget of strategy section 3.8. */
interface MapLoad {
  map: string;
  /** Milliseconds from MapChange to the map being ready. */
  ms: number;
  /** Asset files and decoded bytes the load read. */
  files: number;
  bytes: number;
}
const mapLoads: MapLoad[] = [];
/** Set while a map change loads. Clicks do nothing then. */
let changingMap = false;

form.addEventListener('submit', (e) => {
  e.preventDefault();
  const name = input.value.trim().toLowerCase();
  if (name !== '') {
    const url = new URL(location.href);
    url.searchParams.set('map', name);
    history.replaceState(null, '', url);
    show(name).catch(reportError);
  }
});

if (serverURL === null) {
  input.value = params.get('map') ?? 'prontera';
  show(input.value).catch(reportError);
} else {
  // ?name= plays the character of that name on a --dev server. Without
  // it, the account service gives a one-time game ticket for the
  // character the player chose there.
  const devName = params.get('name');
  const account = devName === null ? accountURL(serverURL, params.get('account')) : undefined;
  let token = devName ?? '';
  if (account !== undefined) {
    const got = await fetchTicket(account, location.href);
    if (got.kind !== 'ticket') {
      if (got.kind === 'redirect') {
        location.assign(got.url);
      } else {
        status.value = got.message;
      }
      // Nothing more runs on this page.
      await new Promise(() => {});
      throw new Error('unreachable');
    }
    token = got.ticket;
    const back = document.querySelector<HTMLButtonElement>('#characters');
    if (back !== null) {
      back.hidden = false;
      back.addEventListener('click', () => {
        location.assign(accountPage(account, 'characters', location.href));
      });
    }
  }
  // The server's client data comes before the connection, so the first
  // entities already find their mods' art: core's manifest is open, and
  // each active mod's build loads now (decision 0033). Without it, mod
  // records show placeholders.
  try {
    setArt(await loadClientData(serverURL, store));
  } catch (error) {
    console.error('client data:', error);
  }
  // Events can follow Welcome at once, so the view exists first. It learns
  // the player's own ID from Welcome.
  const view = new Online(0, sprites, pathfinder);
  const connection = await Connection.open(serverURL, token, {
    events: (events) => {
      view.handle(events);
    },
    closed: (message, reason) => {
      status.value = disconnectText(message, reason);
      // Status icons belong to the session that ended.
      statusIcons?.clear();
      if (account === undefined) {
        return;
      }
      // Back to the character list, which sends a player whose login
      // ended on to the login page. disconnectDelay gives the time to
      // read the message.
      const back = accountPage(account, 'characters', location.href);
      setTimeout(() => {
        location.assign(back);
      }, disconnectDelay(reason));
    },
  });
  view.self = connection.welcome.netId;
  // World events: a banner for each start and end, and a panel of the
  // runs that last, on the server's clock.
  const eventBanner = document.querySelector<HTMLElement>('#event-banner');
  const eventPanel = document.querySelector<HTMLElement>('#world-events');
  if (eventBanner !== null && eventPanel !== null) {
    const offset = Number(connection.welcome.serverTimeMs) - Date.now();
    const worldEvents = new WorldEventsView(eventBanner, eventPanel, () => Date.now() + offset);
    view.onWorldEventList = (runs) => {
      worldEvents.set(runs);
    };
    view.onWorldEventNotice = (run, started, result) => {
      logNotice(worldEvents.notice(run, started, result));
    };
    setInterval(() => {
      worldEvents.tick();
    }, 1000);
  }
  const emblemCache = new EmblemCache(serverURL);
  emblems = emblemCache;
  const hits = document.querySelector<HTMLDivElement>('#hits');
  view.onHit = (h) => {
    if (hits === null) {
      return;
    }
    const p = h.position.clone();
    p.y += 2;
    p.project(camera);
    const div = document.createElement('div');
    const heal = h.kind === DamageKind.HEAL;
    div.className = heal
      ? 'hit heal'
      : `hit${h.mine ? ' mine' : ''}${h.kind === DamageKind.CRITICAL ? ' critical' : ''}${h.amount === 0 ? ' miss' : ''}${h.combo === true ? ' combo' : ''}`;
    div.textContent = h.amount === 0 && !heal ? 'Miss' : String(h.amount);
    div.style.left = `${((p.x + 1) / 2) * 100}%`;
    div.style.top = `${((1 - p.y) / 2) * 100}%`;
    hits.append(div);
    setTimeout(() => {
      div.remove();
    }, 1000);
  };
  const message = document.querySelector<HTMLOutputElement>('#message');
  let messageTimer: ReturnType<typeof setTimeout> | undefined;
  showMessage = (text) => {
    if (message === null) {
      return;
    }
    message.value = text;
    clearTimeout(messageTimer);
    messageTimer = setTimeout(() => {
      message.value = '';
    }, 3000);
  };
  view.onPickUpFailed = (reason) => {
    showMessage(pickUpMessage(reason));
  };
  view.onUseItemFailed = (reason) => {
    showMessage(useItemMessage(reason));
  };
  view.onDropItemFailed = (reason) => {
    showMessage(dropItemMessage(reason));
  };
  // The inventory window opens with its button or Alt+E, as in RO.
  const inventory = new InventoryModel();
  // A long inventory comes in pages: the first replaces the list, and
  // the pages with more set add to it.
  view.onInventory = (items, size, more) => {
    if (more) {
      inventory.addPage(items);
    } else {
      inventory.reset(items, size);
    }
  };
  // Item lines wait for the names, so the first one is not shown by ID.
  const logItem = (text: () => string): void => {
    itemTexts
      .loadNames()
      .then(() => {
        logNotice(text());
      })
      .catch(reportError);
  };
  view.onInventoryAdd = (slot, id, amount, details) => {
    inventory.add(slot, id, amount, details);
    logItem(() => gotItemMessage(itemTexts.label(id), amount));
  };
  view.onItemEquipped = (slot, position) => {
    inventory.equipped(slot, position);
  };
  view.onItemUnequipped = (slot) => {
    inventory.unequipped(slot);
  };
  view.onCardInserted = (_cardSlot, equipSlot, cardId) => {
    inventory.cardInserted(equipSlot, cardId);
  };
  view.onItemRollsChange = (slot, rolls) => {
    inventory.rollsChanged(slot, rolls);
  };
  // Equipment and card failures go to the chat log as notices.
  view.onEquipFailed = (_slot, reason) => {
    logNotice(equipMessage(reason));
  };
  view.onInsertCardFailed = () => {
    logNotice(INSERT_CARD_FAILED);
  };
  view.onInventoryRemove = (slot, amount, reason) => {
    const id = inventory.get(slot)?.itemId;
    inventory.remove(slot, amount);
    if (reason === InventoryRemoveReason.USED && id !== undefined) {
      logItem(() => usedItemMessage(itemTexts.label(id)));
    }
  };
  const inventoryRoot = document.querySelector<HTMLElement>('#inventory-window');
  const inventoryToggle = document.querySelector<HTMLButtonElement>('#inventory-toggle');
  if (inventoryRoot !== null) {
    inventoryWindow = new InventoryWindow(
      inventoryRoot,
      store,
      inventory,
      {
        use: (slot) => {
          connection.useItem(slot);
        },
        drop: (slot, amount) => {
          connection.dropItem(slot, amount);
        },
        equip: (slot) => {
          connection.equipItem(slot);
        },
        insertCard: (cardSlot, equipSlot) => {
          connection.insertCard(cardSlot, equipSlot);
        },
        message: (text) => {
          showMessage(text);
        },
      },
      itemTexts,
    );
    const toggle = (): void => {
      inventoryWindow?.toggle();
      inventoryWindow?.update(view.stats);
    };
    if (inventoryToggle !== null) {
      inventoryToggle.hidden = false;
      inventoryToggle.addEventListener('click', toggle);
    }
    addEventListener('keydown', (e) => {
      if (isTyping(e.target)) {
        return;
      }
      if (e.altKey && !e.ctrlKey && !e.metaKey && e.code === 'KeyE') {
        e.preventDefault();
        toggle();
      }
    });
    // A row dragged onto the game view drops the item.
    canvas.addEventListener('dragover', (e) => {
      if (e.dataTransfer?.types.includes(SLOT_DRAG_TYPE) === true) {
        e.preventDefault();
        e.dataTransfer.dropEffect = 'move';
      }
    });
    canvas.addEventListener('drop', (e) => {
      const slot = Number(e.dataTransfer?.getData(SLOT_DRAG_TYPE) ?? '');
      if (e.dataTransfer?.types.includes(SLOT_DRAG_TYPE) === true && Number.isInteger(slot)) {
        e.preventDefault();
        inventoryWindow?.requestDrop(slot);
      }
    });
  }
  // The equipment window opens with its button or Alt+Q, as in RO.
  // Clicking a worn item takes it off.
  const equipmentRoot = document.querySelector<HTMLElement>('#equipment-window');
  const equipmentToggle = document.querySelector<HTMLButtonElement>('#equipment-toggle');
  if (equipmentRoot !== null) {
    equipmentWindow = new EquipmentWindow(
      equipmentRoot,
      inventory,
      {
        unequip: (slot) => {
          connection.unequipItem(slot);
        },
      },
      itemTexts,
    );
    const toggle = (): void => {
      equipmentWindow?.toggle();
      equipmentWindow?.update();
    };
    if (equipmentToggle !== null) {
      equipmentToggle.hidden = false;
      equipmentToggle.addEventListener('click', toggle);
    }
    addEventListener('keydown', (e) => {
      if (isTyping(e.target)) {
        return;
      }
      if (e.altKey && !e.ctrlKey && !e.metaKey && e.code === 'KeyQ') {
        e.preventDefault();
        toggle();
      }
    });
  }
  // Prompts that ask the player to accept or decline, such as a party
  // invite or a trade request.
  const promptRoot = document.querySelector<HTMLElement>('#social-prompts');
  const prompts = promptRoot === null ? undefined : new Prompts(promptRoot);
  const itemIcons = inventoryWindow?.icons ?? new ItemIcons(store);
  // Storage and cart. The storage window opens when the server sends
  // StorageOpen, such as from a storage NPC. The cart window opens with
  // its button or Alt+W while the player has a cart. Items move between
  // the inventory, the cart and the open storage.
  const storage = new StorageModel();
  const cart = new StorageModel();
  const storageRoot = document.querySelector<HTMLElement>('#storage-window');
  const cartRoot = document.querySelector<HTMLElement>('#cart-window');
  const cartToggle = document.querySelector<HTMLButtonElement>('#cart-toggle');
  let storageWindow: StorageWindow | undefined;
  let cartWindow: StorageWindow | undefined;
  if (storageRoot !== null) {
    storageWindow = new StorageWindow(
      storageRoot,
      storage,
      (slot) => inventory.get(slot),
      {
        put: (slot, amount) => {
          connection.storagePut(slot, amount);
        },
        take: (slot, amount) => {
          connection.storageTake(slot, amount);
        },
        send: (slot, amount) => {
          connection.storageToCart(slot, amount);
        },
        close: () => {
          storage.close();
          connection.storageClose();
          cartWindow?.render();
        },
        log: () => {
          connection.storageLog();
        },
      },
      itemTexts,
      itemIcons,
      { dragType: STORAGE_DRAG_TYPE, cart: false, canSend: () => cart.isOpen },
    );
  }
  if (cartRoot !== null) {
    cartWindow = new StorageWindow(
      cartRoot,
      cart,
      (slot) => inventory.get(slot),
      {
        put: (slot, amount) => {
          connection.cartPut(slot, amount);
        },
        take: (slot, amount) => {
          connection.cartTake(slot, amount);
        },
        send: (slot, amount) => {
          connection.cartToStorage(slot, amount);
        },
        // The cart stays with the player. Closing only hides the window.
        close: () => {},
        log: () => {},
      },
      itemTexts,
      itemIcons,
      { dragType: CART_DRAG_TYPE, cart: true, canSend: () => storage.isOpen },
    );
    const toggle = (): void => {
      if (cart.isOpen) {
        cartWindow?.toggle();
      }
    };
    cartToggle?.addEventListener('click', toggle);
    addEventListener('keydown', (e) => {
      if (isTyping(e.target)) {
        return;
      }
      // e.code, since Alt changes e.key on some layouts, such as macOS.
      if (e.altKey && !e.ctrlKey && !e.metaKey && e.code === 'KeyW') {
        e.preventDefault();
        toggle();
      }
    });
  }
  // Cart rows dropped on the storage window go into the storage, and
  // storage rows dropped on the cart window go into the cart.
  storageWindow?.acceptDrop(CART_DRAG_TYPE, (slot) => {
    cartWindow?.requestSend(slot);
  });
  cartWindow?.acceptDrop(STORAGE_DRAG_TYPE, (slot) => {
    storageWindow?.requestSend(slot);
  });
  const storageOf = (kind: StorageKind): [StorageModel, StorageWindow | undefined] =>
    kind === StorageKind.CART ? [cart, cartWindow] : [storage, storageWindow];
  /** Draws both windows, since a change to one shows or hides the other's send buttons. */
  const renderStorage = (): void => {
    storageWindow?.render();
    cartWindow?.render();
  };
  // The player's limits size the inventory, the account's storage and the
  // cart. A script may expand them (review decision 32).
  view.onPlayerLimits = (limits) => {
    if (applyPlayerLimits(limits, inventory, storage, cart)) {
      renderStorage();
    }
  };
  view.onStorageOpen = (info) => {
    if (info.kind === StorageKind.CART) {
      cart.open(info);
      if (cartToggle !== null) {
        cartToggle.hidden = false;
      }
      itemTexts.loadNames().then(renderStorage).catch(reportError);
      return;
    }
    storage.open(info);
    itemTexts
      .loadNames()
      .then(() => {
        storageWindow?.show();
        cartWindow?.render();
        if (inventoryWindow !== undefined && !inventoryWindow.open) {
          inventoryWindow.toggle();
          inventoryWindow.update(view.stats);
        }
      })
      .catch(reportError);
  };
  view.onStorageItems = (kind, items, page) => {
    const [model, win] = storageOf(kind);
    if (model.addPage(kind, page, items)) {
      win?.render();
    }
  };
  view.onStorageAdd = (kind, item, weight) => {
    const [model, win] = storageOf(kind);
    model.add(item, weight);
    win?.render();
  };
  view.onStorageRemove = (kind, slot, amount, weight) => {
    const [model, win] = storageOf(kind);
    model.remove(slot, amount, weight);
    win?.render();
  };
  view.onStorageClose = (kind) => {
    const [model, win] = storageOf(kind);
    model.close();
    win?.hide();
    if (kind === StorageKind.CART && cartToggle !== null) {
      cartToggle.hidden = true;
    }
    renderStorage();
  };
  view.onStorageResult = (outcome) => {
    logNotice(storageMessage(outcome));
  };
  view.onGuildStorageLog = (entries) => {
    itemTexts
      .loadNames()
      .then(() => {
        storageWindow?.showLog(entries);
      })
      .catch(reportError);
  };
  // Trade. A trade request shows a prompt, and the trade window opens
  // when both players agree.
  let trade = NO_TRADE;
  let tradeWindow: TradeWindow | undefined;
  const setTrade = (e: TradeEvent): void => {
    trade = tradeReducer(trade, e);
    tradeWindow?.render();
  };
  const tradeRoot = document.querySelector<HTMLElement>('#trade-window');
  if (tradeRoot !== null) {
    tradeWindow = new TradeWindow(
      tradeRoot,
      {
        state: () => trade,
        inventory: (slot) => inventory.get(slot),
        zeny: () => Number(view.stats.get(StatId.ZENY) ?? 0n),
      },
      {
        addItem: (slot, amount) => {
          connection.tradeAddItem(slot, amount);
        },
        addZeny: (zeny) => {
          connection.tradeAddZeny(zeny);
        },
        lock: () => {
          connection.tradeLock();
        },
        confirm: () => {
          setTrade({ type: 'confirm' });
          connection.tradeConfirm();
        },
        cancel: () => {
          connection.tradeCancel();
        },
        message: (text) => {
          showMessage(text);
        },
      },
      itemTexts,
      itemIcons,
    );
  }
  view.onTradeRequest = (from, baseLevel) => {
    const answer = (accept: boolean): void => {
      connection.tradeRespond(accept);
    };
    if (prompts === undefined) {
      answer(false);
      return;
    }
    prompts.show({
      key: 'trade',
      text: tradeRequestText(from, baseLevel),
      accept: () => {
        answer(true);
      },
      decline: () => {
        answer(false);
      },
    });
  };
  view.onTradeOpen = (partner, baseLevel) => {
    prompts?.close('trade');
    setTrade({ type: 'open', partner, baseLevel });
    // Offered items show by name once the names load.
    itemTexts.loadNames().catch(reportError);
    if (inventoryWindow !== undefined && !inventoryWindow.open) {
      inventoryWindow.toggle();
      inventoryWindow.update(view.stats);
    }
  };
  view.onTradeOffer = (mine, offer) => {
    setTrade({ type: 'offer', mine, offer });
  };
  view.onTradeLock = (mine) => {
    setTrade({ type: 'lock', mine });
  };
  view.onTradeClose = (outcome) => {
    prompts?.close('trade');
    setTrade({ type: 'close' });
    logNotice(tradeMessage(outcome));
  };
  view.onTradeResult = (outcome, name, _slot, reason) => {
    setTrade({ type: 'result', outcome });
    logNotice(tradeMessage(outcome, name, reason));
  };
  if (inventoryWindow !== undefined && inventoryRoot !== null) {
    // A right click on an inventory row offers it in the trade, or puts it
    // into the open storage, or into the open cart window.
    inventoryWindow.onRowContext = (slot) => {
      if (mailWindow?.writing === true) {
        mailWindow.requestAttach(slot);
        return true;
      }
      if (trade.open && tradeWindow !== undefined) {
        tradeWindow.requestAdd(slot);
        return true;
      }
      if (storageWindow?.open === true) {
        storageWindow.requestPut(slot);
        return true;
      }
      if (cartWindow?.open === true) {
        cartWindow.requestPut(slot);
        return true;
      }
      return false;
    };
    // A storage or cart row dropped on the inventory window takes the item out.
    for (const [type, take] of [
      [STORAGE_DRAG_TYPE, (slot: number) => storageWindow?.requestTake(slot)],
      [CART_DRAG_TYPE, (slot: number) => cartWindow?.requestTake(slot)],
    ] as const) {
      inventoryRoot.addEventListener('dragover', (e) => {
        if (e.dataTransfer?.types.includes(type) === true) {
          e.preventDefault();
          e.dataTransfer.dropEffect = 'move';
        }
      });
      inventoryRoot.addEventListener('drop', (e) => {
        if (e.dataTransfer?.types.includes(type) !== true) {
          return;
        }
        const slot = Number(e.dataTransfer.getData(type));
        if (Number.isInteger(slot)) {
          e.preventDefault();
          take(slot);
        }
      });
    }
  }
  // The player's status icons. They last until the server hides them, and
  // the server sends them again after a map change.
  const icons = new StatusIconBar(document.body, store);
  statusIcons = icons;
  view.onStatusIcon = (icon, on, remainingMs) => {
    icons.set(icon, on, remainingMs);
  };
  view.onSound = (sound, position) => {
    const d = position.distanceTo(controls.target);
    if (d < SPRITE_SOUND_RANGE) {
      audio.play(sound, 1 - d / SPRITE_SOUND_RANGE, pan(position));
    }
  };
  // The status window opens with its button or Alt+A, as in RO. Its "+"
  // buttons raise a stat by 1.
  const statusRoot = document.querySelector<HTMLElement>('#status-window');
  const statusToggle = document.querySelector<HTMLButtonElement>('#status-toggle');
  if (statusRoot !== null) {
    statusWindow = new StatusWindow(statusRoot, (stat) => {
      connection.raiseStat(stat);
    });
    const toggle = (): void => {
      statusWindow?.toggle();
      statusWindow?.update(view.stats);
    };
    if (statusToggle !== null) {
      statusToggle.hidden = false;
      statusToggle.addEventListener('click', toggle);
    }
    addEventListener('keydown', (e) => {
      if (isTyping(e.target)) {
        return;
      }
      // e.code, since Alt changes e.key on some layouts, such as macOS.
      if (e.altKey && !e.ctrlKey && !e.metaKey && e.code === 'KeyA') {
        e.preventDefault();
        toggle();
      }
    });
  }
  // Skills. The skill window opens with its button or Alt+S, as in RO.
  // F1 to F9 cast the skills on the hotkey bar. A skill that needs a
  // target waits for a click (see the click handler below).
  view.skillIcon = (id) => skills.iconOf(id);
  view.onSkillList = (list) => {
    skills.reset(list.map(skillOf));
  };
  view.onSkillFailed = (_skillId, reason) => {
    logNotice(skillFailureMessage(reason));
  };
  view.onSkillCooldown = (skillId, ms) => {
    cooldowns.set(skillId, ms, performance.now());
  };
  view.onSkillCast = (source, _skillId, castMs) => {
    castBars.start(source, castMs, performance.now());
  };
  view.onSkillCastCancel = (source) => {
    castBars.cancel(source);
  };
  castSkill = (skillId, level) => {
    const s = skills.get(skillId);
    if (s === undefined || s.level === 0) {
      logNotice('You have not learned this skill.');
      return;
    }
    if (!castable(s)) {
      logNotice('This skill is passive.');
      return;
    }
    const request = targetMode.begin(s.id, castLevel(s, level), s.target);
    if (request !== undefined) {
      connection.useSkill(request.skillId, request.level, request.target);
    }
    showTargeting();
  };
  const hotkeys = new HotkeyBar(document.body, {
    use: (slot) => {
      const h = hotkeys.get(slot);
      if (h !== undefined) {
        castSkill(h.skillId, h.level);
      }
    },
    changed: (slots) => {
      if (hotkeysOf !== undefined) {
        saveHotkeys(() => localStorage, hotkeysOf, slots);
      }
    },
  });
  hotkeyBar = hotkeys;
  hotkeys.root.hidden = false;
  const skillRoot = document.querySelector<HTMLElement>('#skill-window');
  const skillToggle = document.querySelector<HTMLButtonElement>('#skill-toggle');
  if (skillRoot !== null) {
    skillWindow = new SkillWindow(skillRoot, skills, {
      learn: (skillId) => {
        connection.learnSkill(skillId);
      },
      use: (skillId, level) => {
        castSkill(skillId, level);
      },
      assign: (skillId, level) => {
        const slot = hotkeys.firstEmpty();
        if (slot === undefined) {
          showMessage('The hotkey bar is full. Right-click a key to clear it.');
          return;
        }
        hotkeys.assign(slot, level === undefined ? { skillId } : { skillId, level });
      },
    });
    const toggle = (): void => {
      skillWindow?.toggle();
      skillWindow?.update(view.stats, (id) => cooldowns.remaining(id, performance.now()));
    };
    if (skillToggle !== null) {
      skillToggle.hidden = false;
      skillToggle.addEventListener('click', toggle);
    }
    addEventListener('keydown', (e) => {
      if (isTyping(e.target)) {
        return;
      }
      if (e.altKey && !e.ctrlKey && !e.metaKey && e.code === 'KeyS') {
        e.preventDefault();
        toggle();
      }
    });
  }
  addEventListener('keydown', (e) => {
    if (isTyping(e.target) || e.altKey || e.ctrlKey || e.metaKey) {
      return;
    }
    const slot = hotkeySlot(e.code);
    if (slot !== undefined) {
      // F1 to F9 would open help or reload the page.
      e.preventDefault();
      if (!e.repeat) {
        hotkeys.use(slot);
      }
      return;
    }
    if (e.key === 'Escape' && targetMode.cancel()) {
      e.preventDefault();
      showTargeting();
    }
  });
  // Chat. Enter opens the input, Enter sends the line, and Escape
  // leaves the input. Local lines show in the log and above the speaker.
  // The tabs above the log pick where the input sends a line.
  const chatRoot = document.querySelector<HTMLElement>('#chat');
  const chatList = document.querySelector<HTMLOListElement>('#chat-log');
  const chatInput = document.querySelector<HTMLInputElement>('#chat-input');
  const chatTabsRoot = document.querySelector<HTMLElement>('#chat-tabs');
  if (chatRoot !== null && chatList !== null && chatInput !== null && chatTabsRoot !== null) {
    chatRoot.hidden = false;
    const log = new ChatLog(chatList);
    logNotice = (text) => {
      log.notice(text);
    };
    const rate = new ChatRate(performance.now());
    const tabs = new ChatTabs();
    const tabBar = new ChatTabBar(chatTabsRoot, tabs);
    const showTab = (key: string): void => {
      if (!tabs.activate(key)) {
        return;
      }
      log.setTab(tabs.active);
      chatInput.placeholder = placeholder(tabs.target());
      tabBar.render();
    };
    tabBar.onSelect = (key) => {
      showTab(key);
      chatInput.focus();
    };
    tabBar.onClose = (tab) => {
      if (tab.kind === 'channel') {
        // The tab goes when the server says the player left.
        connection.channel(ChannelAction.LEAVE, tab.name);
        return;
      }
      tabs.close(tab.key);
      showTab(tabs.active);
    };
    tabBar.render();
    /** Marks a tab unread when a line comes to it while it is hidden. */
    const unread = (key: string): void => {
      if (tabs.markUnread(key)) {
        tabBar.render();
      }
    };
    /** The player's mute, as the last SanctionNotice gave it. */
    let mute: Sanction | undefined;
    log.onReport = (name, lineId) => {
      connection.report(name, lineId, '');
    };
    view.onChat = (from, speaker, text, color, lineId) => {
      log.add(speaker, text, from === view.self, color, lineId);
      bubbles.say(from, chatLine(speaker, text), performance.now());
    };
    // Server lines and announcements, such as a world event's boss that
    // appears, show in every tab.
    view.onServerText = (text) => {
      log.server(text);
    };
    view.onAnnouncement = (text, color) => {
      log.announce(text, color);
    };
    view.onChannelChat = (channel, speaker, text, color, lineId) => {
      const tab = tabs.channel(channel);
      const key = tab?.key ?? '';
      log.channel(
        key,
        tab?.alias ?? `[${channel}]`,
        speaker,
        text,
        color === 0 ? (tab?.color ?? 0) : color,
        speaker === view.selfName,
        lineId,
      );
      unread(key);
    };
    view.onWhisper = (from, text, lineId) => {
      const key = tabs.openWhisper(from);
      tabBar.render();
      log.whisper(key, true, from, text, lineId);
      unread(key);
    };
    view.onWhisperResult = (r) => {
      const failed = whisperMessage(r.to, r.outcome);
      if (failed !== undefined) {
        log.notice(failed, tabs.get(whisperKey(r.to)) === undefined ? '' : whisperKey(r.to));
        return;
      }
      const key = tabs.openWhisper(r.to);
      tabBar.render();
      log.whisper(key, false, r.to, r.text);
    };
    view.onChatRefused = (r) => {
      log.notice(refusalMessage(r.reason, r.to, mute));
    };
    view.onSanction = (n) => {
      const s: Sanction = {
        kind: n.kind,
        reason: n.reason,
        endsAtMs: Number(n.endsAtMs),
        ended: n.ended,
      };
      if (s.kind === SanctionKind.MUTE) {
        mute = s.ended ? undefined : s;
      }
      log.notice(sanctionMessage(s));
    };
    view.onChannelJoined = (c) => {
      tabs.joinChannel(c.name, c.displayName, c.color, c.canLeave);
      tabBar.render();
      log.notice(`You joined ${c.name}.`);
    };
    view.onChannelLeft = (name) => {
      tabs.leaveChannel(name);
      showTab(tabs.active);
      log.notice(`You left ${name}.`);
    };
    view.onChannelList = (channels) => {
      log.notice(channelListMessage(channels));
    };
    view.onChannelMembers = (name, names) => {
      log.notice(membersMessage(name, names));
    };
    view.onChannelFailed = (name, reason) => {
      log.notice(channelFailureMessage(name, reason));
    };
    view.onReportResult = (outcome) => {
      log.notice(reportMessage(outcome));
    };
    // The block list panel opens with the Blocked button.
    const blockRoot = document.querySelector<HTMLElement>('#block-panel');
    const blockToggle = document.querySelector<HTMLButtonElement>('#block-toggle');
    if (blockRoot !== null && blockToggle !== null) {
      const panel = new BlockPanel(
        blockRoot,
        {
          block: (name) => {
            connection.block(name);
          },
          unblock: (name) => {
            connection.block(name, true);
          },
        },
        connection.limits.blockList,
      );
      blockToggle.addEventListener('click', () => {
        panel.toggle();
      });
      // The first list comes after login, and its names are not news.
      let blocked: string[] | undefined;
      view.onBlockList = (names, full) => {
        panel.show(names, full);
        if (blocked !== undefined) {
          for (const text of blockChanges(blocked, names)) {
            log.notice(text);
          }
        }
        if (full) {
          log.notice(`Your block list is full (${String(connection.limits.blockList)} names).`);
        }
        blocked = names;
      };
    }
    /** Opens the whisper tab of a character and puts the cursor in the input. */
    const startWhisper = (name: string): void => {
      tabs.openWhisper(name);
      showTab(whisperKey(name));
      chatInput.focus();
    };
    /** A shortcut key with Alt, as RO's window keys. e.code, since Alt changes e.key on macOS. */
    const altKey = (code: string, action: () => void): void => {
      addEventListener('keydown', (e) => {
        if (isTyping(e.target)) {
          return;
        }
        if (e.altKey && !e.ctrlKey && !e.metaKey && e.code === code) {
          e.preventDefault();
          action();
        }
      });
    };
    // Parties. The party window opens with its button or Alt+Z, as in RO.
    // The Party tab shows while the player has a party.
    let party = NO_PARTY;
    const partyRoot = document.querySelector<HTMLElement>('#party-window');
    const partyToggle = document.querySelector<HTMLButtonElement>('#party-toggle');
    const partyWindow =
      partyRoot === null
        ? undefined
        : new PartyWindow(
            partyRoot,
            {
              create: (name, itemEven, pickupShared) => {
                connection.partyCreate(name, itemEven, pickupShared);
              },
              invite: (name) => {
                connection.partyInvite(name);
              },
              leave: () => {
                connection.partyLeave();
              },
              expel: (name) => {
                connection.partyExpel(name);
              },
              setLeader: (name) => {
                connection.partySetLeader(name);
              },
              options: (options) => {
                connection.partyOptions(options);
              },
            },
            connection.limits.partyMembers,
          );
    if (partyWindow !== undefined) {
      partyWindow.selfName = () => view.selfName;
      partyWindow.jobNames = view.jobNames;
      const toggle = (): void => {
        partyWindow.toggle();
      };
      if (partyToggle !== null) {
        partyToggle.hidden = false;
        partyToggle.addEventListener('click', toggle);
      }
      altKey('KeyZ', toggle);
    }
    /** Shows a new party state in the window and the tabs. */
    const setParty = (next: typeof party): void => {
      party = next;
      partyWindow?.show(party);
      if (party.id === 0n) {
        tabs.hideParty();
        showTab(tabs.active);
      } else if (!tabs.hasParty) {
        tabs.showParty();
        tabBar.render();
      }
    };
    view.onPartyInfo = (info) => {
      setParty(partyFromInfo(info));
    };
    view.onPartyMember = (member) => {
      const next = mergeMember(party, member);
      if (next !== party) {
        party = next;
        partyWindow?.show(party);
      }
    };
    view.onPartyResult = (outcome, name) => {
      log.notice(partyMessage(outcome, name, view.selfName));
      setParty(partyAfterResult(party, outcome, name, view.selfName));
    };
    view.onPartyInvite = (invite) => {
      const answer = (accept: boolean): void => {
        connection.partyReply(invite.partyId, accept);
      };
      if (prompts === undefined) {
        answer(false);
        return;
      }
      prompts.show({
        key: `party:${String(invite.partyId)}`,
        text: partyInviteText(invite.from, invite.partyName),
        accept: () => {
          answer(true);
        },
        decline: () => {
          answer(false);
        },
      });
    };
    view.onPartyItemShared = (name, itemId, amount) => {
      itemTexts
        .loadNames()
        .then(() => {
          log.notice(itemSharedMessage(name, itemTexts.label(itemId), amount));
        })
        .catch(reportError);
    };
    view.onPartyChat = (name, text, lineId) => {
      log.party(name, text, name === view.selfName, lineId);
      unread(PARTY_TAB);
    };
    // Guilds. The guild window opens with its button or Alt+G, as in RO.
    // The Guild tab shows while the player has a guild.
    let guild = NO_GUILD;
    const guildRoot = document.querySelector<HTMLElement>('#guild-window');
    const guildToggle = document.querySelector<HTMLButtonElement>('#guild-toggle');
    const guildWindow =
      guildRoot === null
        ? undefined
        : new GuildWindow(
            guildRoot,
            {
              create: (name) => {
                connection.guildCreate(name);
              },
              invite: (name) => {
                connection.guildInvite(name);
              },
              leave: (reason) => {
                connection.guildLeave(reason);
              },
              expel: (name, reason) => {
                connection.guildExpel(name, reason);
              },
              setPosition: (name, position) => {
                connection.guildMemberPosition(name, position);
              },
              positions: (positions) => {
                connection.guildPositions(positions);
              },
              notice: (title, text) => {
                connection.guildNotice(title, text);
              },
              skillUp: (skillId) => {
                connection.guildSkillUp(skillId);
              },
              ally: (name) => {
                connection.guildAlly(name);
              },
              enemy: (name) => {
                connection.guildEnemy(name);
              },
              endRelation: (guildId, enemy) => {
                connection.guildEndRelation(guildId, enemy);
              },
              disband: (name) => {
                connection.guildDisband(name);
              },
              report: (guildId, emblem) => {
                connection.reportGuild(guildId, emblem);
              },
            },
            connection.limits,
          );
    if (guildWindow !== undefined) {
      guildWindow.selfName = () => view.selfName;
      guildWindow.jobNames = view.jobNames;
      guildWindow.emblem = (id, version) => emblemCache.get(id, version);
      emblemCache.onLoad = () => {
        if (guildWindow.open) {
          guildWindow.render();
        }
      };
      const toggle = (): void => {
        guildWindow.toggle();
      };
      if (guildToggle !== null) {
        guildToggle.hidden = false;
        guildToggle.addEventListener('click', toggle);
      }
      altKey('KeyG', toggle);
    }
    /** Shows a new guild state in the window and the tabs. */
    const setGuild = (next: typeof guild): void => {
      guild = next;
      guildWindow?.show(guild);
      if (guild.id === 0n) {
        tabs.hideGuild();
        showTab(tabs.active);
      } else if (!tabs.hasGuild) {
        tabs.showGuild();
        tabBar.render();
      }
    };
    view.onGuildInfo = (info) => {
      setGuild(guildFromInfo(info));
    };
    view.onGuildMember = (member) => {
      const next = mergeGuildMember(guild, member);
      if (next !== guild) {
        guild = next;
        guildWindow?.show(guild);
      }
    };
    view.onGuildResult = (outcome, name) => {
      log.notice(guildMessage(outcome, name, view.selfName));
      setGuild(guildAfterResult(guild, outcome, name, view.selfName));
    };
    view.onGuildInvite = (invite) => {
      const answer = (accept: boolean): void => {
        connection.guildReply(invite.guildId, accept);
      };
      if (prompts === undefined) {
        answer(false);
        return;
      }
      prompts.show({
        key: `guild:${String(invite.guildId)}`,
        text: guildInviteText(invite.from, invite.guildName),
        accept: () => {
          answer(true);
        },
        decline: () => {
          answer(false);
        },
      });
    };
    view.onGuildAllyRequest = (request) => {
      const answer = (accept: boolean): void => {
        connection.guildAllyReply(request.guildId, accept);
      };
      if (prompts === undefined) {
        answer(false);
        return;
      }
      prompts.show({
        key: `ally:${String(request.guildId)}`,
        text: guildAllyRequestText(request.from, request.guildName),
        accept: () => {
          answer(true);
        },
        decline: () => {
          answer(false);
        },
      });
    };
    view.onGuildChat = (name, text, lineId) => {
      log.guild(name, text, name === view.selfName, lineId);
      unread(GUILD_TAB);
    };
    // A right click on another player opens its menu: whisper, trade, invites,
    // and reports of its guild's name or emblem.
    const playerMenuRoot = document.querySelector<HTMLElement>('#player-menu');
    if (playerMenuRoot !== null) {
      const menu = new PlayerMenu(playerMenuRoot, {
        whisper: startWhisper,
        trade: (id) => {
          connection.tradeRequest(id);
        },
        partyInvite: (name) => {
          connection.partyInvite(name);
        },
        guildInvite: (name) => {
          connection.guildInvite(name);
        },
        reportGuild: (guildId, emblem) => {
          connection.reportGuild(guildId, emblem);
        },
        browseShop: (id) => {
          connection.vendingBrowse(id);
        },
        browseStore: (id) => {
          connection.buyingStoreBrowse(id);
        },
      });
      openPlayerMenu = (id, x, y) => {
        const name = view.nameOf(id);
        if (id === view.self || !view.isPlayer(id) || name === undefined) {
          return false;
        }
        const badge = view.guildBadge(id);
        menu.show(
          {
            id,
            name,
            badge:
              badge === undefined
                ? undefined
                : {
                    guildId: badge.guildId,
                    name: badge.name,
                    emblemVersion: badge.emblemVersion,
                    position: badge.position,
                  },
            partyInvite: leads(party, view.selfName),
            guildInvite: permissions(guild, view.selfName).invite,
            ownGuild: guild.id,
            shop: shopKind(view.shopBoard(id)),
          },
          x,
          y,
        );
        return true;
      };
    }
    // Friends. The friend list opens with the Friends button or Alt+H.
    const friendRoot = document.querySelector<HTMLElement>('#friend-panel');
    const friendToggle = document.querySelector<HTMLButtonElement>('#friend-toggle');
    if (friendRoot !== null) {
      const friends = new FriendPanel(
        friendRoot,
        {
          add: (name) => {
            connection.friendAdd(name);
          },
          remove: (name) => {
            connection.friendRemove(name);
          },
          whisper: startWhisper,
        },
        connection.limits.friends,
      );
      friends.selfName = () => view.selfName;
      const toggle = (): void => {
        friends.toggle();
      };
      friendToggle?.addEventListener('click', toggle);
      altKey('KeyH', toggle);
      view.onFriendList = (list) => {
        friends.show(friendsFromList(list));
      };
      view.onFriendStatus = (name, isOnline) => {
        const next = setFriendOnline(friends.friends, name, isOnline);
        if (next !== undefined) {
          friends.show(next);
          log.notice(friendStatusMessage(name, isOnline));
        }
      };
    }
    view.onFriendResult = (outcome, name) => {
      log.notice(friendMessage(outcome, name));
    };
    view.onFriendRequest = (from) => {
      const answer = (accept: boolean): void => {
        connection.friendReply(from, accept);
      };
      if (prompts === undefined) {
        answer(false);
        return;
      }
      prompts.show({
        key: `friend:${from}`,
        text: friendRequestText(from),
        accept: () => {
          answer(true);
        },
        decline: () => {
          answer(false);
        },
      });
    };
    // Party booking opens with its button or Alt+B.
    const bookingRoot = document.querySelector<HTMLElement>('#booking-window');
    const bookingToggle = document.querySelector<HTMLButtonElement>('#booking-toggle');
    if (bookingRoot !== null) {
      /** Set while a search waits for its answer. */
      let searching = false;
      const booking = new BookingWindow(bookingRoot, {
        register: (ad) => {
          connection.bookingRegister(ad);
        },
        search: (search) => {
          searching = true;
          connection.bookingSearch(search);
        },
        remove: () => {
          connection.bookingDelete();
        },
        whisper: startWhisper,
        invite: (name) => {
          connection.partyInvite(name);
        },
      });
      booking.leader = () => leads(party, view.selfName);
      booking.selfName = () => view.selfName;
      const toggle = (): void => {
        booking.toggle();
      };
      if (bookingToggle !== null) {
        bookingToggle.hidden = false;
        bookingToggle.addEventListener('click', toggle);
      }
      altKey('KeyB', toggle);
      view.onPartyBookingList = (list) => {
        booking.showOwn(
          list.registered && list.own !== undefined ? entryView(list.own) : undefined,
        );
        if (list.refused) {
          booking.showError(BOOKING_REFUSED);
          log.notice(BOOKING_REFUSED);
        }
        // An answer to register or delete lists no ads, and keeps the
        // last search's results.
        if (searching || list.entries.length > 0) {
          searching = false;
          booking.showResults(list.entries.map(entryView), list.more);
        }
      };
    }
    /** Sends a line if the rate allows it, and says so when it does not. */
    const sendLine = (send: () => void): boolean => {
      if (!rate.take(performance.now())) {
        // The line stays in the input, to send again later.
        log.notice('You are sending messages too fast.');
        return false;
      }
      send();
      return true;
    };
    /** Runs a line typed in the input. Returns whether to clear the input. */
    const run = (action: ChatAction): boolean => {
      switch (action.kind) {
        case 'empty':
          return true;
        case 'error':
          log.notice(action.message);
          return false;
        case 'say': {
          const target = tabs.target();
          return sendLine(() => {
            if (target.kind === 'channel') {
              connection.channelChat(target.name, action.text);
            } else if (target.kind === 'party') {
              connection.partyChat(action.text);
            } else if (target.kind === 'guild') {
              connection.guildChat(action.text);
            } else if (target.kind === 'whisper') {
              connection.whisper(target.name, action.text);
            } else {
              connection.chat(action.text);
            }
          });
        }
        case 'gm':
          // GM commands go to local chat whatever the tab is.
          return sendLine(() => {
            connection.chat(action.text);
          });
        case 'whisper':
          return sendLine(() => {
            connection.whisper(action.to, action.text);
          });
        case 'openWhisper':
          tabs.openWhisper(action.to);
          showTab(whisperKey(action.to));
          return true;
        case 'channel':
          return sendLine(() => {
            connection.channelChat(action.name, action.text);
          });
        case 'party':
          if (party.id === 0n) {
            log.notice(partyMessage(PartyOutcome.NO_PARTY, ''));
            return false;
          }
          return sendLine(() => {
            connection.partyChat(action.text);
          });
        case 'openParty':
          if (!tabs.hasParty) {
            log.notice(partyMessage(PartyOutcome.NO_PARTY, ''));
            return false;
          }
          showTab(PARTY_TAB);
          return true;
        case 'guild':
          if (guild.id === 0n) {
            log.notice(guildMessage(GuildOutcome.NO_GUILD, ''));
            return false;
          }
          return sendLine(() => {
            connection.guildChat(action.text);
          });
        case 'openGuild':
          if (!tabs.hasGuild) {
            log.notice(guildMessage(GuildOutcome.NO_GUILD, ''));
            return false;
          }
          showTab(GUILD_TAB);
          return true;
        case 'openChannel': {
          const tab = tabs.channel(action.name);
          if (tab === undefined) {
            log.notice(`You are not in ${action.name}. Type /join ${action.name} to join it.`);
            return false;
          }
          showTab(tab.key);
          return true;
        }
        case 'join':
          connection.channel(ChannelAction.JOIN, action.name);
          return true;
        case 'leave':
          connection.channel(ChannelAction.LEAVE, action.name);
          return true;
        case 'list':
          connection.channel(ChannelAction.LIST);
          return true;
        case 'members':
          connection.channel(ChannelAction.MEMBERS, action.name);
          return true;
        case 'block':
          connection.block(action.name);
          return true;
        case 'unblock':
          connection.block(action.name, true);
          return true;
        case 'report':
          connection.report(action.name, undefined, action.reason);
          return true;
        default:
          return false;
      }
    };
    /** Sends the input's line, as Enter or the Send button of touch. */
    const send = (): void => {
      const action = parseChatInput(chatInput.value);
      if (action.kind === 'empty') {
        // Enter on an empty line leaves the input, as in RO.
        chatInput.value = '';
        chatInput.blur();
        return;
      }
      if (run(action)) {
        chatInput.value = '';
      }
    };
    chatInput.addEventListener('keydown', (e) => {
      // Keys typed here are text, not game shortcuts.
      e.stopPropagation();
      if (e.key === 'Escape') {
        chatInput.blur();
        return;
      }
      if (e.key !== 'Enter' || e.isComposing) {
        return;
      }
      e.preventDefault();
      send();
    });
    const sendButton = document.querySelector<HTMLButtonElement>('#chat-send');
    // The button keeps the keyboard open: a press would move the focus.
    sendButton?.addEventListener('pointerdown', (e) => {
      e.preventDefault();
    });
    sendButton?.addEventListener('click', () => {
      if (chatInput.value.trim() === '') {
        chatInput.blur();
        chatRoot.classList.remove('open');
        return;
      }
      send();
    });
    // The chat of touch screens closes when the focus leaves it, as when
    // the on-screen keyboard is put away.
    chatRoot.addEventListener('focusout', (e) => {
      if (!(e.relatedTarget instanceof Node && chatRoot.contains(e.relatedTarget))) {
        chatRoot.classList.remove('open');
      }
    });
    openChat = () => {
      const open = !chatRoot.classList.contains('open');
      chatRoot.classList.toggle('open', open);
      if (open) {
        chatInput.focus();
      } else {
        chatInput.blur();
      }
    };
    addEventListener('keydown', (e) => {
      if (e.key === 'Enter' && !isTyping(e.target) && !e.altKey && !e.ctrlKey && !e.metaKey) {
        e.preventDefault();
        chatInput.focus();
      }
    });
  }
  // NPC dialogs, illustrations, progress bars and shops.
  const dialogRoot = document.querySelector<HTMLElement>('#dialog-window');
  const cutinRoot = document.querySelector<HTMLElement>('#cutins');
  if (dialogRoot !== null && cutinRoot !== null) {
    const d = new DialogWindow(dialogRoot, cutinRoot, store, {
      next: (dialog) => {
        connection.npcReply(dialog, { case: 'next', value: true });
      },
      menu: (dialog, option) => {
        connection.npcReply(dialog, { case: 'menu', value: option });
      },
      number: (dialog, value) => {
        connection.npcReply(dialog, { case: 'inputNumber', value });
      },
      text: (dialog, value) => {
        connection.npcReply(dialog, { case: 'inputText', value });
      },
      close: (dialog) => {
        connection.npcReply(dialog, { case: 'close', value: true });
      },
    });
    d.label = (id) => itemTexts.label(id);
    itemTexts.loadNames().catch(reportError);
    dialogWindow = d;
    view.onNpcDialog = (dialog, name) => {
      d.show(dialog, name);
    };
    view.onNpcImage = (name, position) => {
      d.showImage(name, position);
    };
  }
  view.onProgress = (ms) => {
    progressBar = { start: performance.now(), ms };
  };
  view.onMinimapMark = () => {
    // The client has no minimap yet. Marks are ignored.
  };
  const shopRoot = document.querySelector<HTMLElement>('#shop-window');
  if (shopRoot !== null) {
    const w = new ShopWindow(
      shopRoot,
      inventory,
      itemTexts,
      {
        buy: (npc, lines) => {
          connection.shopBuy(npc, lines);
        },
        sell: (npc, lines) => {
          connection.shopSell(npc, lines);
        },
        close: (npc) => {
          connection.shopClose(npc);
        },
      },
      itemIcons,
    );
    shopWindow = w;
    view.onShopOpen = (shop) => {
      itemTexts
        .loadNames()
        .then(() => {
          w.stats = view.stats;
          w.show(shop);
        })
        .catch(reportError);
    };
    view.onShopResult = (outcome, itemId) => {
      const text = shopMessage(outcome, itemId === 0 ? '' : itemTexts.label(itemId));
      if (text !== undefined) {
        logNotice(text);
      }
    };
  }
  // Currencies show in the inventory window, and the shop window checks
  // costs against them. Each CurrencyUpdate replaces the list.
  view.onCurrencies = (list) => {
    if (inventoryWindow !== undefined) {
      inventoryWindow.currencies = list;
    }
    if (shopWindow !== undefined) {
      shopWindow.currencies = list;
    }
  };
  const zenyNow = (): number => Number(view.stats.get(StatId.ZENY) ?? 0n);
  // Mail opens with its button or Alt+M. The server answers with the
  // mailbox. A new mail shows a chat line and a count on the button.
  const mailRoot = document.querySelector<HTMLElement>('#mail-window');
  const mailToggle = document.querySelector<HTMLButtonElement>('#mail-toggle');
  const setMailUnread = (n: number): void => {
    if (mailToggle !== null) {
      mailToggle.textContent = mailButtonText(n);
      mailToggle.classList.toggle('unread', n > 0);
    }
  };
  if (mailRoot !== null) {
    const mail = new MailWindow(
      mailRoot,
      {
        inventory: (slot) => inventory.get(slot),
        stacks: () => inventory.list(),
        zeny: zenyNow,
        maxItems: () => connection.limits.mailItems,
      },
      {
        read: (id) => {
          connection.mailRead(id);
        },
        send: (d) => {
          connection.mailSend({ ...d, items: [...d.items] });
        },
        take: (id, zeny, items) => {
          connection.mailTake(id, zeny, items);
        },
        remove: (id) => {
          connection.mailDelete(id);
        },
        close: () => {
          connection.mailClose();
        },
      },
      itemTexts,
      itemIcons,
    );
    mailWindow = mail;
    const toggle = (): void => {
      if (mail.open) {
        mail.requestClose();
      } else {
        connection.mailOpen();
      }
    };
    if (mailToggle !== null) {
      mailToggle.hidden = false;
      mailToggle.addEventListener('click', toggle);
    }
    addEventListener('keydown', (e) => {
      if (isTyping(e.target)) {
        return;
      }
      if (e.altKey && !e.ctrlKey && !e.metaKey && e.code === 'KeyM') {
        e.preventDefault();
        toggle();
      }
    });
    view.onMailBox = (mails, size, sentToday, daily) => {
      itemTexts
        .loadNames()
        .then(() => {
          mail.showBox(mails, size, sentToday, daily);
          setMailUnread(mail.unread);
        })
        .catch(reportError);
    };
    view.onMailRead = (m) => {
      itemTexts
        .loadNames()
        .then(() => {
          mail.showMail(m);
          setMailUnread(mail.unread);
        })
        .catch(reportError);
    };
    view.onMailResult = (r) => {
      logNotice(mailMessage(r.outcome, r.message));
      if (r.outcome === MailOutcome.SENT) {
        mail.sent();
      }
    };
  }
  view.onMailNew = (m) => {
    logNotice(mailNewText(m.from, m.title));
    setMailUnread(m.unread);
  };
  // Vending. The Vending skill opens the setup window. The player's own
  // shop shows in its window, and other shops open from their board.
  const vendingSetupRoot = document.querySelector<HTMLElement>('#vending-setup');
  const vendingOwnRoot = document.querySelector<HTMLElement>('#vending-own');
  const vendingBrowseRoot = document.querySelector<HTMLElement>('#vending-browse');
  if (vendingSetupRoot !== null && vendingOwnRoot !== null && vendingBrowseRoot !== null) {
    const setup = new VendingSetupWindow(
      vendingSetupRoot,
      () => cart.list(),
      {
        open: (title, items) => {
          connection.vendingOpen({ title, items });
        },
        cancel: () => {
          connection.vendingCancel();
        },
      },
      itemTexts,
      itemIcons,
    );
    const own = new VendingOwnWindow(
      vendingOwnRoot,
      () => {
        connection.vendingClose();
        own.closed();
      },
      itemTexts,
      itemIcons,
    );
    vendingOwnWindow = own;
    const browse = new VendingBrowseWindow(
      vendingBrowseRoot,
      zenyNow,
      {
        buy: (vendor, shop, lines) => {
          connection.vendingBuy(vendor, shop, lines);
        },
      },
      itemTexts,
      itemIcons,
    );
    view.onVendingSetup = (maxItems) => {
      itemTexts
        .loadNames()
        .then(() => {
          setup.show(maxItems);
        })
        .catch(reportError);
    };
    view.onVendingOpened = (title, items) => {
      setup.hide();
      itemTexts
        .loadNames()
        .then(() => {
          own.show(title, items);
        })
        .catch(reportError);
    };
    view.onVendingList = (shop) => {
      itemTexts
        .loadNames()
        .then(() => {
          browse.show(shop);
        })
        .catch(reportError);
    };
    view.onVendingSold = (sold) => {
      const itemId = own.sold(sold.index, sold.amount);
      logItem(() => vendingSoldText(sold.buyer, sold.amount, itemTexts.label(itemId), sold.zeny));
    };
    view.onVendingResult = (r) => {
      logNotice(vendingMessage(r.outcome, r.message));
      // CLOSED while no other shop is browsed is the player's own shop.
      if (r.outcome === VendingOutcome.CLOSED && !browse.open) {
        own.closed();
      }
    };
  }
  // Buying stores, as vending shops.
  const buyingSetupRoot = document.querySelector<HTMLElement>('#buying-setup');
  const buyingOwnRoot = document.querySelector<HTMLElement>('#buying-own');
  const buyingBrowseRoot = document.querySelector<HTMLElement>('#buying-browse');
  if (buyingSetupRoot !== null && buyingOwnRoot !== null && buyingBrowseRoot !== null) {
    const source = {
      stacks: () => inventory.list(),
      inventory: (slot: number) => inventory.get(slot),
      zeny: zenyNow,
    };
    const setup = new BuyingSetupWindow(
      buyingSetupRoot,
      source,
      {
        open: (title, zenyLimit, items) => {
          connection.buyingStoreOpen({ title, zenyLimit, items });
        },
        cancel: () => {
          connection.buyingStoreCancel();
        },
      },
      itemTexts,
    );
    const own = new BuyingOwnWindow(
      buyingOwnRoot,
      () => {
        connection.buyingStoreClose();
        own.closed();
      },
      itemTexts,
      itemIcons,
    );
    buyingOwnWindow = own;
    const browse = new BuyingBrowseWindow(
      buyingBrowseRoot,
      source,
      {
        sell: (buyer, storeId, lines) => {
          connection.buyingStoreSell(buyer, storeId, lines);
        },
      },
      itemTexts,
      itemIcons,
    );
    view.onBuyingStoreSetup = (slots) => {
      itemTexts
        .loadNames()
        .then(() => {
          setup.show(slots);
        })
        .catch(reportError);
    };
    view.onBuyingStoreOpened = (opened) => {
      setup.hide();
      itemTexts
        .loadNames()
        .then(() => {
          own.show(opened.title, opened.zenyLimit, opened.items);
        })
        .catch(reportError);
    };
    view.onBuyingStoreList = (list) => {
      itemTexts
        .loadNames()
        .then(() => {
          browse.show(list);
        })
        .catch(reportError);
    };
    view.onBuyingStoreBought = (b) => {
      own.bought(b.itemId, b.amount, b.zenyLimit);
      logItem(() => buyingBoughtText(b.seller, b.amount, itemTexts.label(b.itemId), b.zeny));
    };
    view.onBuyingStoreResult = (r) => {
      logNotice(
        buyingStoreMessage(r.outcome, r.itemId === 0 ? '' : itemTexts.label(r.itemId), r.message),
      );
      if (closesOwnStore(r.outcome) && (r.outcome !== BuyingStoreOutcome.CLOSED || !browse.open)) {
        own.closed();
      }
    };
  }
  // Store search opens when the server sends StoreSearchOpen.
  const storeSearchRoot = document.querySelector<HTMLElement>('#store-search');
  if (storeSearchRoot !== null) {
    const search = new StoreSearchWindow(
      storeSearchRoot,
      {
        query: (q) => {
          connection.storeSearchQuery(q);
        },
        next: () => {
          connection.storeSearchNext();
        },
        click: (index) => {
          connection.storeSearchClick(index);
        },
        close: () => {
          connection.storeSearchClose();
        },
      },
      itemTexts,
      itemIcons,
    );
    view.onStoreSearchOpen = (o) => {
      search.show(o.uses, o.open);
    };
    view.onStoreSearchResults = (results, uses, more) => {
      itemTexts
        .loadNames()
        .then(() => {
          search.showResults(results, uses, more);
        })
        .catch(reportError);
    };
    view.onStoreSearchResult = (r) => {
      // The client has no minimap yet, so a store's place shows as text.
      const text = storeSearchMessage(r.outcome, r.map, r.x, r.y);
      search.showStatus(text);
      logNotice(text);
    };
  }
  // A warp moves the player to another map: the view clears, a loading
  // screen shows, and the new map loads from the asset store, which the
  // Service Worker caches.
  const loadingView = document.querySelector<HTMLDivElement>('#loading');
  view.onMapChange = (map, x, y) => {
    // The new map's first frame shows the icons again.
    icons.clear();
    castBars.clear();
    targetMode.cancel();
    showTargeting();
    const started = performance.now();
    const before = { ...store.stats };
    changingMap = true;
    if (loadingView !== null) {
      loadingView.textContent = `Loading ${map.replace(/^map\//, '')}…`;
      loadingView.hidden = false;
    }
    input.value = map;
    show(map, [x, y])
      .then((shown) => {
        if (!shown) {
          return;
        }
        const ms = performance.now() - started;
        mapLoads.push({
          map,
          ms,
          files: store.stats.files - before.files,
          bytes: store.stats.bytes - before.bytes,
        });
        changingMap = false;
        if (loadingView !== null) {
          loadingView.hidden = true;
        }
      })
      .catch(reportError);
  };
  session = { connection, online: view };
  scene.add(view.group);
  input.value = connection.welcome.map;
  input.disabled = true;
  show(connection.welcome.map).catch(reportError);
}

// Lets browser tests inspect the scene in development builds.
if (import.meta.env.DEV) {
  Reflect.set(globalThis, 'viewer', {
    scene,
    camera,
    audio,
    playing: () => playing,
    player: () => player,
    session: () => session,
    dialog: () => dialogWindow,
    shop: () => shopWindow,
    mapLoads: () => mapLoads,
    map: () => current?.map,
    // The touch checks of scripts/browser-check.ts find units on the
    // screen and read the camera and the target mode.
    units: () => {
      const o = session?.online;
      if (o === undefined) {
        return [];
      }
      return o.unitIds().map((id) => {
        const p = o.position(id);
        return {
          id,
          name: o.nameOf(id),
          npc: o.isNpc(id),
          player: o.isPlayer(id),
          self: id === o.self,
          cell: p === undefined ? undefined : [Math.floor(p.x), Math.floor(-p.z)],
        };
      });
    },
    screenOf: (id: number) => {
      const p = session?.online.position(id)?.clone();
      if (p === undefined) {
        return undefined;
      }
      p.y += 1;
      p.project(camera);
      const rect = canvas.getBoundingClientRect();
      return [rect.left + ((p.x + 1) / 2) * rect.width, rect.top + ((1 - p.y) / 2) * rect.height];
    },
    selfCell: () =>
      session?.online.selfCell ?? (player === undefined ? undefined : [player.x, player.y]),
    azimuth: () => controls.getAzimuthalAngle(),
    distance: () => camera.position.distanceTo(controls.target),
    targeting: () => targetMode.kind,
    hotkeys: () => [...Array(9).keys()].map((i) => hotkeyBar?.get(i)),
  });
}

const right = new Vector3();

/** Where a point is left or right of the listener, from -1 to 1. */
function pan(p: Vector3): number {
  right.setFromMatrixColumn(camera.matrixWorld, 0);
  return Math.max(-1, Math.min(1, p.clone().sub(controls.target).dot(right) / SPRITE_SOUND_RANGE));
}

// A click on the ground walks the player there. A drag turns the camera
// instead. Touch has its own gestures, below.
const raycaster = new Raycaster();
const pointer = new Vector2();
let pressed: [number, number] | undefined;
canvas.addEventListener('pointerdown', (e) => {
  if (e.pointerType === 'touch') {
    return;
  }
  pressed = [e.clientX, e.clientY];
});
canvas.addEventListener('pointerup', (e) => {
  if (e.pointerType === 'touch') {
    return;
  }
  if (pressed === undefined || Math.hypot(e.clientX - pressed[0], e.clientY - pressed[1]) > 5) {
    return;
  }
  clickAt(e.clientX, e.clientY, e.button);
});

/** What a click at a point of the page does: button 0 is left, 2 right. */
function clickAt(clientX: number, clientY: number, button: number): void {
  const ground = current?.group.getObjectByName('ground');
  if (ground === undefined || (player === undefined && session === undefined) || changingMap) {
    return;
  }
  const rect = canvas!.getBoundingClientRect();
  pointer.set(
    ((clientX - rect.left) / rect.width) * 2 - 1,
    -((clientY - rect.top) / rect.height) * 2 + 1,
  );
  raycaster.setFromCamera(pointer, camera);
  const hit = raycaster.intersectObject(ground, true)[0];
  // Online, a click on a unit attacks it, a click on an item picks it up,
  // and a click on the ground walks there. The server decides the path and
  // sends it back.
  const target = session?.online.pick(
    clientX - rect.left,
    clientY - rect.top,
    camera,
    rect.width,
    rect.height,
  );
  // A skill waiting for a target takes the click: an entity for enemy
  // and support skills, a cell for ground skills. A support skill may
  // target the player itself. A right click cancels it.
  if (session !== undefined && targetMode.kind !== undefined) {
    if (button === 2) {
      targetMode.cancel();
    } else if (targetMode.kind === 'entity') {
      const t = session.online.pick(
        clientX - rect.left,
        clientY - rect.top,
        camera,
        rect.width,
        rect.height,
        true,
      );
      const request =
        t === undefined || t.item
          ? undefined
          : targetMode.clickEntity(t.id, t.id === session.online.self);
      if (request !== undefined) {
        session.connection.useSkill(request.skillId, request.level, request.target);
      }
    } else if (hit !== undefined) {
      const request = targetMode.clickCell(Math.floor(hit.point.x), Math.floor(-hit.point.z));
      if (request !== undefined) {
        session.connection.useSkill(request.skillId, request.level, request.target);
      }
    }
    showTargeting();
    return;
  }
  // A right click on another player opens its menu.
  if (
    button === 2 &&
    target !== undefined &&
    !target.item &&
    openPlayerMenu?.(target.id, clientX, clientY) === true
  ) {
    return;
  }
  // While the player talks to an NPC, the game does nothing on a click,
  // as the server ignores it (pc_cant_act).
  if (dialogWindow?.open === true && progressBar === undefined) {
    return;
  }
  if (session !== undefined && target !== undefined) {
    if (target.item) {
      session.connection.pickUp(target.id);
    } else if (session.online.isNpc(target.id)) {
      session.connection.npcClick(target.id);
    } else if (session.online.shopBoard(target.id) !== undefined) {
      // A click on a player with an open shop browses it.
      browseBoard(target.id);
    } else if ((weightPercent(session.online.stats) ?? 0) >= MAJOR_OVERWEIGHT) {
      // The server refuses attacks from 90% weight, as the game client.
      showMessage('You are carrying too much to attack.');
    } else {
      session.connection.attack(target.id, true);
    }
    return;
  }
  if (hit !== undefined) {
    const x = Math.floor(hit.point.x);
    const y = Math.floor(-hit.point.z);
    if (session === undefined) {
      player?.moveTo(x, y);
    } else {
      session.connection.walkTo(x, y);
    }
  }
}

/** Shows the target mode: the cursor, and the cell marker for ground skills. */
function showTargeting(): void {
  canvas!.style.cursor = targetMode.kind === undefined ? '' : 'crosshair';
  showTargetCancel(targetMode.kind !== undefined);
  if (targetMode.kind !== 'cell') {
    cellMarker.visible = false;
  }
}

/** Moves the cell marker to the cell under a point of the canvas. */
function markCell(e: PointerEvent): void {
  const ground = current?.group.getObjectByName('ground');
  if (targetMode.kind !== 'cell' || ground === undefined || current === undefined) {
    cellMarker.visible = false;
    return;
  }
  const rect = canvas!.getBoundingClientRect();
  pointer.set(
    ((e.clientX - rect.left) / rect.width) * 2 - 1,
    -((e.clientY - rect.top) / rect.height) * 2 + 1,
  );
  raycaster.setFromCamera(pointer, camera);
  const hit = raycaster.intersectObject(ground, true)[0];
  if (hit === undefined) {
    cellMarker.visible = false;
    return;
  }
  const x = Math.floor(hit.point.x) + 0.5;
  const y = Math.floor(-hit.point.z) + 0.5;
  cellMarker.position.set(x, cellHeight(current.map, x, y) + 0.05, -y);
  cellMarker.visible = true;
}

// The name of the ground item under the pointer shows under it, as in RO.
canvas.addEventListener('pointermove', (e) => {
  if (session === undefined || e.pointerType === 'touch') {
    hoveredItem = undefined;
    return;
  }
  markCell(e);
  const rect = canvas.getBoundingClientRect();
  const target = session.online.pick(
    e.clientX - rect.left,
    e.clientY - rect.top,
    camera,
    rect.width,
    rect.height,
  );
  hoveredItem = target?.item === true ? target.id : undefined;
});
canvas.addEventListener('pointerleave', () => {
  hoveredItem = undefined;
});

// Touch on the game view (review decision 39). A tap does what a left
// click does, a finger held on the ground walks toward it, a long press
// on a unit does what a right click does, and two fingers zoom and turn
// the camera. OrbitControls keeps the mouse and leaves touch to this.
controls.touches = { ONE: null, TWO: null };
const gestures = new GestureRecognizer();
let pressTimer: ReturnType<typeof setTimeout> | undefined;
/** Where a finger held on the ground is, while it walks the player. */
let holdAt: [number, number] | undefined;
/** The joystick's push, while it is held. */
let joystickPushed: Push | undefined;
/** The last direction the joystick walked, in cells, for the stop. */
let joystickDir: [number, number] | undefined;
const walkRequests = new WalkRequests();

/** Asks for a walk to a cell, online or in the map viewer. */
function walkTo(x: number, y: number): void {
  if (session === undefined) {
    player?.moveTo(x, y);
  } else {
    session.connection.walkTo(x, y);
  }
}

/** Whether the player may walk now: not while a map loads or an NPC talks. */
function mayWalk(): boolean {
  return (
    !changingMap &&
    (session !== undefined || player !== undefined) &&
    !(dialogWindow?.open === true && progressBar === undefined)
  );
}

/** The cell under a point of the page, if it is on the ground. */
function cellAt(clientX: number, clientY: number): [number, number] | undefined {
  const ground = current?.group.getObjectByName('ground');
  if (ground === undefined) {
    return undefined;
  }
  const rect = canvas!.getBoundingClientRect();
  pointer.set(
    ((clientX - rect.left) / rect.width) * 2 - 1,
    -((clientY - rect.top) / rect.height) * 2 + 1,
  );
  raycaster.setFromCamera(pointer, camera);
  const hit = raycaster.intersectObject(ground, true)[0];
  return hit === undefined ? undefined : [Math.floor(hit.point.x), Math.floor(-hit.point.z)];
}

/** What the joystick's walks know about the map. */
const joystickGround: Ground = {
  walkable: (x, y) => current !== undefined && walkable(current.map, x, y),
  pathLength: (x0, y0, x1, y1) => pathfinder.search(x0, y0, x1, y1)?.length,
};

/** The player's cell, or the one it steps into now. */
function walkFrom(): [number, number] | undefined {
  const me = session?.online.me ?? player;
  return me?.nextCell;
}

/** Does what a gesture on the game view asks. */
function onGesture(g: Gesture): void {
  switch (g.kind) {
    case 'tap':
      clickAt(g.x, g.y, 0);
      break;
    case 'press': {
      // A long press is a right click: it cancels a waiting skill, and
      // opens another player's menu. On the ground, it walks.
      if (targetMode.kind !== undefined) {
        clickAt(g.x, g.y, 2);
        break;
      }
      const rect = canvas!.getBoundingClientRect();
      const target = session?.online.pick(
        g.x - rect.left,
        g.y - rect.top,
        camera,
        rect.width,
        rect.height,
      );
      if (target === undefined) {
        holdAt = [g.x, g.y];
        walkRequests.reset();
      } else if (!target.item) {
        openPlayerMenu?.(target.id, g.x, g.y);
      }
      break;
    }
    case 'hold':
      if (g.phase === 'start' && targetMode.kind === undefined) {
        holdAt = [g.x, g.y];
        walkRequests.reset();
      } else if (g.phase === 'move' && holdAt !== undefined) {
        holdAt = [g.x, g.y];
      } else if (g.phase === 'end') {
        holdAt = undefined;
      }
      break;
    case 'pinch':
      if (g.phase !== 'move') {
        break;
      }
      if (g.scale !== 1) {
        controls.dollyOut(g.scale);
      }
      // A twist and a sideways drag turn the camera; an upward drag tilts it.
      controls.rotateLeft(g.rotation + (2 * Math.PI * g.dx) / canvas!.clientHeight);
      controls.rotateUp((2 * Math.PI * g.dy) / canvas!.clientHeight);
      break;
    default:
      break;
  }
}

function runGestures(list: Gesture[]): void {
  for (const g of list) {
    onGesture(g);
  }
  clearTimeout(pressTimer);
  const at = gestures.pressAt;
  if (at !== undefined) {
    pressTimer = setTimeout(
      () => {
        runGestures(gestures.tick(performance.now()));
      },
      Math.max(0, at - performance.now()),
    );
  }
}

canvas.addEventListener('pointerdown', (e) => {
  if (e.pointerType === 'touch') {
    runGestures(gestures.down(e.pointerId, e.clientX, e.clientY, performance.now()));
  }
});
canvas.addEventListener('pointermove', (e) => {
  if (e.pointerType === 'touch') {
    runGestures(gestures.move(e.pointerId, e.clientX, e.clientY, performance.now()));
  }
});
canvas.addEventListener('pointerup', (e) => {
  if (e.pointerType === 'touch') {
    runGestures(gestures.up(e.pointerId, e.clientX, e.clientY, performance.now()));
  }
});
canvas.addEventListener('pointercancel', (e) => {
  if (e.pointerType === 'touch') {
    runGestures(gestures.cancel());
  }
});

/** Sends the walks of a held finger and of the joystick, once a frame. */
function touchWalk(now: number): void {
  if (
    (joystickPushed === undefined && holdAt === undefined) ||
    !walkRequests.due(now) ||
    !mayWalk()
  ) {
    return;
  }
  if (joystickPushed !== undefined) {
    const from = walkFrom();
    if (from === undefined) {
      return;
    }
    joystickDir = cellDirection(joystickPushed, controls.getAzimuthalAngle());
    const cell = walkRequests.next(now, joystickTarget(from, joystickDir, joystickGround));
    if (cell !== undefined) {
      walkTo(cell[0], cell[1]);
    }
    return;
  }
  if (holdAt !== undefined) {
    const cell = walkRequests.next(now, cellAt(holdAt[0], holdAt[1]));
    if (cell !== undefined) {
      walkTo(cell[0], cell[1]);
    }
  }
}

// The on-screen controls of touch, the window buttons' menu on small
// screens, and the options.
const inputMode = new InputMode(document.body);
installTouchUI(inputMode, canvas);
let settings = loadSettings(() => localStorage);
const touchControls = new TouchControls(document.body, {
  push: (p) => {
    if (p !== undefined && joystickPushed === undefined) {
      walkRequests.reset();
    }
    joystickPushed = p;
  },
  release: () => {
    const from = walkFrom();
    if (from !== undefined && joystickDir !== undefined && mayWalk()) {
      const cell = walkRequests.final(
        performance.now(),
        stopTarget(from, joystickDir, joystickGround),
      );
      if (cell !== undefined) {
        walkTo(cell[0], cell[1]);
      }
    }
    joystickDir = undefined;
  },
  chat: () => {
    openChat();
  },
  cancelTarget: () => {
    targetMode.cancel();
    showTargeting();
  },
});
touchControls.joystickShown = settings.joystick;
showTargetCancel = (on) => {
  touchControls.targeting = on;
};
const optionsRoot = document.querySelector<HTMLElement>('#options-window');
const joystickOption = document.querySelector<HTMLInputElement>('#joystick-option');
if (joystickOption !== null) {
  joystickOption.checked = settings.joystick;
  joystickOption.addEventListener('change', () => {
    settings = { ...settings, joystick: joystickOption.checked };
    saveSettings(() => localStorage, settings);
    touchControls.joystickShown = settings.joystick;
  });
}
document.querySelector('#options-toggle')?.addEventListener('click', () => {
  if (optionsRoot !== null) {
    optionsRoot.hidden = !optionsRoot.hidden;
  }
});
// Panels on small screens open under the top bar, whose height changes
// with its text and the open menu.
new ResizeObserver(() => {
  document.documentElement.style.setProperty('--controls-height', `${form.offsetHeight}px`);
}).observe(form);
const menuToggle = document.querySelector<HTMLButtonElement>('#menu-toggle');
menuToggle?.addEventListener('click', () => {
  const open = form.classList.toggle('menu-open');
  menuToggle.setAttribute('aria-expanded', String(open));
});
// A window button closes the menu again on small screens.
form.addEventListener('click', (e) => {
  if (
    e.target instanceof HTMLButtonElement &&
    e.target !== menuToggle &&
    form.classList.contains('menu-open')
  ) {
    form.classList.remove('menu-open');
    menuToggle?.setAttribute('aria-expanded', 'false');
  }
});
// Windows move by a title bar, with any pointer.
for (const w of document.querySelectorAll<HTMLElement>(
  'body > section[aria-label]:not(#world-events)',
)) {
  makeMovable(w, w.getAttribute('aria-label') ?? '');
}

// The chat stays above the on-screen keyboard. The keyboard covers the
// bottom of the page without making it smaller, and only the visual
// viewport shows how much it covers.
const visual = window.visualViewport;
if (visual !== null) {
  const keyboard = (): void => {
    const covered = Math.max(0, innerHeight - visual.height - visual.offsetTop);
    document.documentElement.style.setProperty('--keyboard', `${Math.round(covered)}px`);
  };
  visual.addEventListener('resize', keyboard);
  visual.addEventListener('scroll', keyboard);
  keyboard();
}

const itemLabelView = document.querySelector<HTMLDivElement>('#item-label');
const itemLabelAt = new Vector3();

/** Places the hovered ground item's name under it. */
function showItemLabel(): void {
  if (itemLabelView === null) {
    return;
  }
  const g = hoveredItem === undefined ? undefined : session?.online.groundItem(hoveredItem);
  if (g === undefined) {
    itemLabelView.hidden = true;
    return;
  }
  const text = groundItemLabel(itemTexts.label(g.itemId), g.amount);
  if (itemLabelView.textContent !== text) {
    itemLabelView.textContent = text;
  }
  itemLabelAt.copy(g.position).project(camera);
  itemLabelView.hidden = itemLabelAt.z > 1;
  itemLabelView.style.left = `${((itemLabelAt.x + 1) / 2) * 100}%`;
  itemLabelView.style.top = `${((1 - itemLabelAt.y) / 2) * 100}%`;
}

const guildLabelRoot = document.querySelector<HTMLDivElement>('#guild-labels');
const guildLabelViews = new Map<number, HTMLDivElement>();
const guildLabelAt = new Vector3();

/**
 * Places the guild labels under the players that show a guild, as RO
 * shows the emblem, the name, and "Guild [Position]" under a player. An
 * emblem that has not loaded, or failed to, shows nothing.
 */
function showGuildLabels(): void {
  if (guildLabelRoot === null || session === undefined) {
    return;
  }
  const view = session.online;
  const badges = view.guildBadges();
  for (const [id, div] of guildLabelViews) {
    if (!badges.has(id)) {
      div.remove();
      guildLabelViews.delete(id);
    }
  }
  for (const [id, badge] of badges) {
    let div = guildLabelViews.get(id);
    if (div === undefined) {
      div = document.createElement('div');
      div.className = 'guild-label';
      const img = document.createElement('img');
      img.className = 'guild-label-emblem';
      img.alt = '';
      img.width = 24;
      img.height = 24;
      img.hidden = true;
      const text = document.createElement('div');
      const name = document.createElement('bdi');
      name.className = 'guild-label-name';
      const guild = document.createElement('bdi');
      guild.className = 'guild-label-guild';
      text.append(name, guild);
      div.append(img, text);
      guildLabelRoot.append(div);
      guildLabelViews.set(id, div);
    }
    const [img, text] = div.children;
    const name = text?.firstElementChild;
    const guild = text?.lastElementChild;
    const playerName = view.nameOf(id) ?? '';
    if (name instanceof HTMLElement && name.textContent !== playerName) {
      name.textContent = playerName;
    }
    const line = badgeLine(badge);
    if (guild instanceof HTMLElement && guild.textContent !== line) {
      guild.textContent = line;
    }
    if (img instanceof HTMLImageElement) {
      const url = emblems?.get(badge.guildId, badge.emblemVersion);
      if (url === undefined) {
        img.hidden = true;
      } else {
        if (img.getAttribute('src') !== url) {
          img.crossOrigin = 'anonymous';
          img.src = url;
        }
        img.hidden = false;
      }
    }
    const p = view.position(id);
    if (p === undefined) {
      div.hidden = true;
      continue;
    }
    guildLabelAt.copy(p).project(camera);
    div.hidden = guildLabelAt.z > 1;
    div.style.left = `${((guildLabelAt.x + 1) / 2) * 100}%`;
    div.style.top = `${((1 - guildLabelAt.y) / 2) * 100}%`;
  }
}

/** What kind of shop a board shows, for the player menu. */
function shopKind(board: ShopBoard | undefined): 'vending' | 'buying' | undefined {
  if (board === undefined) {
    return undefined;
  }
  return board.buying ? 'buying' : 'vending';
}

/**
 * Browses the shop of a player with a board: its vending shop or its
 * buying store. The player's own board shows its own window.
 */
function browseBoard(id: number): void {
  const s = session;
  const board = s?.online.shopBoard(id);
  if (s === undefined || board === undefined) {
    return;
  }
  if (id === s.online.self) {
    (board.buying ? buyingOwnWindow : vendingOwnWindow)?.toggle();
  } else if (board.buying) {
    s.connection.buyingStoreBrowse(id);
  } else {
    s.connection.vendingBrowse(id);
  }
}

const shopBoardRoot = document.querySelector<HTMLDivElement>('#shop-boards');
const shopBoardViews = new Map<number, HTMLButtonElement>();
const shopBoardAt = new Vector3();

/**
 * Places the shop boards over the players with an open shop, as RO shows
 * a vending board. A click on a board browses the shop.
 */
function showShopBoards(): void {
  if (shopBoardRoot === null || session === undefined) {
    return;
  }
  const view = session.online;
  const boards = view.shopBoards();
  for (const [id, div] of shopBoardViews) {
    if (!boards.has(id)) {
      div.remove();
      shopBoardViews.delete(id);
    }
  }
  for (const [id, board] of boards) {
    let div = shopBoardViews.get(id);
    if (div === undefined) {
      div = document.createElement('button');
      div.type = 'button';
      div.addEventListener('click', () => {
        browseBoard(id);
      });
      shopBoardRoot.append(div);
      shopBoardViews.set(id, div);
    }
    const className = board.buying ? 'shop-board buying' : 'shop-board';
    if (div.className !== className) {
      div.className = className;
    }
    if (div.textContent !== board.title) {
      div.textContent = board.title;
      div.title = board.buying ? 'Buying store: click to browse' : 'Shop: click to browse';
    }
    const p = view.position(id);
    if (p === undefined) {
      div.hidden = true;
      continue;
    }
    shopBoardAt.copy(p);
    shopBoardAt.y += 3.8;
    shopBoardAt.project(camera);
    div.hidden = shopBoardAt.z > 1;
    div.style.left = `${((shopBoardAt.x + 1) / 2) * 100}%`;
    div.style.top = `${((1 - shopBoardAt.y) / 2) * 100}%`;
  }
}

const bubbleAt = new Vector3();

/** Places the chat bubbles above their speakers. */
function showBubbles(now: number): void {
  if (bubbleRoot === null || session === undefined) {
    return;
  }
  for (const id of bubbles.expire(now)) {
    bubbleViews.get(id)?.remove();
    bubbleViews.delete(id);
  }
  for (const [id, text] of bubbles.active(now)) {
    let div = bubbleViews.get(id);
    if (div === undefined) {
      div = document.createElement('div');
      div.className = 'bubble';
      bubbleRoot.append(div);
      bubbleViews.set(id, div);
    }
    if (div.textContent !== text) {
      div.textContent = text;
    }
    // A speaker out of view keeps its bubble hidden until it ends.
    const p = session.online.position(id);
    if (p === undefined) {
      div.hidden = true;
      continue;
    }
    bubbleAt.copy(p);
    bubbleAt.y += 3;
    bubbleAt.project(camera);
    div.hidden = bubbleAt.z > 1;
    div.style.left = `${((bubbleAt.x + 1) / 2) * 100}%`;
    div.style.top = `${((1 - bubbleAt.y) / 2) * 100}%`;
  }
}

const castBarRoot = document.querySelector<HTMLDivElement>('#cast-bars');
const castBarViews = new Map<number, HTMLDivElement>();
const castBarAt = new Vector3();

/** Places the cast bars over their casters and fills them. */
function showCastBars(now: number): void {
  if (castBarRoot === null || session === undefined) {
    return;
  }
  const active = castBars.active(now);
  castBars.expire(now);
  for (const [id, div] of castBarViews) {
    if (!active.has(id)) {
      div.remove();
      castBarViews.delete(id);
    }
  }
  for (const [id, fill] of active) {
    let div = castBarViews.get(id);
    if (div === undefined) {
      div = document.createElement('div');
      div.className = 'cast-bar';
      div.append(document.createElement('div'));
      castBarRoot.append(div);
      castBarViews.set(id, div);
    }
    const p = session.online.position(id);
    if (p === undefined) {
      div.hidden = true;
      continue;
    }
    castBarAt.copy(p);
    castBarAt.y += 3.2;
    castBarAt.project(camera);
    div.hidden = castBarAt.z > 1;
    div.style.left = `${((castBarAt.x + 1) / 2) * 100}%`;
    div.style.top = `${((1 - castBarAt.y) / 2) * 100}%`;
    const bar = div.firstElementChild;
    if (bar instanceof HTMLElement) {
      bar.style.width = `${fill * 100}%`;
    }
  }
}

/** Loads the player's hotkeys once its name is known, and shows the slots. */
function showHotkeys(now: number): void {
  const bar = hotkeyBar;
  if (bar === undefined) {
    return;
  }
  const name = session?.online.selfName;
  if (name !== undefined && name !== hotkeysOf) {
    hotkeysOf = name;
    bar.load(loadHotkeys(() => localStorage, name));
  }
  bar.update((h) => {
    const s = skills.get(h.skillId);
    const left = cooldowns.remaining(h.skillId, now);
    if (s === undefined) {
      return { badge: '?', title: `Skill ${h.skillId}`, cooldown: '', usable: false };
    }
    const label = s.displayName || s.name;
    return {
      badge: skillBadge(label),
      title: `${label} Lv ${castLevel(s, h.level)}`,
      cooldown: cooldownText(left),
      usable: castable(s) && left === 0,
    };
  });
}

const progressView = document.querySelector<HTMLDivElement>('#progress');
const progressAt = new Vector3();

/** Places the progress bar over the player and fills it. */
function showProgress(now: number): void {
  if (progressView === null) {
    return;
  }
  const me = session?.online.me;
  if (progressBar === undefined || me === undefined || now - progressBar.start >= progressBar.ms) {
    progressBar = undefined;
    progressView.hidden = true;
    return;
  }
  progressAt.copy(me.character.group.position);
  progressAt.y += 3.5;
  progressAt.project(camera);
  progressView.hidden = progressAt.z > 1;
  progressView.style.left = `${((progressAt.x + 1) / 2) * 100}%`;
  progressView.style.top = `${((1 - progressAt.y) / 2) * 100}%`;
  const bar = progressView.firstElementChild;
  if (bar instanceof HTMLElement) {
    bar.style.width = `${Math.min(100, ((now - progressBar.start) / progressBar.ms) * 100)}%`;
  }
}

const follow = new Vector3();
const hpOutput = document.querySelector<HTMLOutputElement>('#hp');
const expOutput = document.querySelector<HTMLOutputElement>('#exp');

const timer = new Timer();
const toward = new Vector3();
renderer.setAnimationLoop((time) => {
  timer.update(time);
  current?.update(timer.getElapsed(), timer.getDelta());
  // Sprites face the camera, so their shown direction follows its turn.
  const steps = Math.round(controls.getAzimuthalAngle() / (Math.PI / 4));
  toward.subVectors(camera.position, controls.target).normalize();
  walkers?.update(timer.getDelta() * 1000, steps, toward);
  player?.update(timer.getDelta() * 1000, steps, toward);
  session?.online.update(timer.getDelta() * 1000, steps, toward);
  const me = session?.online.me ?? player;
  if (me !== undefined) {
    // The camera follows the player, keeping its angle and distance.
    follow
      .subVectors(me.character.group.position, controls.target)
      .multiplyScalar(Math.min(1, timer.getDelta() * 8));
    controls.target.add(follow);
    camera.position.add(follow);
  }
  const stats = session?.online.stats;
  if (stats !== undefined) {
    const hp = hpLine(stats);
    if (hp !== undefined && hpOutput !== null && hpOutput.value !== hp) {
      hpOutput.value = hp;
    }
    const exp = expLine(stats, session?.online.jobNames);
    if (exp !== undefined && expOutput !== null && expOutput.value !== exp) {
      expOutput.value = exp;
    }
    if (statusWindow?.open === true) {
      statusWindow.update(stats);
    }
    if (inventoryWindow?.open === true) {
      inventoryWindow.update(stats);
    }
    if (equipmentWindow?.open === true) {
      equipmentWindow.update();
    }
    if (shopWindow !== undefined) {
      shopWindow.stats = stats;
    }
    if (skillWindow?.open === true) {
      const now = performance.now();
      skillWindow.update(stats, (id) => cooldowns.remaining(id, now));
    }
  }
  touchWalk(performance.now());
  showHotkeys(performance.now());
  showCastBars(performance.now());
  showBubbles(performance.now());
  statusIcons?.update(performance.now());
  showItemLabel();
  showGuildLabels();
  showShopBoards();
  showProgress(performance.now());
  right.setFromMatrixColumn(camera.matrixWorld, 0);
  soundscape?.update(timer.getDelta(), { position: controls.target, right });
  for (const e of playing) {
    e.update(timer.getDelta() * 1000);
  }
  controls.update();
  renderer.render(scene, camera);
});

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
import { AudioSystem } from '../audio/audio';
import Soundscape from '../audio/soundscape';
import { cellHeight } from '../formats/wmap';
import MapScene from '../render/map';
import { SpriteCache, SpriteView } from '../render/sprites';
import { Character } from '../render/characters';
import { EffectCache, EffectView } from '../render/effects';
import { DamageKind, InventoryRemoveReason } from '../gen/wathena/v1/event_pb';
import { DisconnectReason } from '../gen/wathena/v1/session_pb';
import { accountPage, accountURL, fetchTicket } from '../net/account';
import gameServerURL from '../net/server';
import { Connection } from '../net/connection';
import { Pathfinder } from '../world/pathfinder';
import { Bubbles, ChatLog, ChatRate, chatLine, cleanChat, isTyping } from './chat';
import { expLine, hpLine, pickUpMessage } from './hud';
import {
  InventoryModel,
  InventoryWindow,
  MAJOR_OVERWEIGHT,
  SLOT_DRAG_TYPE,
  dropItemMessage,
  gotItemMessage,
  useItemMessage,
  usedItemMessage,
  weightPercent,
} from './inventory';
import { ItemTexts } from './itemText';
import { EquipmentWindow, INSERT_CARD_FAILED, equipMessage } from './equipment';
import { DialogWindow } from './dialog';
import { ShopWindow, shopMessage } from './shop';
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
import Online from './online';
import Player from './player';
import Walkers from './walkers';

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
/** Adds a line from the game to the chat log, once the log exists. */
let logNotice: (text: string) => void = () => {};
/** The ground item under the pointer, whose name shows. */
let hoveredItem: number | undefined;
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
  // Events can follow Welcome at once, so the view exists first. It learns
  // the player's own ID from Welcome.
  const view = new Online(0, sprites, pathfinder);
  const connection = await Connection.open(serverURL, token, {
    events: (events) => {
      view.handle(events);
    },
    closed: (message, reason) => {
      status.value = message;
      // Status icons belong to the session that ended.
      statusIcons?.clear();
      if (account === undefined) {
        return;
      }
      // Back to the character list, which sends a player whose login
      // ended on to the login page. A used or expired ticket goes at
      // once. Other endings wait so the message can be read.
      const back = accountPage(account, 'characters', location.href);
      setTimeout(
        () => {
          location.assign(back);
        },
        reason === DisconnectReason.SESSION_INVALID ? 0 : 3000,
      );
    },
  });
  view.self = connection.welcome.netId;
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
  view.onInventory = (items, size) => {
    inventory.reset(items, size);
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
  // Local chat. Enter opens the input, Enter sends the line, and Escape
  // leaves the input. Lines show in the log and above the speaker.
  const chatRoot = document.querySelector<HTMLElement>('#chat');
  const chatList = document.querySelector<HTMLOListElement>('#chat-log');
  const chatInput = document.querySelector<HTMLInputElement>('#chat-input');
  if (chatRoot !== null && chatList !== null && chatInput !== null) {
    chatRoot.hidden = false;
    const log = new ChatLog(chatList);
    logNotice = (text) => {
      log.notice(text);
    };
    const rate = new ChatRate(performance.now());
    view.onChat = (from, speaker, text) => {
      log.add(speaker, text, from === view.self);
      bubbles.say(from, chatLine(speaker, text), performance.now());
    };
    view.onServerText = (text) => {
      log.server(text);
    };
    view.onAnnouncement = (text, color) => {
      log.announce(text, color);
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
      const text = cleanChat(chatInput.value);
      if (text === undefined) {
        // Enter on an empty line leaves the input, as in RO.
        chatInput.value = '';
        chatInput.blur();
        return;
      }
      if (!rate.take(performance.now())) {
        // The line stays in the input, to send again later.
        log.notice('You are sending messages too fast.');
        return;
      }
      connection.chat(text);
      chatInput.value = '';
    });
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
    const w = new ShopWindow(shopRoot, inventory, itemTexts, {
      buy: (npc, lines) => {
        connection.shopBuy(npc, lines);
      },
      sell: (npc, lines) => {
        connection.shopSell(npc, lines);
      },
      close: (npc) => {
        connection.shopClose(npc);
      },
    });
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
    view.onShopResult = (outcome) => {
      const text = shopMessage(outcome);
      if (text !== undefined) {
        logNotice(text);
      }
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
  });
}

const right = new Vector3();

/** Where a point is left or right of the listener, from -1 to 1. */
function pan(p: Vector3): number {
  right.setFromMatrixColumn(camera.matrixWorld, 0);
  return Math.max(-1, Math.min(1, p.clone().sub(controls.target).dot(right) / SPRITE_SOUND_RANGE));
}

// A click on the ground walks the player there. A drag turns the camera
// instead.
const raycaster = new Raycaster();
const pointer = new Vector2();
let pressed: [number, number] | undefined;
canvas.addEventListener('pointerdown', (e) => {
  pressed = [e.clientX, e.clientY];
});
canvas.addEventListener('pointerup', (e) => {
  if (pressed === undefined || Math.hypot(e.clientX - pressed[0], e.clientY - pressed[1]) > 5) {
    return;
  }
  const ground = current?.group.getObjectByName('ground');
  if (ground === undefined || (player === undefined && session === undefined) || changingMap) {
    return;
  }
  const rect = canvas.getBoundingClientRect();
  pointer.set(
    ((e.clientX - rect.left) / rect.width) * 2 - 1,
    -((e.clientY - rect.top) / rect.height) * 2 + 1,
  );
  raycaster.setFromCamera(pointer, camera);
  const hit = raycaster.intersectObject(ground, true)[0];
  // Online, a click on a unit attacks it, a click on an item picks it up,
  // and a click on the ground walks there. The server decides the path and
  // sends it back.
  const target = session?.online.pick(
    e.clientX - rect.left,
    e.clientY - rect.top,
    camera,
    rect.width,
    rect.height,
  );
  // A skill waiting for a target takes the click: an entity for enemy
  // and support skills, a cell for ground skills. A support skill may
  // target the player itself. A right click cancels it.
  if (session !== undefined && targetMode.kind !== undefined) {
    if (e.button === 2) {
      targetMode.cancel();
    } else if (targetMode.kind === 'entity') {
      const t = session.online.pick(
        e.clientX - rect.left,
        e.clientY - rect.top,
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
});

/** Shows the target mode: the cursor, and the cell marker for ground skills. */
function showTargeting(): void {
  canvas!.style.cursor = targetMode.kind === undefined ? '' : 'crosshair';
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
  if (session === undefined) {
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
    const exp = expLine(stats);
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
  showHotkeys(performance.now());
  showCastBars(performance.now());
  showBubbles(performance.now());
  statusIcons?.update(performance.now());
  showItemLabel();
  showProgress(performance.now());
  right.setFromMatrixColumn(camera.matrixWorld, 0);
  soundscape?.update(timer.getDelta(), { position: controls.target, right });
  for (const e of playing) {
    e.update(timer.getDelta() * 1000);
  }
  controls.update();
  renderer.render(scene, camera);
});

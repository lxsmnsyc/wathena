// A map viewer for development. It loads a map from the build that
// `wactl assets serve` serves, and shows download progress.

import {
  Color,
  ColorManagement,
  Group,
  PerspectiveCamera,
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
import { Connection } from '../net/connection';
import { Pathfinder } from '../world/pathfinder';
import randomLook from './looks';
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
let current: MapScene | undefined;
let session: { connection: Connection; online: Online } | undefined;
const pathfinder = await Pathfinder.load();
let player: Player | undefined;
const playerGroup = new Group();
scene.add(playerGroup);
const audio = new AudioSystem(store);
let soundscape: Soundscape | undefined;

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
// ?server= connects to a game server, such as ws://127.0.0.1:8300/ws, as
// ?name=. The server then chooses the map and moves every character, and
// the demo sprites are off unless asked for.
const params = new URL(location.href).searchParams;
const serverURL = params.get('server');
const online = serverURL !== null;
const DEFAULT_SPRITES = online ? '' : 'mob/1002,mob/1063,mob/1007,mob/1113,mob/1008,npc/4_f_kafra1';
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

async function show(name: string): Promise<void> {
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
      return;
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
    session?.online.setMap(next.map);
    const [px, py] =
      session?.online.me === undefined
        ? Player.start(next.map)
        : [session.online.me.x, session.online.me.y];
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
  } catch (error) {
    status!.value = String(error);
  } finally {
    clearInterval(timer);
  }
}

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
  const name = params.get('name') ?? `Player${Math.floor(Math.random() * 10000)}`;
  // Events can follow Welcome at once, so the view exists first. It learns
  // the player's own ID from Welcome.
  const view = new Online(0, sprites, pathfinder);
  const connection = await Connection.open(serverURL, name, {
    events: (events) => {
      view.handle(events);
    },
    closed: (reason) => {
      status.value = reason;
    },
  });
  view.self = connection.welcome.netId;
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
  if (ground === undefined || (player === undefined && session === undefined)) {
    return;
  }
  const rect = canvas.getBoundingClientRect();
  pointer.set(
    ((e.clientX - rect.left) / rect.width) * 2 - 1,
    -((e.clientY - rect.top) / rect.height) * 2 + 1,
  );
  raycaster.setFromCamera(pointer, camera);
  const hit = raycaster.intersectObject(ground, true)[0];
  if (hit !== undefined) {
    const x = Math.floor(hit.point.x);
    const y = Math.floor(-hit.point.z);
    // Online, the server decides the path and sends it back.
    if (session === undefined) {
      player?.moveTo(x, y);
    } else {
      session.connection.walkTo(x, y);
    }
  }
});

const follow = new Vector3();

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
  right.setFromMatrixColumn(camera.matrixWorld, 0);
  soundscape?.update(timer.getDelta(), { position: controls.target, right });
  for (const e of playing) {
    e.update(timer.getDelta() * 1000);
  }
  controls.update();
  renderer.render(scene, camera);
});

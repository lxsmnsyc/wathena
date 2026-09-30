// A map viewer for development. It loads a map from the build that
// `wactl assets serve` serves, and shows download progress.

import {
  Color,
  ColorManagement,
  Group,
  PerspectiveCamera,
  Scene,
  Timer,
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
import randomLook from './looks';
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

const store = await AssetStore.open(base);
let current: MapScene | undefined;
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
const DEFAULT_SPRITES = 'mob/poring,mob/lunatic,mob/fabre,mob/drops,mob/pupa,npc/4_f_kafra1';
// Effects to play in a row near the center, such as effect/firewall.
const effectNames = (new URL(location.href).searchParams.get('effect') ?? '')
  .split(',')
  .filter((n) => n !== '');
const effects = new EffectCache(store);
let playing: EffectView[] = [];
const effectGroup = new Group();
scene.add(effectGroup);

// Dropped items, as item IDs. Their sprites are sprite/item/<id>.
const DEFAULT_DROPS = '501,502,503,504,505,506,507,508,512,601,602,909,914,938,1201,2301,4001';
const dropIDs = (new URL(location.href).searchParams.get('drops') ?? DEFAULT_DROPS)
  .split(',')
  .filter((n) => /^\d+$/.test(n));

const characterCount = Number(new URL(location.href).searchParams.get('characters') ?? 30);
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
  const before = { ...store.stats };
  const progress = (label: string): void => {
    if (id !== loading) {
      return;
    }
    const files = store.stats.files - before.files;
    const bytes = store.stats.bytes - before.bytes;
    const seconds = ((performance.now() - start) / 1000).toFixed(1);
    status!.value = `${label}: ${files} files, ${mb(transferred(start))} sent (${mb(bytes)} decoded), ${seconds} s`;
  };
  progress('loading');
  const timer = setInterval(() => {
    progress('loading');
  }, 200);
  try {
    const { scene: next, ready } = await MapScene.load(store, `map/${name}`);
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
    // Look at the map's center from the south, as RO's camera does.
    const cx = next.map.width / 2;
    const cz = -next.map.height / 2;
    const size = Math.max(next.map.width, next.map.height);
    controls.target.set(cx, 0, cz);
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
      walkers.add(async (i) => {
        const asset = await sprites.get(
          spriteNames[i % spriteNames.length] ?? 'placeholder/sprite',
        );
        return asset === undefined ? undefined : new SpriteView(asset);
      }, 20),
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

input.value = new URL(location.href).searchParams.get('map') ?? 'prontera';
show(input.value).catch(reportError);

// Lets browser tests inspect the scene in development builds.
if (import.meta.env.DEV) {
  Reflect.set(globalThis, 'viewer', { scene, camera, audio, playing: () => playing });
}

const right = new Vector3();

/** Where a point is left or right of the listener, from -1 to 1. */
function pan(p: Vector3): number {
  right.setFromMatrixColumn(camera.matrixWorld, 0);
  return Math.max(-1, Math.min(1, p.clone().sub(controls.target).dot(right) / SPRITE_SOUND_RANGE));
}

const timer = new Timer();
const toward = new Vector3();
renderer.setAnimationLoop((time) => {
  timer.update(time);
  current?.update(timer.getElapsed(), timer.getDelta());
  // Sprites face the camera, so their shown direction follows its turn.
  const steps = Math.round(controls.getAzimuthalAngle() / (Math.PI / 4));
  toward.subVectors(camera.position, controls.target).normalize();
  walkers?.update(timer.getDelta() * 1000, steps, toward);
  right.setFromMatrixColumn(camera.matrixWorld, 0);
  soundscape?.update(timer.getDelta(), { position: controls.target, right });
  for (const e of playing) {
    e.update(timer.getDelta() * 1000);
  }
  controls.update();
  renderer.render(scene, camera);
});

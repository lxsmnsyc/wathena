// A map viewer for development. It loads a map from the build that
// `wactl assets serve` serves, and shows download progress.

import { Color, ColorManagement, PerspectiveCamera, Scene, Timer, WebGLRenderer } from 'three';
import { OrbitControls } from 'three/addons/controls/OrbitControls.js';
import { AssetStore } from '../assets/store';
import { cellHeight } from '../formats/wmap';
import MapScene from '../render/map';
import { SpriteCache } from '../render/sprites';
import Walkers from './walkers';

// Shaders work on stored color values directly, as RO does.
ColorManagement.enabled = false;

const base = import.meta.env.VITE_ASSETS_URL ?? '/assets/';

const canvas = document.querySelector<HTMLCanvasElement>('#view');
const form = document.querySelector<HTMLFormElement>('#controls');
const input = document.querySelector<HTMLInputElement>('#map');
const status = document.querySelector<HTMLOutputElement>('#status');
if (canvas === null || form === null || input === null || status === null) {
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
let walkers: Walkers | undefined;
const sprites = new SpriteCache(store);

// Sprites to scatter on the map, as asset names without "sprite/".
const DEFAULT_SPRITES = 'mob/poring,mob/lunatic,mob/fabre,mob/drops,mob/pupa,npc/4_f_kafra1';
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
    walkers = new Walkers(next.map);
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
    await Promise.all([ready, walkers.add(sprites, spriteNames, 40)]);
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

const timer = new Timer();
renderer.setAnimationLoop((time) => {
  timer.update(time);
  current?.update(timer.getElapsed(), timer.getDelta());
  // Sprites face the camera, so their shown direction follows its turn.
  const steps = Math.round(controls.getAzimuthalAngle() / (Math.PI / 4));
  walkers?.update(timer.getDelta() * 1000, steps);
  controls.update();
  renderer.render(scene, camera);
});

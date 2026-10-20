# 0060: Client Rendering

- Status: proposed. Not built.
- Date: 2026-10-20.
- Code: [client/src](../../client/src) (`render/sprites.ts`, `render/map.ts`, `render/models.ts`, `viewer/main.ts`, `viewer/online.ts`, `assets/store.ts`, `world/generator.ts`, `world/fieldWorker.ts`).
- Lua API: none. Protocol: none. Settings: none.

## Context

The findings below come from reading the code on 2026-10-20. None was measured. The client draws with three.js on WebGL. These are the weak points, most severe first.

1. **One mesh and one material per sprite layer, never culled.** `SpriteView.layer` (`render/sprites.ts`) makes a new `ShaderMaterial` and `Mesh` for each layer, with `frustumCulled = false` and `transparent: true`. Draw calls are units × parts (body, head, headgear, weapon and so on) × layers. Units off screen are drawn too, and three.js sorts every transparent object each frame.
2. **Nothing is freed.** No file in `client/src` calls `dispose`. A map change removes the map's group from the scene (`viewer/main.ts`) without freeing its geometry or textures, and makes a new `TextureCache` (`render/map.ts`). `SpriteCache` keeps every sprite and palette texture. `AssetStore` keeps the bytes of every file it downloaded (`assets/store.ts`). A long session grows in GPU and main memory with every map and every new sprite.
3. **Map objects are not instanced.** Each placed object is a `clone()` of its model in its own group (`render/map.ts`, `render/models.ts`), so it is one draw call per mesh per placement. Each animated object has its own `AnimationMixer`, updated every frame.
4. **Overlay elements are rewritten every frame.** Guild labels, shop signs, quest icons, chat bubbles and cast bars each set `style.left`, `style.top` and `hidden` every frame, changed or not (`viewer/main.ts`). Open windows call `update(stats)` every frame.
5. **Per-frame garbage.** `SpriteView.draw` builds string keys each frame (`render/sprites.ts`). `dying.filter` makes a new array each frame (`viewer/online.ts`). The hotkey bar builds objects and strings for every slot each frame (`viewer/main.ts`).
6. **Map generation copies buffers it does not use.** `Generator.result` copies `types` and `heights` out of wasm memory (`world/generator.ts`). `heights` is about 16 MB on a 1,024 × 1,024 map. The worker only sends `wmap` back (`world/fieldWorker.ts`).

Checked and fine: no wasm call runs every frame. Pathfinding runs on clicks and on limited joystick walks.

## Decision

Fix them in this order. Each step must keep `scripts/check.sh` and the browser checks of decision [0037](0037-reliable-browser-checks.md) passing.

### 1. Free what a map change leaves behind

- On a map change, walk the old map's group and call `dispose` on every geometry, material and texture it owns.
- Give `SpriteCache` and `AssetStore` a size limit with least-recently-used eviction. Dispose a texture when it is evicted.
- Add a debug readout of `renderer.info.memory` and `renderer.info.render.calls`.

### 2. Sprite batching

- Pack each unit's sprite frames into a texture atlas per sprite file.
- Draw all sprite layers of all units with one shared material, as an `InstancedMesh` or a `BatchedMesh`. Per-instance attributes hold the atlas rectangle, palette row, position, scale, tint and draw order.
- Sort only sprites, by depth, in one array each frame. Do not let three.js sort them as separate objects.
- Skip units outside the camera's view.

### 3. Map object instancing

- Static objects that share a model become one `InstancedMesh` per mesh of that model.
- Animated objects of the same model and animation share one mixer and differ only by time offset.

### 4. Overlay updates

- Keep the last written position and visibility for each overlay element. Write the style only when it changed.
- Move elements with `transform: translate(...)` instead of `left` and `top`, so the browser does not lay out the page again.
- Windows update when the stats they show change, not every frame.

### 5. Garbage

- Compare frame numbers directly instead of building string keys.
- Reuse arrays for dying units and the hotkey bar.

### 6. Map generation

- The worker asks the generator for `wmap` only. `types` and `heights` are copied only when another caller asks for them.

## Breaks

- Nothing for the server, mods or worlds.
- Step 2 changes how sprites are drawn. Screenshots in the browser checks will change slightly where draw order between sprites with equal depth changes. Ties keep the order of the unit's net ID.

## Alternatives

- **WebGPU.** Firefox and Safari are not supported anyway, but WebGPU support in Chromium is still not on every device. Batching gives most of the gain on WebGL.
- **Canvas 2D for sprites.** It is simpler, but it cannot mix with the 3D map in one depth order.
- **Fewer layers per unit, by baking body and head together.** Equipment changes often, so baked sprites would be made again often. The atlas keeps layers separate at little cost.

## How to check it worked

- A browser check that opens a town with 200 units: `renderer.info.render.calls` before and after step 2.
- A browser check that changes map 50 times: `renderer.info.memory` returns to near its start value.
- Frame time p99 in Chromium's performance profile for the same town, before and after steps 2 and 4.

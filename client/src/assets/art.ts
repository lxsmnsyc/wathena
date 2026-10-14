// The asset names of records (assets-v1, "Names", and decision 0033). Core's
// records are named by ID, such as icon/item/501. A mod's records get IDs
// that differ per server, so the server's client data says which art each
// of them uses, such as mod/example.lantern/icon/item/Paper_Lantern. A
// record without an entry keeps the name by ID, and a name the builds
// lack shows a placeholder.

import type { ClientData } from '../data/clientData';

let data: ClientData | undefined;

/** Uses the server's client data from now on. Undefined forgets it. */
export function setArt(d: ClientData | undefined): void {
  data = d;
}

/** An item's icon. */
export function itemIcon(id: number): string {
  return data?.items[String(id)]?.icon ?? `icon/item/${id}`;
}

/** An item's large image. */
export function itemImage(id: number): string {
  return data?.items[String(id)]?.image ?? `image/item/${id}`;
}

/** The sprite of an item on the ground. */
export function dropSprite(id: number): string {
  return data?.items[String(id)]?.drop_sprite ?? `sprite/item/${id}`;
}

/** A monster's sprite, by the ID its look shows. */
export function monsterSprite(id: number): string {
  return data?.monsters[String(id)]?.sprite ?? `sprite/mob/${id}`;
}

/** A monster's sound for a moment, such as attack, or undefined. */
export function monsterSound(id: number, moment: string): string | undefined {
  return data?.monsters[String(id)]?.sounds?.[moment];
}

/** An NPC's sprite, by its sprite ID. */
export function npcSprite(id: number): string {
  return data?.npc_sprites[String(id)] ?? `sprite/npc/${id}`;
}

/** A job's body sprite for a sex, when its record names art. */
export function jobBody(job: number, sex: 'm' | 'f'): string | undefined {
  return data?.jobs[String(job)]?.[sex];
}

/** A skill's icon, when its record names art. */
export function skillIcon(id: number): string | undefined {
  return data?.skills[String(id)]?.icon;
}

// The asset names of records (assets-v1, "Names", and decision 0033). Core's
// records are named by ID, such as icon/item/501. A mod's records get IDs
// that differ per server, so the server's client data says which art each
// of them uses, such as mod/example.lantern/icon/item/Paper_Lantern. A
// record without an entry keeps the name by ID, and a name the builds
// lack shows a placeholder.

import type { ClientData } from '../data/clientData';

let data: ClientData | undefined;
/** The view sprites of items, by the view ID the server gave them. */
let views = new Map<number, string>();

/** Uses the server's client data from now on. Undefined forgets it. */
export function setArt(d: ClientData | undefined): void {
  data = d;
  views = new Map();
  for (const item of Object.values(d?.items ?? {})) {
    if (item.view !== undefined && item.view_sprite !== undefined) {
      views.set(item.view, item.view_sprite);
    }
  }
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
  return data?.jobs[String(job)]?.body?.[sex];
}

/** A skill's icon, when its record names art. */
export function skillIcon(id: number): string | undefined {
  return data?.skills[String(id)]?.icon;
}

/**
 * The effects a skill's record names for a moment: caster, effect or hit.
 * Undefined when the record names none for it, so the client tables'
 * effects play.
 */
export function skillArtEffects(id: number, moment: string): readonly string[] | undefined {
  return data?.skills[String(id)]?.effects?.[moment];
}

/**
 * The sprite a worn item shows, by the view ID in a look, when the item's
 * record names a view_sprite. Other views keep their sprite by ID.
 */
export function viewSprite(view: number): string | undefined {
  return views.get(view);
}

/**
 * The art of a companion record, by the record ID in a look, when the
 * record names art (decision 0036).
 */
export function companionSprite(id: number): string | undefined {
  return id === 0 ? undefined : data?.companions[String(id)];
}

/** The art of a mount record, by the record ID in a look, when it names art. */
export function mountSprite(id: number): string | undefined {
  return id === 0 ? undefined : data?.mounts[String(id)];
}

// The names of mods' records (review decision 40). Core's records keep
// the names of the asset build's tables and of the client's tables.

/** A mod item's name, card slots and description, when the server names it. */
export function modItem(
  id: number,
): { name?: string; slots: number; description?: readonly string[] } | undefined {
  const item = data?.items[String(id)];
  if (item?.name === undefined && item?.description === undefined) {
    return undefined;
  }
  return {
    ...(item.name === undefined ? {} : { name: item.name }),
    slots: item.slots ?? 0,
    ...(item.description === undefined ? {} : { description: item.description }),
  };
}

/** The names of every item the server names, for item search. */
export function modItemNames(): [number, string][] {
  return Object.entries(data?.items ?? {}).flatMap(([id, item]) =>
    item.name === undefined ? [] : [[Number(id), item.name] as [number, string]],
  );
}

/** A mod monster's name, by the ID its look shows. */
export function modMonsterName(id: number): string | undefined {
  return data?.monsters[String(id)]?.name;
}

/** A mod skill's name. */
export function modSkillName(id: number): string | undefined {
  return data?.skills[String(id)]?.name;
}

/**
 * The job whose sprites and tables a job uses beside its body art: the
 * look a mod's job starts from, or the job itself.
 */
export function lookJob(id: number): number {
  return data?.jobs[String(id)]?.look ?? id;
}

/** A mod job's name. */
export function modJobName(id: number): string | undefined {
  return data?.jobs[String(id)]?.name;
}

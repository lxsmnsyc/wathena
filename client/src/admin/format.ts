// Text for the cells of the player, item and log screens. These are pure
// functions, so the unit tests check them without a page.

import { timestampDate } from '@bufbuild/protobuf/wkt';
import type { Item, Owner, Position, Sanction, Storage } from '../gen/wathena/admin/v1/admin_pb';

/**
 * Names an owner of an item or zeny: its name and kind when it has a
 * name, else kind#id, or the kind alone for an end without an ID.
 */
export function ownerText(o: Owner | undefined): string {
  if (o === undefined || o.kind === '') {
    return '';
  }
  const extra = o.storage > 0 ? ` storage ${String(o.storage)}` : '';
  if (o.name !== '') {
    return `${o.name} (${o.kind}${extra})`;
  }
  if (o.id !== 0n) {
    return `${o.kind}#${o.id.toString()}${extra}`;
  }
  return o.kind + extra;
}

/** The character an owner belongs to, or 0. */
export function ownerCharacter(o: Owner | undefined): bigint {
  if (o === undefined) {
    return 0n;
  }
  if (o.characterId !== 0n) {
    return o.characterId;
  }
  return o.kind === 'inventory' || o.kind === 'cart' ? o.id : 0n;
}

/** An item's name with its refine, cards and state. */
export function itemText(item: {
  name: string;
  itemId: number;
  refine: number;
  cards: readonly number[];
  identified?: boolean;
}): string {
  const name = item.name === '' ? `item ${String(item.itemId)}` : item.name;
  let text = item.refine > 0 ? `+${String(item.refine)} ${name}` : name;
  const cards = item.cards.filter((c) => c !== 0);
  if (cards.length > 0) {
    text += ` [${cards.join(', ')}]`;
  }
  if (item.identified === false) {
    text += ' (unidentified)';
  }
  return text;
}

/** The items that are worn, which have equip set. */
export function worn(items: readonly Item[]): Item[] {
  return items.filter((i) => i.equip !== 0);
}

/** The items that are carried and not worn. */
export function carried(items: readonly Item[]): Item[] {
  return items.filter((i) => i.equip === 0);
}

/** A place as "map x,y", or "". */
export function positionText(p: Position | undefined): string {
  if (p === undefined || p.map === '') {
    return '';
  }
  return `${p.map} ${String(p.x)},${String(p.y)}`;
}

/** A sanction in effect, such as "mute until ...: spam". */
export function sanctionText(s: Sanction): string {
  let text = s.kind;
  if (s.endsAt !== undefined) {
    text += ` until ${timestampDate(s.endsAt).toLocaleString()}`;
  }
  if (s.reason !== '') {
    text += `: ${s.reason}`;
  }
  return text;
}

/** The title of a storage tab. */
export function storageTitle(s: Storage): string {
  switch (s.kind) {
    case 'cart':
      return 'Cart';
    case 'storage':
      return 'Account storage';
    default:
      return s.name === '' ? `Extra storage ${String(s.storage)}` : s.name;
  }
}

/** The relations of a move in an item history. */
export type Relation = 'self' | 'ancestor' | 'descendant';

/** A move's relation, with self for an unknown one. */
export function relationOf(relation: string): Relation {
  return relation === 'ancestor' || relation === 'descendant' ? relation : 'self';
}

/** The words for a move's relation in the history table. */
export function relationText(relation: string): string {
  switch (relationOf(relation)) {
    case 'ancestor':
      return 'came from';
    case 'descendant':
      return 'went into';
    default:
      return 'this serial';
  }
}

/** The cursor of a page: none for a new search, else the next one. */
export function pageCursor(fresh: boolean, next: string): string {
  return fresh ? '' : next;
}

/** Whether a page answer has more pages after it. */
export function hasMore(nextCursor: string): boolean {
  return nextCursor !== '';
}

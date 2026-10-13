import { create } from '@bufbuild/protobuf';
import { describe, expect, it } from 'vitest';
import {
  ItemSchema,
  OwnerSchema,
  PositionSchema,
  SanctionSchema,
  StorageSchema,
} from '../gen/wathena/admin/v1/admin_pb';
import {
  carried,
  hasMore,
  itemText,
  ownerCharacter,
  ownerText,
  pageCursor,
  positionText,
  relationOf,
  relationText,
  sanctionText,
  storageTitle,
  worn,
} from './format';

describe('ownerText', () => {
  it('names an owner, else writes kind#id, else the kind', () => {
    expect(ownerText(undefined)).toBe('');
    expect(ownerText(create(OwnerSchema, { kind: 'inventory', id: 150000n, name: 'Bob' }))).toBe(
      'Bob (inventory)',
    );
    expect(ownerText(create(OwnerSchema, { kind: 'mail', id: 12n }))).toBe('mail#12');
    expect(ownerText(create(OwnerSchema, { kind: 'ground' }))).toBe('ground');
    expect(ownerText(create(OwnerSchema, { kind: 'storage', id: 2000001n, storage: 3 }))).toBe(
      'storage#2000001 storage 3',
    );
  });

  it('finds the character an owner belongs to', () => {
    expect(ownerCharacter(create(OwnerSchema, { kind: 'mail', id: 5n, characterId: 7n }))).toBe(7n);
    expect(ownerCharacter(create(OwnerSchema, { kind: 'inventory', id: 9n }))).toBe(9n);
    expect(ownerCharacter(create(OwnerSchema, { kind: 'ground' }))).toBe(0n);
    expect(ownerCharacter(undefined)).toBe(0n);
  });
});

describe('itemText', () => {
  it('writes the refine, cards and state', () => {
    expect(itemText({ name: 'Knife', itemId: 1201, refine: 0, cards: [] })).toBe('Knife');
    expect(itemText({ name: 'Knife', itemId: 1201, refine: 7, cards: [4001, 0, 4002, 0] })).toBe(
      '+7 Knife [4001, 4002]',
    );
    expect(itemText({ name: '', itemId: 501, refine: 0, cards: [], identified: false })).toBe(
      'item 501 (unidentified)',
    );
  });
});

describe('worn and carried', () => {
  it('splits the inventory by equip', () => {
    const items = [
      create(ItemSchema, { serial: 'a', equip: 2 }),
      create(ItemSchema, { serial: 'b' }),
    ];
    expect(worn(items).map((i) => i.serial)).toEqual(['a']);
    expect(carried(items).map((i) => i.serial)).toEqual(['b']);
  });
});

describe('places, sanctions and storages', () => {
  it('writes a position as map x,y', () => {
    expect(positionText(create(PositionSchema, { map: 'prontera', x: 150, y: 180 }))).toBe(
      'prontera 150,180',
    );
    expect(positionText(undefined)).toBe('');
  });

  it('writes a sanction with its reason', () => {
    expect(sanctionText(create(SanctionSchema, { kind: 'mute', reason: 'spam' }))).toBe(
      'mute: spam',
    );
  });

  it('titles each storage', () => {
    expect(storageTitle(create(StorageSchema, { kind: 'cart' }))).toBe('Cart');
    expect(storageTitle(create(StorageSchema, { kind: 'storage' }))).toBe('Account storage');
    expect(storageTitle(create(StorageSchema, { kind: 'extra', storage: 2 }))).toBe(
      'Extra storage 2',
    );
    expect(storageTitle(create(StorageSchema, { kind: 'extra', storage: 2, name: 'Bank' }))).toBe(
      'Bank',
    );
  });
});

describe('relations', () => {
  it('reads a move relation, with self for unknown ones', () => {
    expect(relationOf('ancestor')).toBe('ancestor');
    expect(relationOf('descendant')).toBe('descendant');
    expect(relationOf('self')).toBe('self');
    expect(relationOf('')).toBe('self');
    expect(relationText('ancestor')).toBe('came from');
    expect(relationText('descendant')).toBe('went into');
    expect(relationText('self')).toBe('this serial');
  });
});

describe('cursors', () => {
  it('starts a new search without a cursor and stops at an empty one', () => {
    expect(pageCursor(true, 'next')).toBe('');
    expect(pageCursor(false, 'next')).toBe('next');
    expect(hasMore('next')).toBe(true);
    expect(hasMore('')).toBe(false);
  });
});

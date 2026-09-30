import type { AssetStore } from '../assets/store';
import { PickUpFailure, StatId } from '../gen/wathena/v1/event_pb';

// The player's status on the page: HP, levels, exp, and inventory.

/**
 * Exp toward the next level as a percent with one decimal, rounded down,
 * as RO shows it. next is 0 at the maximum level, which shows "max".
 */
export function expPercent(exp: bigint, next: bigint): string {
  if (next <= 0n) {
    return 'max';
  }
  let tenths = (exp * 1000n) / next;
  if (tenths < 0n) {
    tenths = 0n;
  }
  if (tenths > 1000n) {
    tenths = 1000n;
  }
  return `${(Number(tenths) / 10).toFixed(1)}%`;
}

/** The HP line, or undefined until the server sends HP. */
export function hpLine(stats: ReadonlyMap<StatId, bigint>): string | undefined {
  const hp = stats.get(StatId.HP);
  const max = stats.get(StatId.MAX_HP);
  if (hp === undefined || max === undefined) {
    return undefined;
  }
  return `HP ${hp < 0n ? 0n : hp} / ${max}`;
}

/** The levels and exp line, or undefined until the server sends levels. */
export function expLine(stats: ReadonlyMap<StatId, bigint>): string | undefined {
  const level = (name: string, lv: StatId, exp: StatId, next: StatId): string | undefined => {
    const l = stats.get(lv);
    if (l === undefined) {
      return undefined;
    }
    const e = stats.get(exp);
    const n = stats.get(next);
    return e === undefined || n === undefined
      ? `${name} ${l}`
      : `${name} ${l} (${expPercent(e, n)})`;
  };
  const base = level('Base Lv', StatId.BASE_LEVEL, StatId.BASE_EXP, StatId.NEXT_BASE_EXP);
  const job = level('Job Lv', StatId.JOB_LEVEL, StatId.JOB_EXP, StatId.NEXT_JOB_EXP);
  const parts = [base, job].filter((p) => p !== undefined);
  return parts.length === 0 ? undefined : parts.join(', ');
}

/** What to tell the player when a pick up fails. */
export function pickUpMessage(reason: PickUpFailure): string {
  return reason === PickUpFailure.NOT_YOURS
    ? 'You cannot pick up this item yet.'
    : 'The item is no longer there.';
}

/** The inventory list on the page: an icon, the item ID, and the amount. */
export class Inventory {
  private readonly amounts = new Map<number, number>();
  private readonly rows = new Map<number, HTMLLIElement>();

  constructor(
    private readonly list: HTMLElement,
    private readonly store: AssetStore,
  ) {}

  /** Adds an amount of an item, as InventoryAdd says. */
  add(id: number, amount: number): void {
    const total = (this.amounts.get(id) ?? 0) + amount;
    this.amounts.set(id, total);
    let row = this.rows.get(id);
    if (row === undefined) {
      row = document.createElement('li');
      const icon = document.createElement('img');
      icon.alt = '';
      icon.width = 24;
      icon.height = 24;
      const label = document.createElement('span');
      row.append(icon, label);
      this.rows.set(id, row);
      this.list.append(row);
      this.icon(id, icon);
    }
    const label = row.querySelector('span');
    if (label !== null) {
      label.textContent = `${id} × ${total}`;
    }
  }

  /** Loads an item's icon, icon/item/<id>. Items without one show none. */
  private icon(id: number, img: HTMLImageElement): void {
    this.store
      .bytes(`icon/item/${id}`, 'texture')
      .then((data) => {
        if (data === undefined) {
          img.style.visibility = 'hidden';
          return;
        }
        img.src = URL.createObjectURL(new Blob([data], { type: 'image/webp' }));
      })
      .catch((error: unknown) => {
        img.style.visibility = 'hidden';
        console.error(`icon ${id}:`, error);
      });
  }
}

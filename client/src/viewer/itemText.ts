import { modItem, modItemNames } from '../assets/art';
import type { AssetStore } from '../assets/store';

// Item names and descriptions from the asset build's item tables,
// table/items and table/itemdesc (docs/specs/assets-v1.md, "Tables"). The
// names and descriptions of mods' items come from the server's client
// data (art.ts) and win over the tables.
// Names load once, on first need. Descriptions are larger, so they load
// only when a description is first shown. Items the build does not name
// show as "Item <id>".
//
// Description lines keep RO's color codes: "^" and six hex digits set the
// color of the text after them. Text is only ever set with textContent,
// and colors only from the six digits, so a table cannot inject markup.

/** An item's entry in table/items. */
export interface ItemName {
  /** The identified name. */
  name: string;
  /** The name before the item is identified. */
  unidentified: string;
  /** Card slots, 0 to 4. */
  slots: number;
}

export const ITEM_NAMES = 'table/items';
export const ITEM_DESCRIPTIONS = 'table/itemdesc';

/** The longest line the client keeps, in UTF-16 units. */
const MAX_LINE = 1024;

function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v);
}

/** An item ID key: a decimal number without leading zeros. */
function itemId(key: string): number | undefined {
  if (!/^(0|[1-9]\d{0,8})$/.test(key)) {
    return undefined;
  }
  return Number(key);
}

function text(v: unknown): string {
  return typeof v === 'string' ? v.slice(0, MAX_LINE) : '';
}

/** Reads table/items. Entries that do not fit are skipped. */
export function parseItemNames(raw: unknown): Map<number, ItemName> {
  const out = new Map<number, ItemName>();
  if (!isRecord(raw)) {
    return out;
  }
  for (const [key, v] of Object.entries(raw)) {
    const id = itemId(key);
    if (id === undefined || !isRecord(v)) {
      continue;
    }
    const s = v.s;
    out.set(id, {
      name: text(v.n),
      unidentified: text(v.u),
      slots: typeof s === 'number' && Number.isInteger(s) && s > 0 ? Math.min(s, 4) : 0,
    });
  }
  return out;
}

/** Reads table/itemdesc. Entries that do not fit are skipped. */
export function parseItemDescriptions(raw: unknown): Map<number, string[]> {
  const out = new Map<number, string[]>();
  if (!isRecord(raw)) {
    return out;
  }
  for (const [key, v] of Object.entries(raw)) {
    const id = itemId(key);
    if (id !== undefined && Array.isArray(v)) {
      out.set(id, v.map(text));
    }
  }
  return out;
}

/**
 * How an item shows: its identified name, with its slot count as RO
 * shows it, such as "Knife [3]". An item without a name shows its ID.
 */
export function itemLabel(id: number, info: ItemName | undefined): string {
  const name = info?.name ?? '';
  if (name === '') {
    return `Item ${id}`;
  }
  return info !== undefined && info.slots > 0 ? `${name} [${info.slots}]` : name;
}

/** An item's label with its refine level in front, as RO shows it: "+7 Knife [3]". */
export function refinedLabel(label: string, refine: number): string {
  return refine > 0 ? `+${refine} ${label}` : label;
}

/** A rolled value of an item copy, as ItemRoll (strategy section 5.4). */
export interface RollLine {
  /**
   * The roll's display_name when its item data sets one, or else the words of
   * the first static bonus that reads it, as the server sends them.
   */
  label: string;
  value: number;
  /** False for a hidden roll of a copy that is not identified yet. */
  known: boolean;
  min: number;
  max: number;
  /** "%" or "". */
  unit: string;
}

/** A random option of an item copy, as ItemOption. */
export interface OptionLine {
  label: string;
  value: number;
  unit: string;
  /** Set for an option without an amount, such as an attack element. */
  flag: boolean;
}

function signed(value: number, unit: string): string {
  return `${value < 0 ? '' : '+'}${value}${unit}`;
}

/**
 * A rolled value with its range: "Damage against bosses +31% (15 to 35)".
 * A hidden roll shows "?" in place of its value until the copy is identified.
 */
export function rollText(r: RollLine): string {
  const value = r.known ? signed(r.value, r.unit) : '?';
  return `${r.label} ${value} (${r.min} to ${r.max})`;
}

/** A random option: "MaxHP +100", or its label alone for a flag. */
export function optionText(o: OptionLine): string {
  return o.flag ? o.label : `${o.label} ${signed(o.value, o.unit)}`;
}

/** A part of a line in one color. color is "#rrggbb", or undefined before any code. */
export interface ColorRun {
  text: string;
  color: string | undefined;
}

const COLOR_CODE = /\^([0-9a-fA-F]{6})/g;

/** Splits a line at its color codes. Runs without text are left out. */
export function colorRuns(line: string): ColorRun[] {
  const out: ColorRun[] = [];
  let color: string | undefined;
  let last = 0;
  for (const m of line.matchAll(COLOR_CODE)) {
    if (m.index > last) {
      out.push({ text: line.slice(last, m.index), color });
    }
    color = `#${(m[1] ?? '000000').toLowerCase()}`;
    last = m.index + m[0].length;
  }
  if (last < line.length) {
    out.push({ text: line.slice(last), color });
  }
  return out;
}

/** A line without its color codes. */
export function stripColors(line: string): string {
  return line.replaceAll(COLOR_CODE, '');
}

/** Writes description lines into an element, one div per line, with their colors. */
export function renderDescription(parent: HTMLElement, lines: readonly string[]): void {
  parent.replaceChildren();
  for (const line of lines) {
    const div = document.createElement('div');
    for (const run of colorRuns(line)) {
      const span = document.createElement('span');
      span.textContent = run.text;
      if (run.color !== undefined) {
        span.style.color = run.color;
      }
      div.append(span);
    }
    // An empty line keeps its height.
    if (div.childElementCount === 0) {
      div.textContent = ' ';
    }
    parent.append(div);
  }
}

/**
 * Finds an item by what the player typed: an item ID, or a name. A name
 * that matches one item exactly, ignoring case, wins. Otherwise the items
 * whose name holds the text are listed, by ID, up to limit.
 */
export function findItems(
  names: ReadonlyMap<number, ItemName>,
  query: string,
  limit = 10,
): number[] {
  const q = query.trim();
  if (/^\d{1,9}$/.test(q)) {
    return [Number(q)];
  }
  const lower = q.toLowerCase();
  if (lower === '') {
    return [];
  }
  const exact: number[] = [];
  const partial: number[] = [];
  for (const [id, info] of names) {
    const name = info.name.toLowerCase();
    if (name === lower) {
      exact.push(id);
    } else if (name.includes(lower)) {
      partial.push(id);
    }
  }
  if (exact.length > 0) {
    return exact.sort((a, b) => a - b).slice(0, limit);
  }
  return partial.sort((a, b) => a - b).slice(0, limit);
}

/** The item tables of a build, loaded when first needed. */
export class ItemTexts {
  private names: Map<number, ItemName> | undefined;
  private namesLoad: Promise<void> | undefined;
  private descriptions: Promise<Map<number, string[]>> | undefined;
  private readonly listeners = new Set<() => void>();

  constructor(private readonly store: Pick<AssetStore, 'table'>) {}

  /** Calls f once the names have loaded, to show them. */
  onLoad(f: () => void): void {
    this.listeners.add(f);
  }

  /** Loads table/items, once. A build without it keeps IDs. */
  async loadNames(): Promise<void> {
    this.namesLoad ??= this.store
      .table(ITEM_NAMES)
      .then((raw) => {
        this.names = parseItemNames(raw);
        for (const f of this.listeners) {
          f();
        }
      })
      .catch((error: unknown) => {
        this.names = new Map();
        console.error(`${ITEM_NAMES}:`, error);
      });
    return this.namesLoad;
  }

  /** An item's names, if loaded. Asking starts the load. */
  info(id: number): ItemName | undefined {
    const named = modItem(id);
    if (named?.name !== undefined) {
      return { name: named.name, unidentified: named.name, slots: named.slots };
    }
    if (this.names === undefined) {
      this.loadNames().catch(() => undefined);
      return undefined;
    }
    return this.names.get(id);
  }

  /** Finds items by ID or name, as findItems. Until the names load, only IDs work. */
  find(query: string, limit = 10): number[] {
    if (this.names === undefined) {
      this.loadNames().catch(() => undefined);
    }
    const names = new Map(this.names);
    for (const [id, name] of modItemNames()) {
      names.set(id, { name, unidentified: name, slots: 0 });
    }
    return findItems(names, query, limit);
  }

  /** How an item shows now. Until the names load, it shows by ID. */
  label(id: number): string {
    return itemLabel(id, this.info(id));
  }

  /** An item's description lines. Undefined if the build has none for it. */
  async description(id: number): Promise<readonly string[] | undefined> {
    const named = modItem(id)?.description;
    if (named !== undefined) {
      return named;
    }
    this.descriptions ??= this.store
      .table(ITEM_DESCRIPTIONS)
      .then(parseItemDescriptions)
      .catch((error: unknown) => {
        console.error(`${ITEM_DESCRIPTIONS}:`, error);
        return new Map<number, string[]>();
      });
    return (await this.descriptions).get(id);
  }
}

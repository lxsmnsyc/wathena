// The store search window, as RO's shop search. player:search_stores
// makes the server send StoreSearchOpen with the searches left. The
// player chooses vending shops or buying stores, the items by name or ID,
// and may keep only results with some cards, in a price range, or with
// rolls or random options in a range. Query runs a search, which uses one
// of the searches left. Results list each store's title, owner, place,
// item, amount and price. Next page asks for more. A click on a result
// shows where the store is, or opens it when the window allows that.
// User text is only ever set with textContent.

import { StoreSearchOutcome } from '../gen/wathena/v1/event_pb';
import type { StoreSearchQueryValues } from '../net/connection';
import {
  button,
  element,
  field,
  keepKeys,
  mapLabel,
  numberInput,
  parseWhole,
  textInput,
  zenyLabel,
} from './forms';
import { type ItemIcons, rollLines, stackRow } from './inventory';
import type { ItemTexts } from './itemText';
import type { StoreSearchHitView } from './online';

/** The most items, and the most cards, one search names. */
export const SEARCH_MAX_ITEMS = 10;
/** The most roll filters, and the most random option filters, one search has. */
export const SEARCH_MAX_ROLLS = 8;
/** The highest price a search bounds. */
export const SEARCH_MAX_PRICE = 1_000_000_000;

/** What a list of item names or IDs resolved to. */
export interface ResolvedItems {
  ids: number[];
  /** Why parts did not resolve, one line each. */
  problems: string[];
}

/**
 * Resolves a comma separated list of item names or IDs. find gives the
 * items that match a part, best first, as ItemTexts.find. A part that
 * matches several items, none of them by its exact name, is a problem.
 */
export function resolveItems(
  text: string,
  find: (query: string) => number[],
  label: (itemId: number) => string,
): ResolvedItems {
  const ids: number[] = [];
  const problems: string[] = [];
  for (const raw of text.split(/[,\n]/)) {
    const part = raw.trim();
    if (part === '') {
      continue;
    }
    const found = find(part);
    const first = found[0];
    if (first === undefined) {
      problems.push(`No item is named ${part}.`);
    } else if (found.length === 1 || label(first).toLowerCase() === part.toLowerCase()) {
      if (!ids.includes(first)) {
        ids.push(first);
      }
    } else {
      const names = found.slice(0, 5).map((id) => `${label(id)} (${String(id)})`);
      problems.push(`${part} matches ${names.join(', ')}. Type more of the name, or the ID.`);
    }
  }
  return { ids, problems };
}

/** A roll or random option filter as typed: a name and a range. */
export interface RollFilterInput {
  name: string;
  min: string;
  max: string;
}

/**
 * Reads a filter: a name and a range. An empty min or max is the widest
 * bound. A filter without a name is skipped as undefined.
 */
export function parseRollFilter(
  f: RollFilterInput,
): { name: string; min: number; max: number } | undefined | string {
  const name = f.name.trim();
  if (name === '') {
    return undefined;
  }
  const bound = (t: string, empty: number): number | undefined => {
    const s = t.trim();
    if (s === '') {
      return empty;
    }
    if (!/^-?\d{1,9}$/.test(s)) {
      return undefined;
    }
    return Number(s);
  };
  const min = bound(f.min, -2147483648);
  const max = bound(f.max, 2147483647);
  if (min === undefined || max === undefined) {
    return `The range of ${name} is whole numbers.`;
  }
  if (min > max) {
    return `The range of ${name} starts above its end.`;
  }
  return { name, min, max };
}

/** Why a search cannot run, or undefined when it can. */
export function storeSearchProblem(q: StoreSearchQueryValues): string | undefined {
  if (q.itemIds.length === 0) {
    return 'Name at least one item to find.';
  }
  if (q.itemIds.length > SEARCH_MAX_ITEMS) {
    return `A search finds at most ${String(SEARCH_MAX_ITEMS)} items.`;
  }
  if (q.cardIds.length > SEARCH_MAX_ITEMS) {
    return `A search names at most ${String(SEARCH_MAX_ITEMS)} cards.`;
  }
  if (q.minPrice > 0 && q.maxPrice > 0 && q.minPrice > q.maxPrice) {
    return 'The lowest price is above the highest.';
  }
  if (q.rolls.length > SEARCH_MAX_ROLLS || q.options.length > SEARCH_MAX_ROLLS) {
    return `A search has at most ${String(SEARCH_MAX_ROLLS)} roll filters and ${String(SEARCH_MAX_ROLLS)} option filters.`;
  }
  return undefined;
}

/** The searches left, such as "3 searches left". */
export function usesText(uses: number): string {
  return uses === 1 ? '1 search left' : `${String(uses)} searches left`;
}

/** A store's place, such as "prontera (150, 180)". */
export function placeText(map: string, x: number, y: number): string {
  return `${mapLabel(map)} (${String(x)}, ${String(y)})`;
}

/** What to tell the player about a store search answer. */
export function storeSearchMessage(o: StoreSearchOutcome, map = '', x = 0, y = 0): string {
  switch (o) {
    case StoreSearchOutcome.NOTHING:
      return 'No store has that item.';
    case StoreSearchOutcome.NO_USES:
      return 'You have no searches left.';
    case StoreSearchOutcome.TOO_FAST:
      return 'You are searching too fast. Wait a moment.';
    case StoreSearchOutcome.TOO_MANY:
      return 'There are too many results. Only the first ones show.';
    case StoreSearchOutcome.GONE:
      return 'That store is gone.';
    case StoreSearchOutcome.POSITION:
      return `The store is at ${placeText(map, x, y)}.`;
    case StoreSearchOutcome.OTHER_MAP:
      return 'That store is on another map, so it cannot open from here.';
    default:
      return 'The search did nothing.';
  }
}

/** What the window asks the server to do. */
export interface StoreSearchActions {
  query: (q: StoreSearchQueryValues) => void;
  next: () => void;
  click: (index: number) => void;
  close: () => void;
}

/** A filter row of the window. */
interface FilterRow {
  row: HTMLLIElement;
  name: HTMLInputElement;
  min: HTMLInputElement;
  max: HTMLInputElement;
}

/**
 * The window on the page, section#store-search. It holds the search
 * form, output.search-uses, and ol.search-results with a Next page
 * button.
 */
export class StoreSearchWindow {
  private readonly vending = document.createElement('input');
  private readonly buying = document.createElement('input');
  private readonly items = textInput('Item names or IDs, comma separated', 32);
  private readonly cards = textInput('Card names or IDs', 32);
  private readonly minPrice = numberInput('0', 10);
  private readonly maxPrice = numberInput('0', 10);
  private readonly rolls = element('ul', 'search-filters');
  private readonly options = element('ul', 'search-filters');
  private readonly uses = element('output', 'search-uses');
  private readonly problem = element('output', 'store-problem');
  private readonly results = element('ol', 'search-results');
  private readonly nextButton = button('Next page', () => {
    this.actions.next();
  });
  private readonly status = element('output', 'search-status');
  private readonly rollRows: FilterRow[] = [];
  private readonly optionRows: FilterRow[] = [];
  private clickOpens = false;

  constructor(
    private readonly root: HTMLElement,
    private readonly actions: StoreSearchActions,
    private readonly texts: ItemTexts,
    private readonly icons: ItemIcons | undefined,
  ) {
    const kind = element('fieldset', 'search-kind');
    const legend = element('legend', '', 'Search');
    this.vending.type = 'radio';
    this.vending.name = 'store-search-kind';
    this.vending.checked = true;
    this.buying.type = 'radio';
    this.buying.name = 'store-search-kind';
    const v = document.createElement('label');
    v.append(this.vending, ' Vending shops');
    const b = document.createElement('label');
    b.append(this.buying, ' Buying stores');
    kind.append(legend, v, b);
    const form = document.createElement('form');
    form.className = 'search-form';
    const query = document.createElement('button');
    query.type = 'submit';
    query.textContent = 'Search';
    form.append(
      kind,
      field('Items', this.items),
      field('Cards', this.cards),
      field('Lowest price', this.minPrice),
      field('Highest price', this.maxPrice),
      element('div', 'search-filter-head', 'Rolls (roll name, min, max)'),
      this.rolls,
      button('Add roll filter', () => {
        this.addFilter(this.rolls, this.rollRows, 'Roll name');
      }),
      element('div', 'search-filter-head', 'Random options (option name, min, max)'),
      this.options,
      button('Add option filter', () => {
        this.addFilter(this.options, this.optionRows, 'Option name, such as VAR_MAXHPAMOUNT');
      }),
      this.problem,
      query,
    );
    form.addEventListener('submit', (e) => {
      e.preventDefault();
      this.query();
    });
    const head = element('div', 'store-head');
    head.append(
      element('h2', '', 'Store search'),
      this.uses,
      button('Close', () => {
        this.requestClose();
      }),
    );
    this.results.setAttribute('aria-label', 'Search results');
    this.nextButton.hidden = true;
    root.replaceChildren(head, form, this.status, this.results, this.nextButton);
    keepKeys(root, () => {
      this.requestClose();
    });
    root.hidden = true;
  }

  get open(): boolean {
    return !this.root.hidden;
  }

  /** Opens the window, as StoreSearchOpen. */
  show(uses: number, clickOpens: boolean): void {
    this.clickOpens = clickOpens;
    this.uses.textContent = usesText(uses);
    this.results.replaceChildren();
    this.nextButton.hidden = true;
    this.status.textContent = '';
    this.problem.textContent = '';
    this.root.hidden = false;
    // Names are needed to find items by name.
    this.texts.loadNames().catch(() => undefined);
    this.items.focus();
  }

  /** Shows a page of results, as StoreSearchResults. */
  showResults(hits: readonly StoreSearchHitView[], uses: number, more: boolean): void {
    this.uses.textContent = usesText(uses);
    const label = (id: number): string => this.texts.label(id);
    const rows = hits.map((h) => {
      const where = element('span', 'search-place', placeText(h.map, h.x, h.y));
      const price = element('span', 'value', zenyLabel(h.price));
      const store = element('div', 'search-store');
      const title = element('bdi', 'search-title');
      title.textContent = h.title;
      const owner = element('bdi', 'search-owner');
      owner.textContent = h.owner;
      store.append(title, ' · ', owner, ' · ', where);
      const rolls = rollLines(h.item, label);
      const li = stackRow(h.item, label, this.icons, price);
      li.tabIndex = 0;
      li.prepend(store);
      if (rolls.length > 0) {
        li.append(element('div', 'search-rolls', rolls.join(', ')));
      }
      li.title = `${li.title}\n${this.clickOpens ? 'Click to open the store' : 'Click to show where the store is'}`;
      const pick = (): void => {
        this.actions.click(h.index);
      };
      li.addEventListener('click', pick);
      li.addEventListener('keydown', (e) => {
        if (e.key === 'Enter') {
          e.preventDefault();
          pick();
        }
      });
      return li;
    });
    if (rows.length === 0) {
      rows.push(element('li', 'empty', 'No results.'));
    }
    this.results.replaceChildren(...rows);
    this.nextButton.hidden = !more;
    this.status.textContent = '';
  }

  /** Shows what a query or a click did, under the form. */
  showStatus(text: string): void {
    this.status.textContent = text;
  }

  hide(): void {
    this.root.hidden = true;
  }

  requestClose(): void {
    if (this.root.hidden) {
      return;
    }
    this.hide();
    this.actions.close();
  }

  private addFilter(list: HTMLUListElement, rows: FilterRow[], placeholder: string): void {
    if (rows.length >= SEARCH_MAX_ROLLS) {
      this.problem.textContent = `A search has at most ${String(SEARCH_MAX_ROLLS)} of these filters.`;
      return;
    }
    const row = document.createElement('li');
    const name = textInput(placeholder, 20);
    const min = numberInput('min', 6);
    const max = numberInput('max', 6);
    min.setAttribute('aria-label', 'Lowest value');
    max.setAttribute('aria-label', 'Highest value');
    const f: FilterRow = { row, name, min, max };
    row.append(
      name,
      min,
      max,
      button('Remove', () => {
        row.remove();
        rows.splice(rows.indexOf(f), 1);
      }),
    );
    rows.push(f);
    list.append(row);
    name.focus();
  }

  private static filters(
    rows: readonly FilterRow[],
  ): { name: string; min: number; max: number }[] | string {
    const out: { name: string; min: number; max: number }[] = [];
    for (const r of rows) {
      const f = parseRollFilter({ name: r.name.value, min: r.min.value, max: r.max.value });
      if (typeof f === 'string') {
        return f;
      }
      if (f !== undefined) {
        out.push(f);
      }
    }
    return out;
  }

  private query(): void {
    const find = (q: string): number[] => this.texts.find(q);
    const label = (id: number): string => this.texts.label(id);
    const items = resolveItems(this.items.value, find, label);
    const cards = resolveItems(this.cards.value, find, label);
    const problems = [...items.problems, ...cards.problems];
    if (problems.length > 0) {
      this.problem.textContent = problems.join(' ');
      return;
    }
    const price = (input: HTMLInputElement): number | undefined =>
      input.value.trim() === '' ? 0 : parseWhole(input.value, 0, SEARCH_MAX_PRICE);
    const minPrice = price(this.minPrice);
    const maxPrice = price(this.maxPrice);
    if (minPrice === undefined || maxPrice === undefined) {
      this.problem.textContent = 'Prices are whole numbers.';
      return;
    }
    const rolls = StoreSearchWindow.filters(this.rollRows);
    const options = StoreSearchWindow.filters(this.optionRows);
    if (typeof rolls === 'string') {
      this.problem.textContent = rolls;
      return;
    }
    if (typeof options === 'string') {
      this.problem.textContent = options;
      return;
    }
    const q: StoreSearchQueryValues = {
      buying: this.buying.checked,
      itemIds: items.ids,
      cardIds: cards.ids,
      minPrice,
      maxPrice,
      rolls,
      options,
    };
    const problem = storeSearchProblem(q);
    if (problem !== undefined) {
      this.problem.textContent = problem;
      return;
    }
    this.problem.textContent = '';
    this.actions.query(q);
  }
}

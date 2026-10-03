// The party booking panel, kept simple: a form to place the player's ad,
// a form to search ads by level and map or job, and the ads found, with
// buttons to whisper to an ad's owner or invite it to the player's
// party. User text is only ever set with textContent.

import type { BookingAdValues, BookingSearchValues } from '../net/connection';
import { cleanChat } from './chat';
import { clockTime } from './chatNotices';
import { jobName } from './hud';
import { mapLabel } from './party';

/** The highest level an ad may be for. */
export const MAX_BOOKING_LEVEL = 999;

/** The most jobs an ad may want. */
export const MAX_BOOKING_JOBS = 6;

/** The most characters an ad's note may have, as Unicode code points. */
export const MAX_BOOKING_NOTE = 80;

/** The jobs an ad can ask for: the first and second jobs, and the extended ones. */
export const BOOKING_JOBS: readonly number[] = [
  0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 14, 15, 16, 17, 18, 19, 20, 23, 24, 25,
];

/** A job's name for the booking forms. */
export function bookingJobName(id: number, names: ReadonlyMap<number, string> = new Map()): string {
  return names.get(id) ?? jobName(id);
}

/**
 * A map name typed in a form, as the server expects it: "prt_fild08"
 * becomes "map/prt_fild08", and empty stays empty. Returns undefined for
 * a name with characters no map name has.
 */
export function bookingMap(text: string): string | undefined {
  const name = text
    .trim()
    .toLowerCase()
    .replace(/^map\//, '');
  if (name === '') {
    return '';
  }
  if (!/^[a-z0-9_@.-]{1,40}$/.test(name)) {
    return undefined;
  }
  return `map/${name}`;
}

/** Reads a level typed in a form, or undefined when it is not a whole number. */
function readLevel(text: string): number | undefined {
  const t = text.trim();
  if (!/^\d{1,4}$/.test(t)) {
    return undefined;
  }
  return Number(t);
}

/** The fields of the ad form, as typed. */
export interface AdForm {
  level: string;
  map: string;
  jobs: readonly number[];
  note: string;
}

/** Checks the ad form. Returns the ad to send, or the reason it cannot be sent. */
export function checkAd(form: AdForm): { ad: BookingAdValues } | { error: string } {
  const level = readLevel(form.level);
  if (level === undefined || level < 1 || level > MAX_BOOKING_LEVEL) {
    return { error: `Type a level from 1 to ${String(MAX_BOOKING_LEVEL)}.` };
  }
  const map = bookingMap(form.map);
  if (map === undefined) {
    return { error: 'Type a map name such as prt_fild08, or leave it empty.' };
  }
  const jobs = [...new Set(form.jobs)];
  if (jobs.length > MAX_BOOKING_JOBS) {
    return { error: `Choose at most ${String(MAX_BOOKING_JOBS)} jobs.` };
  }
  let count = 0;
  for (const _ of form.note.trim()) {
    count++;
  }
  if (count > MAX_BOOKING_NOTE) {
    return { error: `The note may have at most ${String(MAX_BOOKING_NOTE)} characters.` };
  }
  return { ad: { level, map, jobs, note: cleanChat(form.note) ?? '' } };
}

/** The fields of the search form, as typed. job is 0 for any job. */
export interface SearchForm {
  level: string;
  map: string;
  job: number;
}

/** Checks the search form. Returns the search to send, or the reason it cannot be sent. */
export function checkSearch(form: SearchForm): { search: BookingSearchValues } | { error: string } {
  let level = 0;
  if (form.level.trim() !== '') {
    const l = readLevel(form.level);
    if (l === undefined || l > MAX_BOOKING_LEVEL) {
      return { error: `Type a level from 1 to ${String(MAX_BOOKING_LEVEL)}, or leave it empty.` };
    }
    level = l;
  }
  const map = bookingMap(form.map);
  if (map === undefined) {
    return { error: 'Type a map name such as prt_fild08, or leave it empty.' };
  }
  // The server searches by map or by job, not both.
  if (map !== '' && form.job !== 0) {
    return { error: 'Search by map or by job, not both.' };
  }
  return { search: { level, map, job: form.job } };
}

/** An ad as PartyBookingEntry gives it. */
export interface BookingEntryView {
  name: string;
  level: number;
  map: string;
  jobs: number[];
  note: string;
  placedAtMs: number;
}

/** An ad as PartyBookingEntry gives it. */
export interface BookingEntryInput {
  name: string;
  ad?: { level: number; map: string; jobs: number[]; note: string } | undefined;
  placedAtMs: bigint;
}

/** The ad PartyBookingEntry gives, as the panel shows it. */
export function entryView(e: BookingEntryInput): BookingEntryView {
  return {
    name: e.name,
    level: e.ad?.level ?? 0,
    map: e.ad?.map ?? '',
    jobs: [...(e.ad?.jobs ?? [])],
    note: e.ad?.note ?? '',
    placedAtMs: Number(e.placedAtMs),
  };
}

/** The notice for an ad the server refused, as PartyBookingList.refused. */
export const BOOKING_REFUSED = 'You have an ad already. Delete it before you place another.';

/** The jobs of an ad, such as "Knight, Priest", or "Any job". */
export function jobsLabel(jobs: readonly number[], names?: ReadonlyMap<number, string>): string {
  return jobs.length === 0 ? 'Any job' : jobs.map((j) => bookingJobName(j, names)).join(', ');
}

/** What the panel asks for. */
export interface BookingActions {
  register: (ad: BookingAdValues) => void;
  search: (search: BookingSearchValues) => void;
  remove: () => void;
  whisper: (name: string) => void;
  invite: (name: string) => void;
}

function field(className: string, label: string, placeholder: string): HTMLInputElement {
  const input = document.createElement('input');
  input.className = className;
  input.autocomplete = 'off';
  input.spellcheck = false;
  input.placeholder = placeholder;
  input.setAttribute('aria-label', label);
  input.addEventListener('keydown', (e) => {
    // Keys typed here are text, not game shortcuts.
    e.stopPropagation();
    if (e.key === 'Escape') {
      input.blur();
    }
  });
  return input;
}

function button(
  className: string,
  text: string,
  type: 'button' | 'submit' = 'button',
): HTMLButtonElement {
  const b = document.createElement('button');
  b.type = type;
  b.className = className;
  b.textContent = text;
  return b;
}

/**
 * The panel on the page, section#booking-window. It holds form.booking-ad,
 * div.booking-own with the player's ad and button.booking-delete,
 * form.booking-search, and ul.booking-results with one li[data-owner] per
 * ad, each with button.booking-whisper and button.booking-invite.
 */
export class BookingWindow {
  private readonly adLevel = field('booking-ad-level', 'Level', 'Level');
  private readonly adMap = field('booking-ad-map', 'Map', 'Map, such as prt_fild08');
  private readonly adNote = field('booking-ad-note', 'Note', 'Note');
  private readonly adJobs = document.createElement('fieldset');
  private readonly jobBoxes = new Map<number, HTMLInputElement>();
  private readonly own = document.createElement('div');
  private readonly searchLevel = field('booking-search-level', 'Level', 'Level');
  private readonly searchMap = field('booking-search-map', 'Map', 'Map');
  private readonly searchJob = document.createElement('select');
  private readonly results = document.createElement('ul');
  private readonly more = document.createElement('p');
  private readonly error = document.createElement('p');
  private entries: BookingEntryView[] = [];

  /** Whether the player leads a party, for the Invite buttons. */
  leader: (() => boolean) | undefined;
  /** The player's own name. */
  selfName: (() => string | undefined) | undefined;

  constructor(
    private readonly root: HTMLElement,
    private readonly actions: BookingActions,
  ) {
    const title = document.createElement('h2');
    title.textContent = 'Party booking';

    const adForm = document.createElement('form');
    adForm.className = 'booking-ad';
    const adHead = document.createElement('h3');
    adHead.textContent = 'Your ad';
    this.adLevel.inputMode = 'numeric';
    this.adLevel.maxLength = 3;
    this.adNote.maxLength = MAX_BOOKING_NOTE;
    const adLegend = document.createElement('legend');
    adLegend.textContent = `Jobs wanted (up to ${String(MAX_BOOKING_JOBS)})`;
    this.adJobs.className = 'booking-jobs';
    this.adJobs.append(adLegend);
    for (const id of BOOKING_JOBS) {
      const label = document.createElement('label');
      const box = document.createElement('input');
      box.type = 'checkbox';
      box.value = String(id);
      box.dataset.job = String(id);
      box.addEventListener('change', () => {
        this.limitJobs();
      });
      this.jobBoxes.set(id, box);
      label.append(box, ` ${bookingJobName(id)}`);
      this.adJobs.append(label);
    }
    const adRow = document.createElement('div');
    adRow.className = 'booking-row';
    adRow.append(this.adLevel, this.adMap);
    const place = button('booking-register', 'Place ad', 'submit');
    adForm.append(adHead, adRow, this.adJobs, this.adNote, place);
    adForm.addEventListener('submit', (e) => {
      e.preventDefault();
      const got = checkAd({
        level: this.adLevel.value,
        map: this.adMap.value,
        jobs: this.checkedJobs(),
        note: this.adNote.value,
      });
      if ('error' in got) {
        this.showError(got.error);
        return;
      }
      this.showError(undefined);
      this.actions.register(got.ad);
    });

    this.own.className = 'booking-own';
    this.own.hidden = true;

    const searchForm = document.createElement('form');
    searchForm.className = 'booking-search';
    const searchHead = document.createElement('h3');
    searchHead.textContent = 'Find a party';
    this.searchLevel.inputMode = 'numeric';
    this.searchLevel.maxLength = 3;
    this.searchJob.className = 'booking-search-job';
    this.searchJob.setAttribute('aria-label', 'Job');
    const any = document.createElement('option');
    any.value = '0';
    any.textContent = 'Any job';
    this.searchJob.append(any);
    for (const id of BOOKING_JOBS) {
      if (id === 0) {
        continue;
      }
      const o = document.createElement('option');
      o.value = String(id);
      o.textContent = bookingJobName(id);
      this.searchJob.append(o);
    }
    this.searchJob.addEventListener('keydown', (e) => {
      e.stopPropagation();
    });
    const searchRow = document.createElement('div');
    searchRow.className = 'booking-row';
    searchRow.append(
      this.searchLevel,
      this.searchMap,
      this.searchJob,
      button('booking-search-go', 'Search', 'submit'),
    );
    searchForm.append(searchHead, searchRow);
    searchForm.addEventListener('submit', (e) => {
      e.preventDefault();
      const got = checkSearch({
        level: this.searchLevel.value,
        map: this.searchMap.value,
        job: Number(this.searchJob.value),
      });
      if ('error' in got) {
        this.showError(got.error);
        return;
      }
      this.showError(undefined);
      this.actions.search(got.search);
    });

    this.results.className = 'booking-results';
    this.more.className = 'booking-more';
    this.more.textContent = 'More ads match. Narrow the search to see them.';
    this.more.hidden = true;
    this.error.className = 'booking-error';
    this.error.hidden = true;
    this.root.replaceChildren(
      title,
      adForm,
      this.own,
      searchForm,
      this.results,
      this.more,
      this.error,
    );
    this.showResults([], false);
  }

  get open(): boolean {
    return !this.root.hidden;
  }

  /** Opens or closes the panel. */
  toggle(): void {
    this.root.hidden = !this.root.hidden;
    if (this.open) {
      this.showResults(this.entries, !this.more.hidden);
    }
  }

  /** Shows the player's own ad, or none. */
  showOwn(ad: BookingEntryView | undefined): void {
    this.own.hidden = ad === undefined;
    if (ad === undefined) {
      this.own.replaceChildren();
      return;
    }
    const text = document.createElement('span');
    text.textContent = `Your ad: Lv ${String(ad.level)}, ${mapLabel(ad.map) || 'any map'}, ${jobsLabel(ad.jobs)}${ad.note === '' ? '' : `, "${ad.note}"`}`;
    const remove = button('booking-delete', 'Delete');
    remove.addEventListener('click', () => {
      this.actions.remove();
    });
    this.own.replaceChildren(text, ' ', remove);
  }

  /** Shows the ads a search found. more is set when it found more than it lists. */
  showResults(entries: BookingEntryView[], more: boolean): void {
    this.entries = entries;
    this.more.hidden = !more;
    const self = this.selfName?.();
    const leader = this.leader?.() ?? false;
    const rows = entries.map((e) => {
      const row = document.createElement('li');
      row.dataset.owner = e.name;
      const head = document.createElement('div');
      const name = document.createElement('bdi');
      name.className = 'booking-name';
      name.textContent = e.name;
      const info = document.createElement('span');
      info.className = 'booking-info';
      info.textContent = ` Lv ${String(e.level)}, ${mapLabel(e.map) || 'any map'}, ${clockTime(e.placedAtMs)}`;
      head.append(name, info);
      const jobs = document.createElement('div');
      jobs.className = 'booking-jobs-wanted';
      jobs.textContent = jobsLabel(e.jobs);
      row.append(head, jobs);
      if (e.note !== '') {
        const note = document.createElement('div');
        note.className = 'booking-note';
        note.textContent = e.note;
        row.append(note);
      }
      if (e.name !== self) {
        const tools = document.createElement('div');
        const whisper = button('booking-whisper', 'Whisper');
        whisper.addEventListener('click', () => {
          this.actions.whisper(e.name);
        });
        tools.append(whisper);
        if (leader) {
          const invite = button('booking-invite', 'Invite');
          invite.addEventListener('click', () => {
            this.actions.invite(e.name);
          });
          tools.append(' ', invite);
        }
        row.append(tools);
      }
      return row;
    });
    if (rows.length === 0) {
      const row = document.createElement('li');
      row.className = 'empty';
      row.textContent = 'No ads to show. Search to find some.';
      rows.push(row);
    }
    this.results.replaceChildren(...rows);
  }

  /** Shows a notice in the panel, such as a refused ad. */
  showError(text: string | undefined): void {
    this.error.textContent = text ?? '';
    this.error.hidden = text === undefined;
  }

  private checkedJobs(): number[] {
    return [...this.jobBoxes].filter(([, box]) => box.checked).map(([id]) => id);
  }

  /** Disables the unchecked jobs once MAX_BOOKING_JOBS are checked. */
  private limitJobs(): void {
    const full = this.checkedJobs().length >= MAX_BOOKING_JOBS;
    for (const box of this.jobBoxes.values()) {
      box.disabled = full && !box.checked;
    }
  }
}

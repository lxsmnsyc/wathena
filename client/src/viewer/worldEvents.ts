// World events (strategy section 8.8, proposal 0002, review decision
// 31): the runs that last now, from WorldEventList at login and the
// WorldEventNotice of each start and end. A banner shows each start and
// end for a few seconds, and a small panel lists the runs with the time
// they have left. The announcements of a run come as chat lines.

/** A run of a world event, as WorldEventInfo gives it. */
export interface WorldEventRun {
  name: string;
  displayName: string;
  /** "boss", "invasion", "treasure_hunt" or "script". */
  type: string;
  /** The map's asset name, such as map/prontera, or "". */
  map: string;
  /** When the run ends, in Unix milliseconds, or 0. */
  endsAtMs: number;
}

/** How long the banner of a start or an end shows, in milliseconds. */
export const BANNER_MS = 6000;

const TYPE_NAMES: Record<string, string> = {
  boss: 'Boss',
  invasion: 'Invasion',
  treasure_hunt: 'Treasure hunt',
};

/** The map's name as players read it: map/prontera is prontera. */
export function mapLabel(map: string): string {
  return map.replace(/^map\//, '');
}

/** The banner of a start or an end, such as "Invasion: Prontera Raid has started." */
export function noticeText(run: WorldEventRun, started: boolean, result: string): string {
  const kind = TYPE_NAMES[run.type];
  const name = kind === undefined ? run.displayName : `${kind}: ${run.displayName}`;
  if (started) {
    const where = run.map === '' ? '' : ` on ${mapLabel(run.map)}`;
    return `${name} has started${where}.`;
  }
  switch (result) {
    case 'won':
      return `${name} is over. It was won.`;
    case 'lost':
      return `${name} is over. It was lost.`;
    case 'stopped':
      return `${name} was stopped.`;
    default:
      return `${name} is over.`;
  }
}

/** The time a run has left, such as "42 min left", or "" without an end. */
export function remainingText(endsAtMs: number, nowMs: number): string {
  if (endsAtMs <= 0) {
    return '';
  }
  const left = Math.max(0, endsAtMs - nowMs);
  if (left < 60_000) {
    return 'under a minute left';
  }
  const minutes = Math.ceil(left / 60_000);
  if (minutes < 120) {
    return `${minutes} min left`;
  }
  return `${Math.round(minutes / 60)} h left`;
}

/** The runs that last, by name. */
export class EventRuns {
  private readonly runs = new Map<string, WorldEventRun>();

  /** Replaces the runs with a list, as WorldEventList. */
  set(list: WorldEventRun[]): void {
    this.runs.clear();
    for (const r of list) {
      this.runs.set(r.name, r);
    }
  }

  /** Adds or removes a run, as WorldEventNotice. */
  notice(run: WorldEventRun, started: boolean): void {
    if (started) {
      this.runs.set(run.name, run);
    } else {
      this.runs.delete(run.name);
    }
  }

  /** The runs, by display name. */
  list(): WorldEventRun[] {
    return [...this.runs.values()].sort((a, b) => a.displayName.localeCompare(b.displayName));
  }
}

/**
 * The banner and the panel of world events. clock is Unix time in
 * milliseconds on the server's clock.
 */
export class WorldEventsView {
  readonly events = new EventRuns();
  private bannerUntil = 0;

  constructor(
    private readonly banner: HTMLElement,
    private readonly panel: HTMLElement,
    private readonly clock: () => number,
  ) {}

  /** Shows a start or an end in the banner and updates the panel. */
  notice(run: WorldEventRun, started: boolean, result: string): string {
    this.events.notice(run, started);
    const text = noticeText(run, started, result);
    this.banner.textContent = text;
    this.banner.hidden = false;
    this.bannerUntil = this.clock() + BANNER_MS;
    this.render();
    return text;
  }

  /** Sets the runs that last, as WorldEventList. */
  set(list: WorldEventRun[]): void {
    this.events.set(list);
    this.render();
  }

  /** Hides the banner once its time passed, and updates the times left. */
  tick(): void {
    if (!this.banner.hidden && this.clock() >= this.bannerUntil) {
      this.banner.hidden = true;
    }
    this.render();
  }

  private render(): void {
    const runs = this.events.list();
    this.panel.hidden = runs.length === 0;
    const now = this.clock();
    const items = runs.map((r) => {
      const li = document.createElement('li');
      const name = document.createElement('strong');
      name.textContent = r.displayName;
      li.append(name);
      const details = [TYPE_NAMES[r.type] ?? 'Event'];
      if (r.map !== '') {
        details.push(mapLabel(r.map));
      }
      const left = remainingText(r.endsAtMs, now);
      if (left !== '') {
        details.push(left);
      }
      li.append(` ${details.join(', ')}`);
      return li;
    });
    const title = document.createElement('h2');
    title.textContent = 'Events';
    const ol = document.createElement('ul');
    ol.append(...items);
    this.panel.replaceChildren(title, ol);
  }
}

// The quest log window and the quest icons over NPCs (proposal 0002,
// review decision 45), as rAthena's quest window and showevent.
//
// - The server sends a QuestList after login and after each map change.
//   It replaces the log. A QuestUpdate adds or replaces one quest, and a
//   QuestRemove takes one out. Complete quests are never in the log.
// - The window lists active quests, then inactive ones. Each has its
//   title, its timer, its hunt objectives and a button that marks it
//   active or inactive.
// - A QuestIcon shows an icon over an NPC until the server removes it,
//   the NPC goes out of sight, or the map changes. The asset pack has no
//   quest icon art, so the icon is a badge with a mark.
//
// The log logic is pure, so tests can check it. User text is only ever
// set with textContent.

/** One hunt objective of a quest, as plain data. */
export interface QuestObjectiveView {
  /** The monster's name, or the text of the objective's filters. */
  label: string;
  /** The monster ID, or 0 for an objective of filters. */
  monster: number;
  count: number;
  goal: number;
  /** The monsters' level bounds, 0 for no bound. */
  minLevel: number;
  maxLevel: number;
}

/** One quest of the log, as plain data. */
export interface QuestView {
  id: number;
  title: string;
  active: boolean;
  /**
   * The time left when the message arrived, in milliseconds: 0 once the
   * limit passed, or -1 for no limit.
   */
  limitMs: number;
  /** When the message arrived, on the clock the window uses (performance.now). */
  receivedAt: number;
  objectives: QuestObjectiveView[];
}

/** The QuestInfo fields that the client reads. */
export interface QuestInfoInput {
  id: number;
  title: string;
  active: boolean;
  limitMs: bigint;
  objectives: readonly {
    label: string;
    monster: number;
    count: number;
    goal: number;
    minLevel: number;
    maxLevel: number;
  }[];
}

/** Copies a QuestInfo into plain data. now is when it arrived. */
export function questOf(q: QuestInfoInput, now: number): QuestView {
  return {
    id: q.id,
    title: q.title,
    active: q.active,
    limitMs: q.limitMs < 0n ? -1 : Number(q.limitMs),
    receivedAt: now,
    objectives: q.objectives.map((o) => ({
      label: o.label,
      monster: o.monster,
      count: o.count,
      goal: o.goal,
      minLevel: o.minLevel,
      maxLevel: o.maxLevel,
    })),
  };
}

/** Adds a quest to the log, or replaces the quest with the same ID in its place. */
export function questUpdated(log: readonly QuestView[], quest: QuestView): QuestView[] {
  const at = log.findIndex((q) => q.id === quest.id);
  if (at < 0) {
    return [...log, quest];
  }
  return log.map((q, i) => (i === at ? quest : q));
}

/** Takes a quest out of the log. */
export function questRemoved(log: readonly QuestView[], id: number): QuestView[] {
  return log.filter((q) => q.id !== id);
}

/** The time left of a quest at now, in milliseconds, or -1 for no limit. */
export function timeLeft(q: QuestView, now: number): number {
  if (q.limitMs < 0) {
    return -1;
  }
  return Math.max(0, q.limitMs - Math.max(0, now - q.receivedAt));
}

const SECOND = 1000;
const DAY = 86_400;

/**
 * The timer text of a time left: "Time left 0:59:12", "Time left 2 days
 * 3:04:05", "Time is up" at 0, or "No time limit" for -1.
 */
export function timerText(ms: number): string {
  if (ms < 0) {
    return 'No time limit';
  }
  // Rounded up, so the timer shows 0:00:00 only when the time is up.
  const total = Math.ceil(ms / SECOND);
  if (total <= 0) {
    return 'Time is up';
  }
  const days = Math.floor(total / DAY);
  const rest = total % DAY;
  const h = Math.floor(rest / 3600);
  const m = Math.floor((rest % 3600) / 60);
  const s = rest % 60;
  const clock = `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
  if (days === 0) {
    return `Time left ${clock}`;
  }
  return `Time left ${days} ${days === 1 ? 'day' : 'days'} ${clock}`;
}

/** A level range as text: "Lv 10-20", "Lv 10", "Lv 10+", "Lv 1-20", or empty. */
export function levelText(minLevel: number, maxLevel: number): string {
  if (minLevel <= 0 && maxLevel <= 0) {
    return '';
  }
  if (maxLevel <= 0) {
    return `Lv ${minLevel}+`;
  }
  const low = Math.max(minLevel, 1);
  return low === maxLevel ? `Lv ${low}` : `Lv ${low}-${maxLevel}`;
}

/** An objective as text, such as "Poring 2/3" or "Poring 2/3 Lv 10-20". */
export function objectiveText(o: QuestObjectiveView): string {
  return [o.label, `${o.count}/${o.goal}`, levelText(o.minLevel, o.maxLevel)]
    .filter((s) => s !== '')
    .join(' ');
}

/** The active quests, then the inactive ones, each in log order. */
export function questGroups(log: readonly QuestView[]): {
  active: QuestView[];
  inactive: QuestView[];
} {
  return { active: log.filter((q) => q.active), inactive: log.filter((q) => !q.active) };
}

/** How a quest icon shows: its mark and its color class. */
export interface QuestIconMark {
  /** The text of the badge, such as "!" or "?". */
  mark: string;
  /** quest, job, event or warg. */
  kind: string;
}

/** The marks of the icon names of npc:show_quest_icon, from rAthena's QTYPE_ values. */
const ICON_MARKS: Readonly<Record<string, QuestIconMark>> = {
  quest: { mark: '!', kind: 'quest' },
  quest_question: { mark: '?', kind: 'quest' },
  job: { mark: '!', kind: 'job' },
  job_question: { mark: '?', kind: 'job' },
  event: { mark: '!', kind: 'event' },
  event_question: { mark: '?', kind: 'event' },
  warg: { mark: 'W', kind: 'warg' },
  warg_face: { mark: 'W', kind: 'warg' },
};

/** How an icon name shows. A name the client does not know shows as a quest "!". */
export function questIconMark(icon: string): QuestIconMark {
  return ICON_MARKS[icon] ?? { mark: '!', kind: 'quest' };
}

/** What the quest window asks the server to do. */
export interface QuestActions {
  /** Marks a quest active or inactive. */
  setActive: (id: number, active: boolean) => void;
}

/**
 * The quest log window. It draws again when it opens and on each change,
 * and its timers count down while it is open.
 */
export class QuestWindow {
  private log: readonly QuestView[] = [];
  private readonly body = document.createElement('div');
  /** The timer of each quest with a limit, by quest ID, while the window shows it. */
  private readonly timers = new Map<number, { quest: QuestView; el: HTMLElement }>();

  constructor(
    private readonly root: HTMLElement,
    private readonly actions: QuestActions,
    /** The clock of receivedAt. */
    private readonly clock: () => number = () => performance.now(),
  ) {
    this.body.className = 'quest-body';
    root.append(this.body);
  }

  get open(): boolean {
    return !this.root.hidden;
  }

  /** Shows the quest log. It replaces the one before. */
  set(log: readonly QuestView[]): void {
    this.log = log;
    this.render();
  }

  /** Opens or closes the window. */
  toggle(): void {
    this.root.hidden = !this.root.hidden;
    this.render();
  }

  /** Counts the timers down. Call it each frame. It does nothing while the window is closed. */
  tick(): void {
    if (!this.open) {
      return;
    }
    const now = this.clock();
    for (const { quest, el } of this.timers.values()) {
      const text = timerText(timeLeft(quest, now));
      if (el.textContent !== text) {
        el.textContent = text;
        el.classList.toggle('up', text === 'Time is up');
      }
    }
  }

  /** Draws the window, while it is open. */
  render(): void {
    if (!this.open) {
      return;
    }
    this.timers.clear();
    if (this.log.length === 0) {
      const p = document.createElement('p');
      p.className = 'quest-empty';
      p.textContent = 'You have no quests.';
      this.body.replaceChildren(p);
      return;
    }
    const groups = questGroups(this.log);
    const parts: HTMLElement[] = [];
    for (const [name, list] of [
      ['Active', groups.active],
      ['Inactive', groups.inactive],
    ] as const) {
      if (list.length === 0) {
        continue;
      }
      const h = document.createElement('h3');
      h.className = 'quest-group';
      h.textContent = `${name} (${list.length})`;
      const ul = document.createElement('ul');
      ul.className = 'quest-list';
      ul.dataset.group = name.toLowerCase();
      ul.append(...list.map((q) => this.item(q)));
      parts.push(h, ul);
    }
    this.body.replaceChildren(...parts);
    this.tick();
  }

  /** One quest's row. */
  private item(q: QuestView): HTMLLIElement {
    const li = document.createElement('li');
    li.className = q.active ? 'quest active' : 'quest inactive';
    li.dataset.questId = String(q.id);
    const head = document.createElement('div');
    head.className = 'quest-head';
    const title = document.createElement('span');
    title.className = 'quest-title';
    title.textContent = q.title === '' ? `Quest ${q.id}` : q.title;
    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'quest-state';
    button.textContent = q.active ? 'Set inactive' : 'Set active';
    button.setAttribute('aria-pressed', String(q.active));
    button.addEventListener('click', () => {
      button.disabled = true;
      this.actions.setActive(q.id, !q.active);
    });
    head.append(title, button);
    const timer = document.createElement('div');
    timer.className = 'quest-timer';
    if (q.limitMs < 0) {
      timer.textContent = timerText(-1);
    } else {
      this.timers.set(q.id, { quest: q, el: timer });
    }
    li.append(head, timer);
    if (q.objectives.length > 0) {
      const ol = document.createElement('ul');
      ol.className = 'quest-objectives';
      for (const o of q.objectives) {
        const row = document.createElement('li');
        row.className =
          o.goal > 0 && o.count >= o.goal ? 'quest-objective done' : 'quest-objective';
        row.textContent = objectiveText(o);
        ol.append(row);
      }
      li.append(ol);
    }
    return li;
  }
}

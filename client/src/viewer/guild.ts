// The guild window, as RO's Alt+G window. Its tabs are Info, Members,
// Positions, Notice, Skills and Relations. Without a guild it shows a
// form to make one. What a player may do follows the player's position:
// the master does everything, and other positions may invite, expel or
// use the storage as the master allows. User text, such as names and the
// notice, is only ever set with textContent.

import { GuildOutcome } from '../gen/wathena/v1/event_pb';
import type { GuildPositionValues } from '../net/connection';
import { cleanChat, reportButton } from './chat';
import { MAX_NAME, cleanName } from './chatCommands';
import { memberDetail } from './party';

/** The most characters of a leave or expel reason, as the expulsion list keeps. */
export const MAX_REASON = 39;
/** The most characters of the notice's title and text, as mes1 and mes2. */
export const MAX_NOTICE_TITLE = 59;
export const MAX_NOTICE_TEXT = 119;
/**
 * The highest exp tax the window takes. The server checks its own rule,
 * guild_exp_limit (50 by default), which it does not send.
 */
export const MAX_EXP_TAX = 99;

/** The names of rAthena's guild skills, by skill ID, from db/re/skill_db.yml. */
export const GUILD_SKILL_NAMES: ReadonlyMap<number, string> = new Map([
  [10000, 'Official Guild Approval'],
  [10001, 'Kafra Contract'],
  [10002, 'Guardian Research'],
  [10003, 'Strengthen Guardians'],
  [10004, 'Guild Extension'],
  [10005, "Guild's Glory"],
  [10006, 'Great Leadership'],
  [10007, 'Glorious Wounds'],
  [10008, 'Cold Heart'],
  [10009, 'Sharp Gaze'],
  [10010, 'Battle Orders'],
  [10011, 'Regeneration'],
  [10012, 'Restoration'],
  [10013, 'Urgent Call'],
  [10014, 'Permanent Development'],
  [10015, 'Item Emergency Call'],
  [10016, 'Guild Storage Expansion'],
  [10017, 'Charge Shout Flag'],
  [10018, 'Charge Shout Beating'],
  [10019, 'Emergency Move'],
]);

/** A guild skill's name, such as "Guild Extension", or "Skill 10050" for one the client does not know. */
export function guildSkillName(id: number): string {
  return GUILD_SKILL_NAMES.get(id) ?? `Skill ${String(id)}`;
}

/** One guild member as the window shows it. */
export interface GuildMemberView {
  name: string;
  position: number;
  online: boolean;
  /** The map's asset name, such as map/prontera. */
  map: string;
  job: number;
  baseLevel: number;
  /** The exp the member gave the guild. */
  exp: bigint;
  master: boolean;
}

export interface GuildRelationView {
  guildId: bigint;
  name: string;
  enemy: boolean;
}

export interface GuildSkillView {
  skillId: number;
  level: number;
  maxLevel: number;
  canRaise: boolean;
}

export interface GuildNoticeView {
  title: string;
  text: string;
}

/** The player's guild. An id of 0n means the player has no guild. */
export interface GuildState {
  id: bigint;
  name: string;
  level: number;
  exp: bigint;
  nextExp: bigint;
  skillPoints: number;
  maxMembers: number;
  master: string;
  notice: GuildNoticeView;
  emblemVersion: number;
  members: GuildMemberView[];
  positions: GuildPositionValues[];
  relations: GuildRelationView[];
  skills: GuildSkillView[];
  /** The player's own position index. */
  position: number;
  averageLevel: number;
}

/** The state of a player without a guild. */
export const NO_GUILD: GuildState = {
  id: 0n,
  name: '',
  level: 0,
  exp: 0n,
  nextExp: 0n,
  skillPoints: 0,
  maxMembers: 0,
  master: '',
  notice: { title: '', text: '' },
  emblemVersion: 0,
  members: [],
  positions: [],
  relations: [],
  skills: [],
  position: 0,
  averageLevel: 0,
};

/** A member as GuildInfo and GuildMemberUpdate give it. */
export interface GuildMemberInput {
  name: string;
  position: number;
  online: boolean;
  map: string;
  job: number;
  baseLevel: number;
  exp: bigint;
  master: boolean;
}

/** A guild as GuildInfo gives it. */
export interface GuildInfoInput {
  id: bigint;
  name: string;
  level: number;
  exp: bigint;
  nextExp: bigint;
  skillPoints: number;
  maxMembers: number;
  master: string;
  notice?: GuildNoticeView | undefined;
  emblemVersion: number;
  members: readonly GuildMemberInput[];
  positions: readonly GuildPositionValues[];
  relations: readonly GuildRelationView[];
  skills: readonly GuildSkillView[];
  position: number;
  averageLevel: number;
}

function memberView(m: GuildMemberInput): GuildMemberView {
  return {
    name: m.name,
    position: m.position,
    online: m.online,
    map: m.map,
    job: m.job,
    baseLevel: m.baseLevel,
    exp: m.exp,
    master: m.master,
  };
}

/** The guild GuildInfo gives. It replaces the whole guild. */
export function guildFromInfo(info: GuildInfoInput): GuildState {
  if (info.id === 0n) {
    return NO_GUILD;
  }
  return {
    id: info.id,
    name: info.name,
    level: info.level,
    exp: info.exp,
    nextExp: info.nextExp,
    skillPoints: info.skillPoints,
    maxMembers: info.maxMembers,
    master: info.master,
    notice: { title: info.notice?.title ?? '', text: info.notice?.text ?? '' },
    emblemVersion: info.emblemVersion,
    members: info.members.map(memberView),
    positions: info.positions
      .map((p) => ({
        index: p.index,
        name: p.name,
        canInvite: p.canInvite,
        canExpel: p.canExpel,
        canUseStorage: p.canUseStorage,
        expTax: p.expTax,
      }))
      .sort((a, b) => a.index - b.index),
    relations: info.relations.map((r) => ({ guildId: r.guildId, name: r.name, enemy: r.enemy })),
    skills: info.skills.map((s) => ({
      skillId: s.skillId,
      level: s.level,
      maxLevel: s.maxLevel,
      canRaise: s.canRaise,
    })),
    position: info.position,
    averageLevel: info.averageLevel,
  };
}

/**
 * The guild after a GuildMemberUpdate. The member with the same name
 * takes the new online state, map, job, level and exp. The position and
 * the master mark stay as GuildInfo gave them. An empty map, or a job and
 * level of 0, keep the old values. A name the guild does not have, or an
 * update that changes nothing the window shows, gives the same state.
 */
export function mergeGuildMember(state: GuildState, m: GuildMemberInput): GuildState {
  const at = state.members.findIndex((x) => x.name === m.name);
  if (state.id === 0n || at < 0) {
    return state;
  }
  const old = state.members[at]!;
  const merged: GuildMemberView = {
    ...old,
    online: m.online,
    map: m.map === '' ? old.map : m.map,
    job: m.job === 0 && m.baseLevel === 0 ? old.job : m.job,
    baseLevel: m.baseLevel === 0 ? old.baseLevel : m.baseLevel,
    exp: m.exp,
  };
  if (
    merged.online === old.online &&
    merged.map === old.map &&
    merged.job === old.job &&
    merged.baseLevel === old.baseLevel &&
    merged.exp === old.exp
  ) {
    return state;
  }
  const members = [...state.members];
  members[at] = merged;
  return { ...state, members };
}

/**
 * The guild after a GuildResult. The player has no guild once it left or
 * was expelled, or once the guild was disbanded. The server sends
 * GuildInfo for other changes.
 */
export function guildAfterResult(
  state: GuildState,
  outcome: GuildOutcome,
  name: string,
  self: string | undefined,
): GuildState {
  if (outcome === GuildOutcome.DISBANDED) {
    return NO_GUILD;
  }
  if (
    (outcome === GuildOutcome.LEFT || outcome === GuildOutcome.EXPELLED) &&
    name !== '' &&
    name === self
  ) {
    return NO_GUILD;
  }
  return state;
}

/** Whether a player is the guild's master. */
export function isMaster(state: GuildState, self: string | undefined): boolean {
  return self !== undefined && state.id !== 0n && state.master === self;
}

/** What the player's position allows. */
export interface GuildPermissions {
  invite: boolean;
  expel: boolean;
  storage: boolean;
}

/** What a player may do in the guild. The master may do everything. */
export function permissions(state: GuildState, self: string | undefined): GuildPermissions {
  if (state.id === 0n) {
    return { invite: false, expel: false, storage: false };
  }
  if (isMaster(state, self)) {
    return { invite: true, expel: true, storage: true };
  }
  const p = state.positions.find((x) => x.index === state.position);
  return {
    invite: p?.canInvite ?? false,
    expel: p?.canExpel ?? false,
    storage: p?.canUseStorage ?? false,
  };
}

/** Whether the player may expel a member: not the master, and not the player itself. */
export function canExpel(
  state: GuildState,
  self: string | undefined,
  member: GuildMemberView,
): boolean {
  return permissions(state, self).expel && !member.master && member.name !== self;
}

/** The name of a position, or "Position n" for one the guild does not list. */
export function positionName(state: GuildState, index: number): string {
  return state.positions.find((p) => p.index === index)?.name ?? `Position ${String(index + 1)}`;
}

/**
 * The guild's positions, max of them: the ones the guild has, and a
 * default for each missing index.
 */
export function positionSlots(state: GuildState, max: number): GuildPositionValues[] {
  const out: GuildPositionValues[] = [];
  for (let i = 0; i < max; i++) {
    const p = state.positions.find((x) => x.index === i);
    out.push(
      p === undefined
        ? {
            index: i,
            name: `Position ${String(i + 1)}`,
            canInvite: false,
            canExpel: false,
            canUseStorage: false,
            expTax: 0,
          }
        : { ...p },
    );
  }
  return out;
}

/** Reads an exp tax: a whole number from 0 to MAX_EXP_TAX, or undefined. */
export function parseExpTax(text: string): number | undefined {
  const t = text.trim();
  if (!/^\d+$/.test(t)) {
    return undefined;
  }
  const n = Number(t);
  return n <= MAX_EXP_TAX ? n : undefined;
}

/**
 * Checks positions before they go to the server. Names are cleaned as
 * chat is, and must have 1 to MAX_NAME characters. Returns the cleaned
 * positions, or the reason for the player.
 */
export function checkPositions(
  positions: readonly GuildPositionValues[],
): { positions: GuildPositionValues[] } | { error: string } {
  const out: GuildPositionValues[] = [];
  for (const p of positions) {
    const name = cleanName(p.name);
    if (name === undefined) {
      return {
        error: `Position ${String(p.index + 1)} needs a name of 1 to ${String(MAX_NAME)} characters.`,
      };
    }
    if (!Number.isInteger(p.expTax) || p.expTax < 0 || p.expTax > MAX_EXP_TAX) {
      return {
        error: `The exp tax of ${name} must be 0 to ${String(MAX_EXP_TAX)}.`,
      };
    }
    out.push({ ...p, name });
  }
  return { positions: out };
}

/** Cuts text to a number of code points, as the server counts them. */
function cut(text: string, max: number): string {
  let out = '';
  let count = 0;
  for (const ch of text) {
    if (count === max) {
      break;
    }
    out += ch;
    count++;
  }
  return out;
}

/** Counts code points. */
function length(text: string): number {
  let count = 0;
  for (const _ of text) {
    count++;
  }
  return count;
}

/** Checks the notice. Returns the cleaned notice, or the reason for the player. */
export function checkNotice(title: string, text: string): GuildNoticeView | { error: string } {
  const t = cleanChat(title) ?? '';
  const x = cleanChat(text) ?? '';
  if (length(t) > MAX_NOTICE_TITLE) {
    return { error: `The title may have at most ${String(MAX_NOTICE_TITLE)} characters.` };
  }
  if (length(x) > MAX_NOTICE_TEXT) {
    return { error: `The notice may have at most ${String(MAX_NOTICE_TEXT)} characters.` };
  }
  return { title: t, text: x };
}

/** A leave or expel reason: cleaned as chat, and cut to MAX_REASON characters. */
export function cleanReason(text: string): string {
  return cut(cleanChat(text) ?? '', MAX_REASON);
}

/** The guild's exp as a percent of the next level, from 0 to 100, rounded down. 100 at the top level. */
export function expPercent(exp: bigint, nextExp: bigint): number {
  if (nextExp <= 0n) {
    return 100;
  }
  if (exp <= 0n) {
    return 0;
  }
  const p = Number((exp * 100n) / nextExp);
  return Math.min(100, p);
}

/** The member count, such as "(3 / 16)". */
export function guildMemberCount(state: GuildState, fallbackMax: number): string {
  const max = state.maxMembers > 0 ? state.maxMembers : fallbackMax;
  return `(${String(state.members.length)} / ${String(max)})`;
}

/** The allied or the enemy guilds. */
export function relationsOf(state: GuildState, enemy: boolean): GuildRelationView[] {
  return state.relations.filter((r) => r.enemy === enemy);
}

/** The line under a player's name for its guild, as "Guild [Position]". */
export function badgeLine(badge: { name: string; position: string }): string {
  return badge.position === '' ? badge.name : `${badge.name} [${badge.position}]`;
}

/** The window's tabs, in order. */
export const GUILD_TABS = [
  'info',
  'members',
  'positions',
  'notice',
  'skills',
  'relations',
] as const;
export type GuildTab = (typeof GUILD_TABS)[number];

const TAB_LABELS: Record<GuildTab, string> = {
  info: 'Info',
  members: 'Members',
  positions: 'Positions',
  notice: 'Notice',
  skills: 'Skills',
  relations: 'Relations',
};

/** What the guild window asks for. */
export interface GuildActions {
  create: (name: string) => void;
  invite: (name: string) => void;
  leave: (reason: string) => void;
  expel: (name: string, reason: string) => void;
  /** Gives a member a position. Position 0 hands the guild over. */
  setPosition: (name: string, position: number) => void;
  positions: (positions: GuildPositionValues[]) => void;
  notice: (title: string, text: string) => void;
  skillUp: (skillId: number) => void;
  ally: (name: string) => void;
  enemy: (name: string) => void;
  endRelation: (guildId: bigint, enemy: boolean) => void;
  disband: (name: string) => void;
  /** Reports another guild's name, or its emblem with emblem set. */
  report: (guildId: bigint, emblem: boolean) => void;
}

/** The title of a button that reports another guild's name or emblem. */
export function guildReportTitle(name: string, emblem: boolean): string {
  return emblem ? `Report the emblem of the guild ${name}` : `Report the guild name ${name}`;
}

/** The limits that size the window, from Welcome.limits. */
export interface GuildLimits {
  guildMembers: number;
  guildPositions: number;
  guildAllies: number;
  guildEnemies: number;
}

/** A question the window asks before an action, with an optional text field. */
interface Ask {
  text: string;
  /** A text field, such as a reason. */
  field?: { label: string; maxLength: number; placeholder: string };
  ok: string;
  /** Called with the field's value. Returns an error to keep the question open. */
  answer: (value: string) => string | undefined;
}

/** A text field whose keys are not game shortcuts. */
function field(
  input: HTMLInputElement | HTMLTextAreaElement,
  label: string,
  max: number,
  placeholder = '',
): void {
  input.autocomplete = 'off';
  input.spellcheck = false;
  input.maxLength = max;
  input.placeholder = placeholder;
  input.setAttribute('aria-label', label);
  const el: HTMLElement = input;
  el.addEventListener('keydown', (e) => {
    e.stopPropagation();
    if (e.key === 'Escape') {
      input.blur();
    }
  });
}

function button(text: string, className: string, title = ''): HTMLButtonElement {
  const b = document.createElement('button');
  b.type = 'button';
  b.className = className;
  b.textContent = text;
  if (title !== '') {
    b.title = title;
  }
  return b;
}

function span(text: string, className: string): HTMLSpanElement {
  const s = document.createElement('span');
  s.className = className;
  s.textContent = text;
  return s;
}

/** A name in its own bidirectional isolate. */
function bdi(text: string, className: string): HTMLElement {
  const e = document.createElement('bdi');
  e.className = className;
  e.textContent = text;
  return e;
}

/**
 * The window on the page, section#guild-window. It holds h2 with the
 * guild's name, div.guild-tabs with one button[data-guild-tab] per tab,
 * one div.guild-panel[data-guild-panel] per tab, form.guild-create
 * without a guild, div.guild-ask for questions, and p.guild-error.
 */
export class GuildWindow {
  private readonly title = document.createElement('h2');
  private readonly tabBar = document.createElement('div');
  private readonly tabButtons = new Map<GuildTab, HTMLButtonElement>();
  private readonly panels = new Map<GuildTab, HTMLDivElement>();
  private readonly createForm = document.createElement('form');
  private readonly createName = document.createElement('input');
  private readonly askBox = document.createElement('div');
  /** The invite and relation forms stay, so typing is not lost on redraws. */
  private readonly inviteForm = document.createElement('form');
  private readonly relationForm = document.createElement('form');
  private readonly error = document.createElement('p');
  private tab: GuildTab = 'info';
  private state: GuildState = NO_GUILD;
  /** What the Positions and Notice forms last drew, so typing is not lost on redraws. */
  private positionsDrawn = '';
  private noticeDrawn = '';

  /** The player's own name, to know the player's rights. */
  selfName: (() => string | undefined) | undefined;
  /** Job names from the server, which win over the client's. */
  jobNames: ReadonlyMap<number, string> = new Map();
  /** The URL of a loaded emblem, by guild ID and version. */
  emblem: ((guildId: bigint, version: number) => string | undefined) | undefined;

  constructor(
    private readonly root: HTMLElement,
    private readonly actions: GuildActions,
    private limits: GuildLimits,
  ) {
    this.title.className = 'guild-title';
    this.tabBar.className = 'guild-tabs';
    this.tabBar.setAttribute('role', 'tablist');
    for (const t of GUILD_TABS) {
      const b = button(TAB_LABELS[t], 'guild-tab');
      b.dataset.guildTab = t;
      b.setAttribute('role', 'tab');
      b.addEventListener('click', () => {
        this.showTab(t);
      });
      this.tabButtons.set(t, b);
      this.tabBar.append(b);
      const panel = document.createElement('div');
      panel.className = 'guild-panel';
      panel.dataset.guildPanel = t;
      panel.setAttribute('role', 'tabpanel');
      this.panels.set(t, panel);
    }

    this.createForm.className = 'guild-create';
    field(this.createName, 'Guild name', MAX_NAME, 'Guild name');
    this.createName.className = 'guild-create-name';
    const make = document.createElement('button');
    make.type = 'submit';
    make.textContent = 'Create guild';
    const hint = document.createElement('p');
    hint.className = 'guild-hint';
    hint.textContent = 'Making a guild may take an Emperium.';
    const row = document.createElement('div');
    row.className = 'guild-row';
    row.append(this.createName, make);
    this.createForm.append(row, hint);
    this.createForm.addEventListener('submit', (e) => {
      e.preventDefault();
      const name = cleanName(this.createName.value);
      if (name === undefined) {
        this.showError(`Type a guild name of 1 to ${String(MAX_NAME)} characters.`);
        return;
      }
      this.showError(undefined);
      this.actions.create(name);
      this.createName.value = '';
    });

    this.inviteForm.className = 'guild-invite guild-row';
    const inviteName = document.createElement('input');
    inviteName.className = 'guild-invite-name';
    field(inviteName, 'Name to invite', MAX_NAME, 'Character name');
    const invite = document.createElement('button');
    invite.type = 'submit';
    invite.textContent = 'Invite';
    this.inviteForm.append(inviteName, invite);
    this.inviteForm.addEventListener('submit', (e) => {
      e.preventDefault();
      const n = cleanName(inviteName.value);
      if (n === undefined) {
        this.showError(`Type a name of 1 to ${String(MAX_NAME)} characters.`);
        return;
      }
      const g = this.state;
      const max = g.maxMembers > 0 ? g.maxMembers : this.limits.guildMembers;
      if (g.members.length >= max) {
        this.showError('The guild is full.');
        return;
      }
      this.showError(undefined);
      this.actions.invite(n);
      inviteName.value = '';
    });

    this.relationForm.className = 'guild-relation-form guild-row';
    const player = document.createElement('input');
    player.className = 'guild-relation-player';
    field(player, 'Player of the other guild', MAX_NAME, 'Player name');
    const ally = document.createElement('button');
    ally.type = 'submit';
    ally.className = 'guild-ally';
    ally.textContent = 'Ally';
    ally.title = "Ask this online player's guild to be allies";
    const enemy = button('Enemy', 'guild-enemy', "Make this online player's guild an enemy");
    const read = (): string | undefined => {
      const n = cleanName(player.value);
      if (n === undefined) {
        this.showError(`Type a name of 1 to ${String(MAX_NAME)} characters.`);
        return undefined;
      }
      this.showError(undefined);
      player.value = '';
      return n;
    };
    this.relationForm.addEventListener('submit', (e) => {
      e.preventDefault();
      const n = read();
      if (n !== undefined) {
        this.actions.ally(n);
      }
    });
    enemy.addEventListener('click', () => {
      const n = read();
      if (n !== undefined) {
        this.actions.enemy(n);
      }
    });
    this.relationForm.append(player, ally, enemy);

    this.askBox.className = 'guild-ask';
    this.askBox.hidden = true;
    this.askBox.setAttribute('role', 'alertdialog');
    this.error.className = 'guild-error';
    this.error.hidden = true;
    this.root.replaceChildren(
      this.title,
      this.tabBar,
      ...this.panels.values(),
      this.createForm,
      this.askBox,
      this.error,
    );
    this.render();
  }

  get open(): boolean {
    return !this.root.hidden;
  }

  /** The guild the window shows. */
  get guild(): GuildState {
    return this.state;
  }

  /** Changes the limits that size the window. */
  setLimits(limits: GuildLimits): void {
    this.limits = limits;
    this.render();
  }

  /** Shows a guild. */
  show(state: GuildState): void {
    if (state.id !== this.state.id) {
      this.closeAsk();
    }
    this.state = state;
    this.render();
  }

  /** Opens or closes the window. */
  toggle(): void {
    this.root.hidden = !this.root.hidden;
    if (this.open) {
      this.render();
    } else {
      this.closeAsk();
    }
  }

  /** Shows a tab. */
  showTab(tab: GuildTab): void {
    this.tab = tab;
    this.render();
  }

  /** Draws the window again, such as after the player's name is known. */
  render(): void {
    const s = this.state;
    const inGuild = s.id !== 0n;
    const self = this.selfName?.();
    const master = isMaster(s, self);
    this.root.dataset.guild = inGuild ? 'yes' : 'no';
    this.root.dataset.master = master ? 'yes' : 'no';
    this.title.textContent = inGuild ? s.name : 'Guild';
    this.tabBar.hidden = !inGuild;
    this.createForm.hidden = inGuild;
    for (const [t, b] of this.tabButtons) {
      const active = t === this.tab;
      b.classList.toggle('active', active);
      b.setAttribute('aria-selected', String(active));
    }
    for (const [t, panel] of this.panels) {
      panel.hidden = !inGuild || t !== this.tab;
    }
    if (!inGuild || !this.open) {
      this.positionsDrawn = '';
      this.noticeDrawn = '';
      return;
    }
    const panel = this.panels.get(this.tab)!;
    switch (this.tab) {
      case 'info':
        this.drawInfo(panel, master);
        break;
      case 'members':
        this.drawMembers(panel, self, master);
        break;
      case 'positions':
        this.drawPositions(panel, master);
        break;
      case 'notice':
        this.drawNotice(panel, master);
        break;
      case 'skills':
        this.drawSkills(panel, master);
        break;
      case 'relations':
        this.drawRelations(panel, master);
        break;
    }
  }

  private drawInfo(panel: HTMLDivElement, master: boolean): void {
    const s = this.state;
    const head = document.createElement('div');
    head.className = 'guild-info-head';
    const img = document.createElement('img');
    img.className = 'guild-emblem';
    img.alt = '';
    img.width = 24;
    img.height = 24;
    const url = this.emblem?.(s.id, s.emblemVersion);
    if (url === undefined) {
      img.hidden = true;
    } else {
      img.crossOrigin = 'anonymous';
      img.src = url;
    }
    const name = bdi(s.name, 'guild-info-name');
    head.append(img, name);

    const list = document.createElement('dl');
    list.className = 'guild-info';
    const add = (term: string, value: string | HTMLElement): void => {
      const dt = document.createElement('dt');
      dt.textContent = term;
      const dd = document.createElement('dd');
      dd.append(value);
      list.append(dt, dd);
    };
    add('Level', String(s.level));
    const bar = document.createElement('div');
    bar.className = 'guild-exp';
    const percent = expPercent(s.exp, s.nextExp);
    bar.setAttribute('role', 'meter');
    bar.setAttribute('aria-label', 'Guild exp');
    bar.setAttribute('aria-valuemin', '0');
    bar.setAttribute('aria-valuemax', '100');
    bar.setAttribute('aria-valuenow', String(percent));
    const fill = document.createElement('div');
    fill.className = 'guild-exp-fill';
    fill.style.width = `${String(percent)}%`;
    bar.append(fill);
    const expText =
      s.nextExp > 0n ? `${String(s.exp)} / ${String(s.nextExp)} (${String(percent)}%)` : 'Max';
    const exp = document.createElement('div');
    exp.append(bar, span(expText, 'guild-exp-text'));
    add('Exp', exp);
    add('Members', guildMemberCount(s, this.limits.guildMembers));
    add('Average level', String(s.averageLevel));
    const masterName = document.createElement('bdi');
    masterName.textContent = s.master;
    add('Master', masterName);
    add('Your position', positionName(s, s.position));

    const actions = document.createElement('div');
    actions.className = 'guild-row guild-info-actions';
    if (master) {
      const disband = button('Disband guild', 'guild-disband');
      disband.addEventListener('click', () => {
        this.ask({
          text: `Type the guild name ${s.name} to disband it. Only a guild with no other members can be disbanded.`,
          field: { label: 'Guild name', maxLength: MAX_NAME, placeholder: s.name },
          ok: 'Disband',
          answer: (value) => {
            if (value.trim() !== s.name) {
              return 'The name does not match the guild name.';
            }
            this.actions.disband(s.name);
            return undefined;
          },
        });
      });
      actions.append(disband);
    } else {
      const leave = button('Leave guild', 'guild-leave');
      leave.addEventListener('click', () => {
        this.ask({
          text: `Leave ${s.name}? You can give a reason.`,
          field: { label: 'Reason', maxLength: MAX_REASON, placeholder: 'Reason' },
          ok: 'Leave',
          answer: (value) => {
            this.actions.leave(cleanReason(value));
            return undefined;
          },
        });
      });
      actions.append(leave);
    }
    panel.replaceChildren(head, list, actions);
  }

  private drawMembers(panel: HTMLDivElement, self: string | undefined, master: boolean): void {
    const s = this.state;
    const rights = permissions(s, self);
    const count = document.createElement('p');
    count.className = 'guild-count';
    count.textContent = `Members ${guildMemberCount(s, this.limits.guildMembers)}`;
    const list = document.createElement('ul');
    list.className = 'guild-members';
    for (const m of s.members) {
      list.append(this.memberRow(m, self, master));
    }
    const parts: HTMLElement[] = [count, list];
    if (rights.invite) {
      parts.push(this.inviteForm);
    }
    panel.replaceChildren(...parts);
  }

  private memberRow(m: GuildMemberView, self: string | undefined, master: boolean): HTMLLIElement {
    const s = this.state;
    const row = document.createElement('li');
    row.dataset.member = m.name;
    row.classList.toggle('offline', !m.online);
    const head = document.createElement('div');
    head.className = 'guild-member-head';
    const dot = document.createElement('span');
    dot.className = `online-dot${m.online ? ' online' : ''}`;
    dot.title = m.online ? 'Online' : 'Offline';
    head.append(dot, bdi(m.name, 'guild-member-name'));
    if (m.master) {
      const mark = span('★', 'guild-master');
      mark.title = 'Guild master';
      mark.setAttribute('aria-label', 'Guild master');
      head.append(' ', mark);
    }
    head.append(' ', bdi(positionName(s, m.position), 'guild-member-position'));
    const detail = document.createElement('div');
    detail.className = 'guild-detail';
    detail.textContent = `${memberDetail(m, this.jobNames)}, exp given ${String(m.exp)}`;
    row.append(head, detail);
    const tools = document.createElement('div');
    tools.className = 'guild-member-tools';
    if (master && m.name !== self) {
      const select = document.createElement('select');
      select.className = 'guild-member-select';
      select.setAttribute('aria-label', `Position of ${m.name}`);
      for (const p of positionSlots(s, this.limits.guildPositions)) {
        const option = document.createElement('option');
        option.value = String(p.index);
        option.textContent = p.index === 0 ? `${p.name} (hand over the guild)` : p.name;
        select.append(option);
      }
      select.value = String(m.position);
      select.addEventListener('keydown', (e) => {
        e.stopPropagation();
      });
      select.addEventListener('change', () => {
        const position = Number(select.value);
        if (position === 0) {
          select.value = String(m.position);
          this.ask({
            text: `Hand the guild over to ${m.name}? ${m.name} becomes the guild master.`,
            ok: 'Hand over',
            answer: () => {
              this.actions.setPosition(m.name, 0);
              return undefined;
            },
          });
          return;
        }
        this.actions.setPosition(m.name, position);
      });
      tools.append(select);
    }
    if (canExpel(s, self, m)) {
      const expel = button('Expel', 'guild-expel', `Expel ${m.name}`);
      expel.addEventListener('click', () => {
        this.ask({
          text: `Expel ${m.name} from the guild? Give a reason.`,
          field: { label: 'Reason', maxLength: MAX_REASON, placeholder: 'Reason' },
          ok: 'Expel',
          answer: (value) => {
            this.actions.expel(m.name, cleanReason(value));
            return undefined;
          },
        });
      });
      tools.append(expel);
    }
    if (tools.childElementCount > 0) {
      row.append(tools);
    }
    return row;
  }

  private drawPositions(panel: HTMLDivElement, master: boolean): void {
    const slots = positionSlots(this.state, this.limits.guildPositions);
    const key = `${String(master)}:${JSON.stringify(slots)}`;
    if (key === this.positionsDrawn && panel.childElementCount > 0) {
      return;
    }
    this.positionsDrawn = key;
    const form = document.createElement('form');
    form.className = 'guild-positions';
    const table = document.createElement('table');
    const head = document.createElement('tr');
    for (const text of ['#', 'Name', 'Invite', 'Expel', 'Storage', 'Exp tax %']) {
      const th = document.createElement('th');
      th.textContent = text;
      head.append(th);
    }
    table.append(head);
    const rows: {
      index: number;
      name: HTMLInputElement;
      invite: HTMLInputElement;
      expel: HTMLInputElement;
      storage: HTMLInputElement;
      tax: HTMLInputElement;
    }[] = [];
    for (const p of slots) {
      const tr = document.createElement('tr');
      tr.dataset.position = String(p.index);
      const number = document.createElement('td');
      number.textContent = String(p.index + 1);
      const name = document.createElement('input');
      name.className = 'guild-position-name';
      field(name, `Name of position ${String(p.index + 1)}`, MAX_NAME);
      name.value = p.name;
      const box = (checked: boolean, label: string): HTMLInputElement => {
        const b = document.createElement('input');
        b.type = 'checkbox';
        b.checked = checked;
        b.setAttribute('aria-label', `${label} for ${p.name}`);
        return b;
      };
      const invite = box(p.canInvite, 'Invite');
      const expel = box(p.canExpel, 'Expel');
      const storage = box(p.canUseStorage, 'Storage');
      const tax = document.createElement('input');
      tax.type = 'number';
      tax.min = '0';
      tax.max = String(MAX_EXP_TAX);
      tax.className = 'guild-position-tax';
      tax.value = String(p.expTax);
      tax.setAttribute('aria-label', `Exp tax of ${p.name}`);
      tax.addEventListener('keydown', (e) => {
        e.stopPropagation();
      });
      const cells = [name, invite, expel, storage, tax].map((el) => {
        const td = document.createElement('td');
        td.append(el);
        el.disabled = !master;
        return td;
      });
      tr.append(number, ...cells);
      table.append(tr);
      rows.push({ index: p.index, name, invite, expel, storage, tax });
    }
    const save = document.createElement('button');
    save.type = 'submit';
    save.textContent = 'Save positions';
    save.hidden = !master;
    const hint = document.createElement('p');
    hint.className = 'guild-hint';
    hint.textContent = master
      ? 'The server limits the exp tax by its own rule.'
      : 'Only the guild master can change the positions.';
    form.append(table, save, hint);
    form.addEventListener('submit', (e) => {
      e.preventDefault();
      const values: GuildPositionValues[] = [];
      for (const r of rows) {
        const tax = parseExpTax(r.tax.value);
        values.push({
          index: r.index,
          name: r.name.value,
          canInvite: r.invite.checked,
          canExpel: r.expel.checked,
          canUseStorage: r.storage.checked,
          expTax: tax ?? -1,
        });
      }
      const checked = checkPositions(values);
      if ('error' in checked) {
        this.showError(checked.error);
        return;
      }
      this.showError(undefined);
      this.actions.positions(checked.positions);
    });
    panel.replaceChildren(form);
  }

  private drawNotice(panel: HTMLDivElement, master: boolean): void {
    const n = this.state.notice;
    const key = `${String(master)}:${n.title}\n${n.text}`;
    if (key === this.noticeDrawn && panel.childElementCount > 0) {
      return;
    }
    this.noticeDrawn = key;
    if (!master) {
      const title = document.createElement('h3');
      title.className = 'guild-notice-title';
      title.textContent = n.title === '' ? 'No notice' : n.title;
      const text = document.createElement('p');
      text.className = 'guild-notice-text';
      text.textContent = n.text;
      panel.replaceChildren(title, text);
      return;
    }
    const form = document.createElement('form');
    form.className = 'guild-notice';
    const title = document.createElement('input');
    title.className = 'guild-notice-title';
    field(title, 'Notice title', MAX_NOTICE_TITLE, 'Title');
    title.value = n.title;
    const text = document.createElement('textarea');
    text.className = 'guild-notice-text';
    text.rows = 3;
    field(text, 'Notice text', MAX_NOTICE_TEXT, 'Notice');
    text.value = n.text;
    const save = document.createElement('button');
    save.type = 'submit';
    save.textContent = 'Save notice';
    form.append(title, text, save);
    form.addEventListener('submit', (e) => {
      e.preventDefault();
      const checked = checkNotice(title.value, text.value);
      if ('error' in checked) {
        this.showError(checked.error);
        return;
      }
      this.showError(undefined);
      this.actions.notice(checked.title, checked.text);
    });
    panel.replaceChildren(form);
  }

  private drawSkills(panel: HTMLDivElement, master: boolean): void {
    const s = this.state;
    const points = document.createElement('p');
    points.className = 'guild-skill-points';
    points.textContent = `Skill points: ${String(s.skillPoints)}`;
    const list = document.createElement('ul');
    list.className = 'guild-skills';
    for (const k of s.skills) {
      const li = document.createElement('li');
      li.dataset.skill = String(k.skillId);
      li.append(
        span(guildSkillName(k.skillId), 'guild-skill-name'),
        ' ',
        span(`Lv ${String(k.level)} / ${String(k.maxLevel)}`, 'guild-skill-level'),
      );
      if (master) {
        const up = button('+', 'guild-skill-up', `Raise ${guildSkillName(k.skillId)}`);
        up.disabled = !k.canRaise;
        up.addEventListener('click', () => {
          this.actions.skillUp(k.skillId);
        });
        li.append(' ', up);
      }
      list.append(li);
    }
    panel.replaceChildren(points, list);
  }

  private drawRelations(panel: HTMLDivElement, master: boolean): void {
    const s = this.state;
    const parts: HTMLElement[] = [];
    for (const enemy of [false, true]) {
      const list = relationsOf(s, enemy);
      const max = enemy ? this.limits.guildEnemies : this.limits.guildAllies;
      const h = document.createElement('h3');
      h.textContent = `${enemy ? 'Enemies' : 'Allies'} (${String(list.length)} / ${String(max)})`;
      const ul = document.createElement('ul');
      ul.className = enemy ? 'guild-enemies' : 'guild-allies';
      for (const r of list) {
        const li = document.createElement('li');
        li.dataset.guildId = String(r.guildId);
        li.append(bdi(r.name, 'guild-relation-name'));
        if (master) {
          const end = button(
            enemy ? 'End' : 'Break',
            'guild-end-relation',
            `End the relation with ${r.name}`,
          );
          end.addEventListener('click', () => {
            this.actions.endRelation(r.guildId, r.enemy);
          });
          li.append(' ', end);
        }
        for (const emblem of [false, true]) {
          li.append(
            ' ',
            reportButton(emblem ? '!E' : '!', guildReportTitle(r.name, emblem), () => {
              this.actions.report(r.guildId, emblem);
            }),
          );
        }
        ul.append(li);
      }
      if (list.length === 0) {
        const li = document.createElement('li');
        li.className = 'empty';
        li.textContent = 'None';
        ul.append(li);
      }
      parts.push(h, ul);
    }
    if (master) {
      parts.push(this.relationForm);
    }
    panel.replaceChildren(...parts);
  }

  /** Asks a question in the window before an action. */
  private ask(a: Ask): void {
    const text = document.createElement('p');
    text.textContent = a.text;
    const parts: HTMLElement[] = [text];
    let input: HTMLInputElement | undefined;
    if (a.field !== undefined) {
      input = document.createElement('input');
      input.className = 'guild-ask-field';
      field(input, a.field.label, a.field.maxLength, a.field.placeholder);
      parts.push(input);
    }
    const ok = button(a.ok, 'guild-ask-ok');
    const cancel = button('Cancel', 'guild-ask-cancel');
    const row = document.createElement('div');
    row.className = 'guild-row';
    row.append(ok, cancel);
    parts.push(row);
    const submit = (): void => {
      const error = a.answer(input?.value ?? '');
      if (error !== undefined) {
        this.showError(error);
        return;
      }
      this.showError(undefined);
      this.closeAsk();
    };
    ok.addEventListener('click', submit);
    input?.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') {
        e.preventDefault();
        submit();
      }
    });
    cancel.addEventListener('click', () => {
      this.closeAsk();
    });
    this.askBox.replaceChildren(...parts);
    this.askBox.hidden = false;
    (input ?? ok).focus();
  }

  private closeAsk(): void {
    this.askBox.hidden = true;
    this.askBox.replaceChildren();
  }

  private showError(text: string | undefined): void {
    this.error.textContent = text ?? '';
    this.error.hidden = text === undefined;
  }
}

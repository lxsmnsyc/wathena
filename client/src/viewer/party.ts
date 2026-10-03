// The party window: the party's name, its members in join order with
// the leader marked, each member's online state, map, job, base level and
// HP bar, and the leader's tools: invite, expel, make leader and the
// sharing options. Without a party it shows a form to make one. User text
// is only ever set with textContent.

import { PartyOutcome } from '../gen/wathena/v1/event_pb';
import type { PartyOptionValues } from '../net/connection';
import { MAX_NAME, cleanName } from './chatCommands';
import { jobName } from './hud';

/** One party member as the window shows it. */
export interface PartyMemberView {
  name: string;
  leader: boolean;
  online: boolean;
  /** The map's asset name, such as map/prontera. */
  map: string;
  job: number;
  baseLevel: number;
  hp: number;
  maxHp: number;
}

/** The player's party. An id of 0n means the player has no party. */
export interface PartyState {
  id: bigint;
  name: string;
  options: PartyOptionValues;
  members: PartyMemberView[];
}

/** The state of a player without a party. */
export const NO_PARTY: PartyState = {
  id: 0n,
  name: '',
  options: { expEven: false, itemEven: false, pickupShared: false },
  members: [],
};

/** A party member as PartyInfo and PartyMemberUpdate give it. */
export interface PartyMemberInput {
  name: string;
  leader: boolean;
  online: boolean;
  map: string;
  job: number;
  baseLevel: number;
  hp: bigint;
  maxHp: bigint;
}

/** A party as PartyInfo gives it. */
export interface PartyInfoInput {
  id: bigint;
  name: string;
  options?: PartyOptionValues | undefined;
  members: PartyMemberInput[];
}

function memberView(m: PartyMemberInput): PartyMemberView {
  return {
    name: m.name,
    leader: m.leader,
    online: m.online,
    map: m.map,
    job: m.job,
    baseLevel: m.baseLevel,
    hp: Number(m.hp),
    maxHp: Number(m.maxHp),
  };
}

/** The party PartyInfo gives. It replaces the whole party. */
export function partyFromInfo(info: PartyInfoInput): PartyState {
  if (info.id === 0n) {
    return NO_PARTY;
  }
  const o = info.options;
  return {
    id: info.id,
    name: info.name,
    options: {
      expEven: o?.expEven ?? false,
      itemEven: o?.itemEven ?? false,
      pickupShared: o?.pickupShared ?? false,
    },
    members: info.members.map(memberView),
  };
}

/**
 * The party after a PartyMemberUpdate. The member with the same name
 * takes the new values. The leader mark stays as PartyInfo gave it, and
 * an empty map, a job or level of 0, or a max HP of 0 keep the old value,
 * since no member has those. A name the party does not have changes
 * nothing: PartyInfo adds members.
 */
export function mergeMember(state: PartyState, m: PartyMemberInput): PartyState {
  const at = state.members.findIndex((x) => x.name === m.name);
  if (state.id === 0n || at < 0) {
    return state;
  }
  const old = state.members[at]!;
  const next = memberView(m);
  const merged: PartyMemberView = {
    name: old.name,
    leader: old.leader,
    online: next.online,
    map: next.map === '' ? old.map : next.map,
    job: next.job === 0 && next.baseLevel === 0 ? old.job : next.job,
    baseLevel: next.baseLevel === 0 ? old.baseLevel : next.baseLevel,
    hp: next.maxHp === 0 ? old.hp : next.hp,
    maxHp: next.maxHp === 0 ? old.maxHp : next.maxHp,
  };
  const members = [...state.members];
  members[at] = merged;
  return { ...state, members };
}

/**
 * The party after a PartyResult. The player has no party once it left or
 * was expelled, or once the party was disbanded. The server sends
 * PartyInfo for other changes.
 */
export function partyAfterResult(
  state: PartyState,
  outcome: PartyOutcome,
  name: string,
  self: string | undefined,
): PartyState {
  if (outcome === PartyOutcome.DISBANDED) {
    return NO_PARTY;
  }
  if (
    (outcome === PartyOutcome.LEFT || outcome === PartyOutcome.EXPELLED) &&
    name !== '' &&
    name === self
  ) {
    return NO_PARTY;
  }
  return state;
}

/** The party's leader, if it has one. */
export function leaderOf(state: PartyState): PartyMemberView | undefined {
  return state.members.find((m) => m.leader);
}

/** Whether a player leads the party. */
export function leads(state: PartyState, name: string | undefined): boolean {
  return name !== undefined && state.id !== 0n && leaderOf(state)?.name === name;
}

/** A member's HP as a percent from 0 to 100, rounded down. */
export function hpPercent(hp: number, maxHp: number): number {
  if (maxHp <= 0 || hp <= 0) {
    return 0;
  }
  return Math.min(100, Math.floor((hp * 100) / maxHp));
}

/** A map's name without "map/", such as prontera. */
export function mapLabel(map: string): string {
  return map.replace(/^map\//, '');
}

/** A member's details, such as "Lv 50 Knight, prontera". */
export function memberDetail(
  m: Pick<PartyMemberView, 'job' | 'baseLevel' | 'map'>,
  names: ReadonlyMap<number, string> = new Map(),
): string {
  const job = names.get(m.job) ?? jobName(m.job);
  const where = mapLabel(m.map);
  return where === ''
    ? `Lv ${String(m.baseLevel)} ${job}`
    : `Lv ${String(m.baseLevel)} ${job}, ${where}`;
}

/** The member count, such as "(3 / 12)". */
export function memberCount(state: PartyState, max: number): string {
  return `(${String(state.members.length)} / ${String(max)})`;
}

/** What the party window asks for. */
export interface PartyActions {
  create: (name: string, itemEven: boolean, pickupShared: boolean) => void;
  invite: (name: string) => void;
  leave: () => void;
  expel: (name: string) => void;
  setLeader: (name: string) => void;
  options: (options: PartyOptionValues) => void;
}

/** The options as the window's toggles name them. */
const OPTION_LABELS: [keyof PartyOptionValues, string, string][] = [
  ['expEven', 'Even exp share', 'Each member gets its own exp when off.'],
  ['itemEven', 'Even item share', 'Each member keeps the items it picks up when off.'],
  ['pickupShared', 'Shared pick-up', 'Members can pick up items the party may take.'],
];

/** A checkbox with its label. */
function toggle(text: string, title: string): { label: HTMLLabelElement; box: HTMLInputElement } {
  const label = document.createElement('label');
  const box = document.createElement('input');
  box.type = 'checkbox';
  label.title = title;
  label.append(box, ` ${text}`);
  return { label, box };
}

/** Keeps keys typed in a field from acting as game shortcuts. */
function textField(input: HTMLInputElement, label: string, placeholder: string): void {
  input.autocomplete = 'off';
  input.spellcheck = false;
  input.maxLength = MAX_NAME;
  input.placeholder = placeholder;
  input.setAttribute('aria-label', label);
  input.addEventListener('keydown', (e) => {
    e.stopPropagation();
    if (e.key === 'Escape') {
      input.blur();
    }
  });
}

/**
 * The window on the page, section#party-window. It holds h2 with the
 * party's name and span.party-count, ul.party-members with one
 * li[data-member] per member, and the forms. Each member row has
 * span.online-dot, span.party-leader on the leader, span.party-detail,
 * and div.party-hp[data-hp][data-max-hp] with div.party-hp-fill inside
 * and span.party-hp-text after it.
 */
export class PartyWindow {
  private readonly title = document.createElement('span');
  private readonly count = document.createElement('span');
  private readonly members = document.createElement('ul');
  private readonly createForm = document.createElement('form');
  private readonly createName = document.createElement('input');
  private readonly createItem = toggle('Even item share', OPTION_LABELS[1]![2]);
  private readonly createPickup = toggle('Shared pick-up', OPTION_LABELS[2]![2]);
  private readonly inviteForm = document.createElement('form');
  private readonly inviteName = document.createElement('input');
  private readonly optionsBox = document.createElement('fieldset');
  private readonly optionBoxes = new Map<keyof PartyOptionValues, HTMLInputElement>();
  private readonly leaveButton = document.createElement('button');
  private readonly error = document.createElement('p');
  private state: PartyState = NO_PARTY;

  /** The player's own name, to know whether the player leads. */
  selfName: (() => string | undefined) | undefined;
  /** Job names from the server, which win over the client's. */
  jobNames: ReadonlyMap<number, string> = new Map();

  constructor(
    private readonly root: HTMLElement,
    private readonly actions: PartyActions,
    private max: number,
  ) {
    const heading = document.createElement('h2');
    this.count.className = 'party-count';
    this.title.className = 'party-name';
    heading.append(this.title, ' ', this.count);
    this.members.className = 'party-members';

    this.createForm.className = 'party-create';
    this.createName.className = 'party-create-name';
    textField(this.createName, 'Party name', 'Party name');
    const make = document.createElement('button');
    make.type = 'submit';
    make.textContent = 'Create';
    this.createItem.box.className = 'party-create-item';
    this.createPickup.box.className = 'party-create-pickup';
    const createRow = document.createElement('div');
    createRow.className = 'party-row';
    createRow.append(this.createName, make);
    this.createForm.append(createRow, this.createItem.label, this.createPickup.label);
    this.createForm.addEventListener('submit', (e) => {
      e.preventDefault();
      const name = cleanName(this.createName.value);
      if (name === undefined) {
        this.showError(`Type a party name of 1 to ${String(MAX_NAME)} characters.`);
        return;
      }
      this.showError(undefined);
      this.actions.create(name, this.createItem.box.checked, this.createPickup.box.checked);
      this.createName.value = '';
    });

    this.inviteForm.className = 'party-invite';
    this.inviteName.className = 'party-invite-name';
    textField(this.inviteName, 'Name to invite', 'Character name');
    const invite = document.createElement('button');
    invite.type = 'submit';
    invite.textContent = 'Invite';
    this.inviteForm.append(this.inviteName, invite);
    this.inviteForm.addEventListener('submit', (e) => {
      e.preventDefault();
      const name = cleanName(this.inviteName.value);
      if (name === undefined) {
        this.showError(`Type a name of 1 to ${String(MAX_NAME)} characters.`);
        return;
      }
      if (this.state.members.length >= this.max) {
        this.showError('The party is full.');
        return;
      }
      this.showError(undefined);
      this.actions.invite(name);
      this.inviteName.value = '';
    });

    this.optionsBox.className = 'party-options';
    const legend = document.createElement('legend');
    legend.textContent = 'Options';
    this.optionsBox.append(legend);
    for (const [key, text, title] of OPTION_LABELS) {
      const t = toggle(text, title);
      t.box.className = 'party-option';
      t.box.dataset.option = key;
      t.box.addEventListener('change', () => {
        // The window shows the server's options again when PartyInfo comes.
        this.actions.options({ ...this.state.options, [key]: t.box.checked });
      });
      this.optionBoxes.set(key, t.box);
      this.optionsBox.append(t.label);
    }

    this.leaveButton.type = 'button';
    this.leaveButton.className = 'party-leave';
    this.leaveButton.textContent = 'Leave party';
    this.leaveButton.addEventListener('click', () => {
      this.actions.leave();
    });

    this.error.className = 'party-error';
    this.error.hidden = true;
    this.root.replaceChildren(
      heading,
      this.members,
      this.createForm,
      this.inviteForm,
      this.optionsBox,
      this.leaveButton,
      this.error,
    );
    this.render();
  }

  get open(): boolean {
    return !this.root.hidden;
  }

  /** The party the window shows. */
  get party(): PartyState {
    return this.state;
  }

  /** Changes the most members a party holds. */
  setMax(max: number): void {
    this.max = max;
    this.render();
  }

  /** Shows a party. */
  show(state: PartyState): void {
    this.state = state;
    this.render();
  }

  /** Opens or closes the window. */
  toggle(): void {
    this.root.hidden = !this.root.hidden;
    if (this.open) {
      this.render();
    }
  }

  /** Draws the window again, such as after the player's name is known. */
  render(): void {
    const s = this.state;
    const inParty = s.id !== 0n;
    const leader = leads(s, this.selfName?.());
    this.root.dataset.party = inParty ? 'yes' : 'no';
    this.root.dataset.leader = leader ? 'yes' : 'no';
    this.title.textContent = inParty ? s.name : 'Party';
    this.count.textContent = inParty ? memberCount(s, this.max) : '';
    this.members.hidden = !inParty;
    this.createForm.hidden = inParty;
    this.inviteForm.hidden = !leader;
    this.optionsBox.hidden = !inParty;
    this.optionsBox.disabled = !leader;
    this.optionsBox.title = leader ? '' : 'Only the party leader can change the options.';
    for (const [key, box] of this.optionBoxes) {
      box.checked = s.options[key];
    }
    this.leaveButton.hidden = !inParty;
    this.members.replaceChildren(...s.members.map((m) => this.row(m, leader)));
  }

  private row(m: PartyMemberView, leader: boolean): HTMLLIElement {
    const row = document.createElement('li');
    row.dataset.member = m.name;
    row.classList.toggle('offline', !m.online);
    const head = document.createElement('div');
    head.className = 'party-member-head';
    const dot = document.createElement('span');
    dot.className = `online-dot${m.online ? ' online' : ''}`;
    dot.title = m.online ? 'Online' : 'Offline';
    const name = document.createElement('bdi');
    name.className = 'party-member-name';
    name.textContent = m.name;
    head.append(dot, name);
    if (m.leader) {
      const mark = document.createElement('span');
      mark.className = 'party-leader';
      mark.textContent = '★';
      mark.title = 'Leader';
      mark.setAttribute('aria-label', 'Leader');
      head.append(' ', mark);
    }
    const detail = document.createElement('span');
    detail.className = 'party-detail';
    detail.textContent = memberDetail(m, this.jobNames);
    head.append(' ', detail);
    if (leader && m.name !== this.selfName?.()) {
      const promote = document.createElement('button');
      promote.type = 'button';
      promote.className = 'party-set-leader';
      promote.textContent = 'Lead';
      promote.title = `Make ${m.name} the leader`;
      promote.addEventListener('click', () => {
        this.actions.setLeader(m.name);
      });
      const expel = document.createElement('button');
      expel.type = 'button';
      expel.className = 'party-expel';
      expel.textContent = 'Expel';
      expel.title = `Expel ${m.name}`;
      expel.addEventListener('click', () => {
        this.actions.expel(m.name);
      });
      head.append(' ', promote, ' ', expel);
    }
    const bar = document.createElement('div');
    bar.className = 'party-hp';
    bar.dataset.hp = String(m.hp);
    bar.dataset.maxHp = String(m.maxHp);
    bar.setAttribute('role', 'meter');
    bar.setAttribute('aria-label', `HP of ${m.name}`);
    bar.setAttribute('aria-valuemin', '0');
    bar.setAttribute('aria-valuemax', String(m.maxHp));
    bar.setAttribute('aria-valuenow', String(Math.max(0, m.hp)));
    const fill = document.createElement('div');
    fill.className = 'party-hp-fill';
    const percent = hpPercent(m.hp, m.maxHp);
    fill.style.width = `${String(percent)}%`;
    fill.classList.toggle('low', percent < 25);
    bar.append(fill);
    const text = document.createElement('span');
    text.className = 'party-hp-text';
    text.textContent = m.maxHp > 0 ? `${String(Math.max(0, m.hp))} / ${String(m.maxHp)}` : '';
    row.append(head, bar, text);
    return row;
  }

  private showError(text: string | undefined): void {
    this.error.textContent = text ?? '';
    this.error.hidden = text === undefined;
  }
}

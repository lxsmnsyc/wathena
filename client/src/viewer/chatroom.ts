// Chat rooms (proposal 0006, section 2; decision 0080). A room is an NPC's
// waiting room or a player's chat room. The server sends ChatRoomShow for
// each room near the player, and ChatRoomJoined, ChatRoomMember and the
// room's chat lines to its members. The window lists the rooms near the
// player with a join button, makes a room, and for a member shows the
// room: its members with their owner's badge, the room's own chat, and
// for the owner the title, limit and password, the hand over and the
// kick. The client has no room bubble over heads yet, so the list stands
// in for it. Titles, names and lines are only ever set with textContent.

import { ChatRoomChange, ChatRoomOutcome } from '../gen/wathena/v1/event_pb';
import {
  button,
  bytesProblem,
  element,
  field,
  keepKeys,
  numberInput,
  parseWhole,
  textInput,
} from './forms';

/** The limits of rAthena's chat rooms. */
export const MAX_TITLE_CHARS = 36;
export const MAX_PASSWORD_CHARS = 8;
export const MAX_USERS = 20;

/** A room near the player, from ChatRoomShow. */
export interface RoomInfo {
  room: number;
  owner: number;
  title: string;
  users: number;
  limit: number;
  public: boolean;
  npc: boolean;
  minLevel: number;
  maxLevel: number;
  zeny: number;
}

/** The room the player is in, from ChatRoomJoined and ChatRoomMember. */
export interface RoomView {
  room: number;
  members: string[];
  owner: string;
  title: string;
  limit: number;
  public: boolean;
}

/** A ChatRoomMember event. */
export interface RoomMember {
  room: number;
  name: string;
  change: ChatRoomChange;
  title: string;
  limit: number;
  public: boolean;
}

/** What the window sends. */
export interface RoomActions {
  create(title: string, password: string, limit: number, isPublic: boolean): void;
  join(room: number, password: string): void;
  leave(): void;
  change(title: string, password: string, limit: number, isPublic: boolean): void;
  owner(name: string): void;
  kick(name: string): void;
  say(text: string): void;
}

/** The text of a refused join or create. */
export function outcomeText(outcome: ChatRoomOutcome): string | undefined {
  switch (outcome) {
    case ChatRoomOutcome.FULL:
      return 'The room is full, or you cannot join it now.';
    case ChatRoomOutcome.WRONG_PASSWORD:
      return 'Wrong password.';
    case ChatRoomOutcome.KICKED:
      return 'You were kicked from this room.';
    case ChatRoomOutcome.NO_ZENY:
      return 'You do not have the zeny the room asks for.';
    case ChatRoomOutcome.LEVEL_LOW:
      return 'Your level is too low for this room.';
    case ChatRoomOutcome.LEVEL_HIGH:
      return 'Your level is too high for this room.';
    case ChatRoomOutcome.NOT_ALLOWED:
      return 'You cannot make a room here.';
    default:
      return undefined;
  }
}

/** The line a room shows for a room, as the bubble would: title, lock and users. */
export function roomLabel(r: RoomInfo): string {
  const lock = r.public ? '' : ' (locked)';
  return `${r.title}${lock} ${String(r.users)}/${String(r.limit)}`;
}

/** The conditions of an NPC room: its levels and its fee. */
export function roomConditions(r: RoomInfo): string {
  const parts: string[] = [];
  if (r.minLevel > 1 || r.maxLevel < 275) {
    parts.push(`level ${String(r.minLevel)} to ${String(r.maxLevel)}`);
  }
  if (r.zeny > 0) {
    parts.push(`${String(r.zeny)} z`);
  }
  return parts.join(', ');
}

/** Whether joining asks for a password: a private room of a player. */
export function needsPassword(r: RoomInfo): boolean {
  return !r.public && !r.npc;
}

/** The notice a change of the room's members gives, or undefined for none. */
export function memberNotice(m: RoomMember, self: string): string | undefined {
  const who = m.name === self ? 'You' : m.name;
  switch (m.change) {
    case ChatRoomChange.JOIN:
      return `${who} ${who === 'You' ? 'joined' : 'joins'} the room.`;
    case ChatRoomChange.LEAVE:
      return `${who} ${who === 'You' ? 'left' : 'leaves'} the room.`;
    case ChatRoomChange.KICKED:
      return `${who} ${who === 'You' ? 'were' : 'was'} kicked from the room.`;
    case ChatRoomChange.OWNER:
      return `${who} ${who === 'You' ? 'are' : 'is'} the owner of the room now.`;
    case ChatRoomChange.STATUS:
      return `The room is now "${m.title}", for ${String(m.limit)}.`;
    default:
      return undefined;
  }
}

/**
 * What the client knows of rooms: the rooms near the player, and the room
 * the player is in. It is plain data, so it needs no window.
 */
export class RoomBook {
  readonly near = new Map<number, RoomInfo>();
  current: RoomView | undefined;

  constructor(private self = '') {}

  /** Sets the player's name, which tells the player's own events apart. */
  setSelf(name: string): void {
    this.self = name;
  }

  /** A ChatRoomShow: a new room, or the new state of a known one. */
  show(r: RoomInfo): void {
    this.near.set(r.room, r);
  }

  /** A ChatRoomClear. */
  clear(room: number): void {
    this.near.delete(room);
  }

  /** The nearby rooms, newest first by ID. */
  list(): RoomInfo[] {
    return [...this.near.values()].sort((a, b) => b.room - a.room);
  }

  /** A ChatRoomJoined: the player's own view of its room. */
  joined(v: RoomView): void {
    this.current = { ...v, members: [...v.members] };
  }

  /**
   * A ChatRoomMember. It changes the room the player is in, and returns
   * the notice to show. The player's own leave or kick ends the room.
   */
  member(m: RoomMember): string | undefined {
    const c = this.current;
    if (c?.room !== m.room) {
      return undefined;
    }
    switch (m.change) {
      case ChatRoomChange.JOIN:
        if (!c.members.includes(m.name)) {
          c.members.push(m.name);
        }
        break;
      case ChatRoomChange.LEAVE:
      case ChatRoomChange.KICKED: {
        c.members = c.members.filter((n) => n !== m.name);
        break;
      }
      case ChatRoomChange.OWNER: {
        // The new owner takes the first place, as the server swaps them.
        const i = c.members.indexOf(m.name);
        const first = c.members[0];
        if (i > 0 && first !== undefined) {
          c.members[0] = m.name;
          c.members[i] = first;
        }
        c.owner = m.name;
        break;
      }
      case ChatRoomChange.STATUS:
        c.title = m.title;
        c.limit = m.limit;
        c.public = m.public;
        break;
      default:
        break;
    }
    const notice = memberNotice(m, this.self);
    if (
      m.name === this.self &&
      (m.change === ChatRoomChange.LEAVE || m.change === ChatRoomChange.KICKED)
    ) {
      this.current = undefined;
    }
    return notice;
  }

  /** Whether the player owns the room it is in. */
  get owns(): boolean {
    return (
      this.current !== undefined && this.current.owner === this.self && this.current.owner !== ''
    );
  }
}

/** Why a title, password and limit do not make a room, or undefined. */
export function roomProblem(title: string, password: string, limit: string): string | undefined {
  const t = title.trim();
  if (Array.from(t).length < 1 || Array.from(t).length > MAX_TITLE_CHARS) {
    return `The title is 1 to ${String(MAX_TITLE_CHARS)} characters.`;
  }
  if (Array.from(password).length > MAX_PASSWORD_CHARS) {
    return `The password is at most ${String(MAX_PASSWORD_CHARS)} characters.`;
  }
  if (parseWhole(limit, 1, MAX_USERS) === undefined) {
    return `The most users is 1 to ${String(MAX_USERS)}.`;
  }
  return bytesProblem('The title', t, 1, MAX_TITLE_CHARS * 4);
}

/** The window of chat rooms. */
export class ChatRoomWindow {
  readonly book: RoomBook;
  private readonly nearby = element('ul', 'room-near');
  private readonly view = element('div', 'room-view');
  private readonly notice = element('p', 'room-notice');
  private readonly log = element('ul', 'room-log');
  private readonly members = element('ul', 'room-members');
  private readonly head = element('h3', 'room-title');
  private readonly say = textInput('Say to the room', 24);
  private readonly title = textInput('Title', 20);
  private readonly password = textInput('Password', 8);
  private readonly limit = numberInput('10', 3);
  private readonly isPublic = document.createElement('input');
  private readonly form = element('div', 'room-form');
  private readonly submit: HTMLButtonElement;
  private readonly owner = element('div', 'room-owner');

  constructor(
    private readonly root: HTMLElement,
    private readonly actions: RoomActions,
    self = '',
  ) {
    this.book = new RoomBook(self);
    this.isPublic.type = 'checkbox';
    this.isPublic.checked = true;
    this.password.maxLength = MAX_PASSWORD_CHARS;
    this.title.maxLength = MAX_TITLE_CHARS;
    this.submit = button('Make a room', () => {
      this.submitForm();
    });
    this.form.append(
      field('Title', this.title),
      field('Most users', this.limit),
      field('Password', this.password),
      field('Public', this.isPublic),
      this.submit,
    );
    const sayRow = element('div', 'room-say');
    sayRow.append(
      this.say,
      button('Say', () => {
        this.sendLine();
      }),
    );
    this.say.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') {
        this.sendLine();
      }
    });
    this.owner.append(
      button('Hand over', () => {
        const name = this.selected();
        if (name !== undefined) {
          this.actions.owner(name);
        }
      }),
      button('Kick', () => {
        const name = this.selected();
        if (name !== undefined) {
          this.actions.kick(name);
        }
      }),
    );
    this.view.append(
      this.head,
      this.members,
      this.log,
      sayRow,
      this.owner,
      button('Leave', () => {
        this.actions.leave();
      }),
    );
    root.append(element('h2', '', 'Chat rooms'), this.notice, this.nearby, this.form, this.view);
    keepKeys(root, () => {
      this.hide();
    });
    root.hidden = true;
    this.render();
  }

  /** Whether the window shows. */
  get open(): boolean {
    return !this.root.hidden;
  }

  show(): void {
    this.root.hidden = false;
    this.render();
  }

  hide(): void {
    this.root.hidden = true;
  }

  toggle(): void {
    if (this.open) {
      this.hide();
    } else {
      this.show();
    }
  }

  /** A notice of the window, such as a member who joined. */
  note(text: string): void {
    this.notice.textContent = text;
  }

  /** A ChatRoomResult. */
  result(outcome: ChatRoomOutcome): void {
    const text = outcomeText(outcome);
    this.notice.textContent = text ?? '';
    this.render();
  }

  /** A line of the room's chat. */
  line(name: string, text: string): void {
    const li = element('li');
    li.textContent = `${name}: ${text}`;
    this.log.append(li);
    while (this.log.children.length > 100) {
      this.log.firstElementChild?.remove();
    }
    this.log.scrollTop = this.log.scrollHeight;
  }

  /** Draws the lists again after the book changed. */
  render(): void {
    const cur = this.book.current;
    this.view.hidden = cur === undefined;
    // The form makes a room, and for the owner changes the room it owns.
    this.form.hidden = cur !== undefined && !this.book.owns;
    this.submit.textContent = cur === undefined ? 'Make a room' : 'Change the room';
    if (cur !== undefined && this.form.dataset.room !== String(cur.room)) {
      this.form.dataset.room = String(cur.room);
      this.title.value = cur.title;
      this.limit.value = String(cur.limit);
      this.password.value = '';
      this.isPublic.checked = cur.public;
    }
    this.nearby.hidden = cur !== undefined;
    this.nearby.replaceChildren();
    if (cur === undefined) {
      this.log.replaceChildren();
      for (const r of this.book.list()) {
        this.nearby.append(this.roomRow(r));
      }
      if (this.book.near.size === 0) {
        this.nearby.append(element('li', 'room-none', 'No rooms near you.'));
      }
      return;
    }
    this.head.textContent = `${cur.title} (${String(cur.members.length)}/${String(cur.limit)})`;
    this.members.replaceChildren();
    for (const name of cur.members) {
      const li = element('li');
      li.textContent = name === cur.owner ? `${name} (owner)` : name;
      li.dataset.name = name;
      li.tabIndex = 0;
      li.addEventListener('click', () => {
        for (const o of this.members.children) {
          o.classList.toggle('selected', o === li);
        }
      });
      this.members.append(li);
    }
    this.owner.hidden = !this.book.owns;
  }

  private selected(): string | undefined {
    const el = this.members.querySelector<HTMLElement>('li.selected');
    const name = el?.dataset.name;
    return name === undefined || name === this.book.current?.owner ? undefined : name;
  }

  private roomRow(r: RoomInfo): HTMLLIElement {
    const li = element('li', 'room-row');
    const label = element('span', 'room-label');
    const conditions = roomConditions(r);
    label.textContent = conditions === '' ? roomLabel(r) : `${roomLabel(r)}: ${conditions}`;
    const pass = textInput('Password', 8);
    pass.maxLength = MAX_PASSWORD_CHARS;
    pass.hidden = !needsPassword(r);
    li.append(
      label,
      pass,
      button('Join', () => {
        this.actions.join(r.room, pass.value);
      }),
    );
    return li;
  }

  private submitForm(): void {
    const problem = roomProblem(this.title.value, this.password.value, this.limit.value);
    if (problem !== undefined) {
      this.notice.textContent = problem;
      return;
    }
    const limit = parseWhole(this.limit.value, 1, MAX_USERS) ?? 1;
    this.notice.textContent = '';
    if (this.book.current !== undefined) {
      this.actions.change(
        this.title.value.trim(),
        this.password.value,
        limit,
        this.isPublic.checked,
      );
      return;
    }
    this.actions.create(this.title.value.trim(), this.password.value, limit, this.isPublic.checked);
  }

  private sendLine(): void {
    const text = this.say.value.trim();
    if (text === '') {
      return;
    }
    this.say.value = '';
    this.actions.say(text);
  }
}

// The friend list panel: the player's friends with an online dot each, a
// Remove button each, and a field to add a friend by name. User text is
// only ever set with textContent.

import { MAX_NAME, cleanName } from './chatCommands';

/** A friend as the panel shows it. */
export interface FriendView {
  name: string;
  online: boolean;
}

/** The friend list FriendList gives. */
export function friendsFromList(list: readonly FriendView[]): FriendView[] {
  return list.map((f) => ({ name: f.name, online: f.online }));
}

/**
 * The list after a FriendStatus. Returns undefined when nothing changed:
 * the name is not a friend, or its state is the same.
 */
export function setFriendOnline(
  list: readonly FriendView[],
  name: string,
  online: boolean,
): FriendView[] | undefined {
  const at = list.findIndex((f) => f.name === name);
  if (at < 0 || list[at]!.online === online) {
    return undefined;
  }
  const next = [...list];
  next[at] = { name, online };
  return next;
}

/** The online friends first, each group in list order. */
export function sortFriends(list: readonly FriendView[]): FriendView[] {
  return [...list.filter((f) => f.online), ...list.filter((f) => !f.online)];
}

/**
 * Checks a name typed in the add field. Returns the cleaned name, or the
 * reason it cannot be added.
 */
export function checkFriendName(
  text: string,
  list: readonly FriendView[],
  max: number,
  self?: string,
): { name: string } | { error: string } {
  const name = cleanName(text);
  if (name === undefined) {
    return { error: `Type a name of 1 to ${String(MAX_NAME)} characters.` };
  }
  if (name === self) {
    return { error: 'You cannot add yourself.' };
  }
  if (list.some((f) => f.name === name)) {
    return { error: `${name} is your friend already.` };
  }
  if (list.length >= max) {
    return { error: 'Your friend list is full. Remove a name first.' };
  }
  return { name };
}

/** What the panel asks for. */
export interface FriendActions {
  add: (name: string) => void;
  remove: (name: string) => void;
  whisper: (name: string) => void;
}

/**
 * The panel on the page, section#friend-panel. It holds a count,
 * ul.friend-names with one li[data-friend] per friend, each with
 * span.online-dot, a Whisper button and button.friend-remove, and a form
 * with input.friend-name and its Add button.
 */
export class FriendPanel {
  private readonly count = document.createElement('span');
  private readonly names = document.createElement('ul');
  private readonly input = document.createElement('input');
  private readonly error = document.createElement('p');
  private current: FriendView[] = [];

  /** The player's own name. */
  selfName: (() => string | undefined) | undefined;

  constructor(
    private readonly root: HTMLElement,
    private readonly actions: FriendActions,
    private readonly max: number,
  ) {
    const title = document.createElement('h2');
    title.append('Friends ', this.count);
    this.names.className = 'friend-names';
    const form = document.createElement('form');
    form.className = 'friend-form';
    this.input.className = 'friend-name';
    this.input.autocomplete = 'off';
    this.input.spellcheck = false;
    this.input.maxLength = MAX_NAME;
    this.input.placeholder = 'Character name';
    this.input.setAttribute('aria-label', 'Name to add as a friend');
    const add = document.createElement('button');
    add.type = 'submit';
    add.className = 'friend-add';
    add.textContent = 'Add';
    form.append(this.input, add);
    form.addEventListener('submit', (e) => {
      e.preventDefault();
      this.submit();
    });
    this.input.addEventListener('keydown', (e) => {
      // Keys typed here are text, not game shortcuts.
      e.stopPropagation();
      if (e.key === 'Escape') {
        this.input.blur();
      }
    });
    this.error.className = 'friend-error';
    this.error.hidden = true;
    this.root.replaceChildren(title, this.names, form, this.error);
    this.show([]);
  }

  /** The friends the panel shows. */
  get friends(): readonly FriendView[] {
    return this.current;
  }

  /** Shows a friend list. */
  show(list: readonly FriendView[]): void {
    this.current = [...list];
    const online = list.filter((f) => f.online).length;
    this.count.textContent = `(${String(list.length)} / ${String(this.max)}, ${String(online)} online)`;
    const rows = sortFriends(list).map((f) => {
      const row = document.createElement('li');
      row.dataset.friend = f.name;
      row.classList.toggle('offline', !f.online);
      const dot = document.createElement('span');
      dot.className = `online-dot${f.online ? ' online' : ''}`;
      dot.title = f.online ? 'Online' : 'Offline';
      const label = document.createElement('bdi');
      label.textContent = f.name;
      const who = document.createElement('span');
      who.className = 'friend-who';
      who.append(dot, label);
      const whisper = document.createElement('button');
      whisper.type = 'button';
      whisper.className = 'friend-whisper';
      whisper.textContent = 'Whisper';
      whisper.setAttribute('aria-label', `Whisper to ${f.name}`);
      whisper.addEventListener('click', () => {
        this.actions.whisper(f.name);
      });
      const remove = document.createElement('button');
      remove.type = 'button';
      remove.className = 'friend-remove';
      remove.textContent = 'Remove';
      remove.setAttribute('aria-label', `Remove ${f.name}`);
      remove.addEventListener('click', () => {
        this.actions.remove(f.name);
      });
      const buttons = document.createElement('span');
      buttons.append(whisper, ' ', remove);
      row.append(who, buttons);
      return row;
    });
    if (rows.length === 0) {
      const row = document.createElement('li');
      row.className = 'empty';
      row.textContent = 'You have no friends on the list yet.';
      rows.push(row);
    }
    this.names.replaceChildren(...rows);
  }

  /** Opens or closes the panel. */
  toggle(): void {
    this.root.hidden = !this.root.hidden;
  }

  private submit(): void {
    const got = checkFriendName(this.input.value, this.current, this.max, this.selfName?.());
    const problem = 'error' in got ? got.error : undefined;
    this.error.textContent = problem ?? '';
    this.error.hidden = problem === undefined;
    if ('name' in got) {
      this.actions.add(got.name);
      this.input.value = '';
    }
  }
}

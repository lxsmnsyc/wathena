// The hotkey bar, as RO's F1 to F9 row: nine slots that each cast a
// skill. Dragging a skill from the skill window onto a slot assigns it,
// and a right click on a slot clears it. The slots are saved in
// localStorage for each character name.

/** The number of slots, F1 to F9. */
export const HOTKEY_SLOTS = 9;

/** The drag data type that carries a skill ID and level from the skill window. */
export const SKILL_DRAG_TYPE = 'application/x-wathena-skill';

/**
 * A skill in a slot. Without a level, it casts at the level the player
 * has learned.
 */
export interface Hotkey {
  skillId: number;
  level?: number;
}

export type Hotkeys = (Hotkey | undefined)[];

/** The localStorage key of a character's hotkeys. */
export function hotkeyStorageKey(name: string): string {
  return `wathena.hotkeys.${name}`;
}

/** The slot of a key, 0 to 8 for F1 to F9, from KeyboardEvent.code. */
export function hotkeySlot(code: string): number | undefined {
  const m = /^F([1-9])$/.exec(code);
  return m === null ? undefined : Number(m[1]) - 1;
}

/** Empty slots. */
export function emptyHotkeys(): Hotkeys {
  return Array.from({ length: HOTKEY_SLOTS }, () => undefined);
}

function positive(v: unknown): v is number {
  return typeof v === 'number' && Number.isInteger(v) && v > 0;
}

/** Reads saved slots. Anything malformed becomes an empty slot. */
export function parseHotkeys(text: string | null): Hotkeys {
  const out = emptyHotkeys();
  if (text === null) {
    return out;
  }
  let data: unknown;
  try {
    data = JSON.parse(text);
  } catch {
    return out;
  }
  if (!Array.isArray(data)) {
    return out;
  }
  for (let i = 0; i < HOTKEY_SLOTS; i++) {
    const v: unknown = data[i];
    if (typeof v !== 'object' || v === null || !('skillId' in v) || !positive(v.skillId)) {
      continue;
    }
    const level = 'level' in v && positive(v.level) ? v.level : undefined;
    out[i] = level === undefined ? { skillId: v.skillId } : { skillId: v.skillId, level };
  }
  return out;
}

/** Writes slots for parseHotkeys. */
export function serializeHotkeys(slots: Hotkeys): string {
  return JSON.stringify(slots.slice(0, HOTKEY_SLOTS).map((s) => s ?? null));
}

/** The part of Storage the hotkeys use. */
export type HotkeyStorage = Pick<Storage, 'getItem' | 'setItem'>;

/**
 * Loads a character's slots. storage returns localStorage. Reading it can
 * throw, such as when the browser blocks storage, and then the slots are
 * empty.
 */
export function loadHotkeys(storage: () => HotkeyStorage, name: string): Hotkeys {
  try {
    return parseHotkeys(storage().getItem(hotkeyStorageKey(name)));
  } catch {
    return emptyHotkeys();
  }
}

/** Saves a character's slots. Returns false when the browser refuses. */
export function saveHotkeys(storage: () => HotkeyStorage, name: string, slots: Hotkeys): boolean {
  try {
    storage().setItem(hotkeyStorageKey(name), serializeHotkeys(slots));
    return true;
  } catch {
    return false;
  }
}

/** Reads a dragged skill, written as "<skill ID>" or "<skill ID>:<level>". */
export function parseDraggedSkill(data: string): Hotkey | undefined {
  const m = /^(\d+)(?::(\d+))?$/.exec(data);
  if (m === null) {
    return undefined;
  }
  const skillId = Number(m[1]);
  const level = m[2] === undefined ? undefined : Number(m[2]);
  if (!positive(skillId)) {
    return undefined;
  }
  return level !== undefined && positive(level) ? { skillId, level } : { skillId };
}

/** Writes a skill for parseDraggedSkill. */
export function draggedSkill(h: Hotkey): string {
  return h.level === undefined ? String(h.skillId) : `${h.skillId}:${h.level}`;
}

/** How a slot shows: its badge and title, and its cooldown. */
export interface SlotView {
  badge: string;
  title: string;
  /** The cooldown's text, or '' when the skill is ready. */
  cooldown: string;
  /** Whether the slot's skill can be cast now. */
  usable: boolean;
}

interface SlotElements {
  li: HTMLLIElement;
  badge: HTMLSpanElement;
  cooldown: HTMLSpanElement;
}

/** The hotkey bar on the page. */
export class HotkeyBar {
  readonly root: HTMLOListElement;
  private slots: Hotkeys = emptyHotkeys();
  private readonly elements: SlotElements[] = [];

  constructor(
    parent: HTMLElement,
    private readonly actions: {
      /** Casts the skill in a slot. */
      use: (slot: number) => void;
      /** Called after the slots change, to save them. */
      changed: (slots: Hotkeys) => void;
    },
  ) {
    this.root = document.createElement('ol');
    this.root.id = 'hotkeys';
    this.root.setAttribute('aria-label', 'Hotkeys');
    this.root.hidden = true;
    for (let i = 0; i < HOTKEY_SLOTS; i++) {
      const li = document.createElement('li');
      li.className = 'hotkey empty';
      const key = document.createElement('span');
      key.className = 'key';
      key.textContent = `F${i + 1}`;
      const badge = document.createElement('span');
      badge.className = 'badge';
      const cooldown = document.createElement('span');
      cooldown.className = 'cooldown';
      li.append(badge, key, cooldown);
      li.addEventListener('click', () => {
        actions.use(i);
      });
      li.addEventListener('contextmenu', (e) => {
        e.preventDefault();
        this.assign(i, undefined);
      });
      li.addEventListener('dragover', (e) => {
        if (e.dataTransfer?.types.includes(SKILL_DRAG_TYPE) === true) {
          e.preventDefault();
          e.dataTransfer.dropEffect = 'copy';
        }
      });
      li.addEventListener('drop', (e) => {
        const h = parseDraggedSkill(e.dataTransfer?.getData(SKILL_DRAG_TYPE) ?? '');
        if (h !== undefined) {
          e.preventDefault();
          this.assign(i, h);
        }
      });
      this.root.append(li);
      this.elements.push({ li, badge, cooldown });
    }
    parent.append(this.root);
  }

  /** Casts the skill in a slot. */
  use(slot: number): void {
    this.actions.use(slot);
  }

  /** The skill in a slot. */
  get(slot: number): Hotkey | undefined {
    return this.slots[slot];
  }

  /** Replaces every slot, such as with a character's saved slots. */
  load(slots: Hotkeys): void {
    this.slots = [...slots];
  }

  /** Puts a skill in a slot, or clears it. */
  assign(slot: number, hotkey: Hotkey | undefined): void {
    if (slot < 0 || slot >= HOTKEY_SLOTS) {
      return;
    }
    this.slots[slot] = hotkey;
    this.actions.changed([...this.slots]);
  }

  /** The first empty slot, if any. */
  firstEmpty(): number | undefined {
    const i = this.slots.findIndex((s) => s === undefined);
    return i < 0 ? undefined : i;
  }

  /** Shows each slot. view describes the skill in a slot. */
  update(view: (h: Hotkey) => SlotView): void {
    this.elements.forEach((el, i) => {
      const h = this.slots[i];
      const v = h === undefined ? undefined : view(h);
      setText(el.badge, v?.badge ?? '');
      setText(el.cooldown, v?.cooldown ?? '');
      const title = v === undefined ? `F${i + 1}: drag a skill here` : `F${i + 1}: ${v.title}`;
      if (el.li.title !== title) {
        el.li.title = title;
      }
      el.li.classList.toggle('empty', v === undefined);
      el.li.classList.toggle('waiting', v !== undefined && !v.usable);
    });
  }
}

function setText(el: HTMLElement, text: string): void {
  if (el.textContent !== text) {
    el.textContent = text;
  }
}

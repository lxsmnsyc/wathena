// The tabs above the chat log: All, Party while the player has a party,
// one tab per chat channel the player is in, and one tab per whisper
// partner. The chat input sends to the
// active tab. User text is only ever set with textContent.

/** The key of the All tab. */
export const ALL_TAB = 'all';

/** The key of the Party tab, and of party lines. */
export const PARTY_TAB = 'party';

/** The key of local chat lines, which only the All tab shows. */
export const LOCAL_LINES = 'local';

/** The most whisper tabs open at once. The oldest closes first. */
export const MAX_WHISPER_TABS = 10;

/** Where the chat input sends a line. */
export type ChatTarget =
  | { kind: 'local' }
  | { kind: 'party' }
  | { kind: 'channel'; name: string }
  | { kind: 'whisper'; name: string };

export interface ChatTab {
  /** ALL_TAB, PARTY_TAB, channel:#name in lower case, or whisper:Name. */
  key: string;
  kind: 'all' | 'party' | 'channel' | 'whisper';
  /** The channel's name with its #, or the whisper partner's name. */
  name: string;
  /** What the tab shows. */
  label: string;
  /** What a channel shows before each line, such as [Trade]. */
  alias: string;
  /** A channel's color as 0xrrggbb, or 0. */
  color: number;
  /** Whether the tab has a close button. */
  closable: boolean;
  /** Whether lines came since the tab was last shown. */
  unread: boolean;
}

/** The tab key of a chat channel. Channel names do not depend on case. */
export function channelKey(name: string): string {
  return `channel:${name.toLowerCase()}`;
}

/** The tab key of a whisper partner. */
export function whisperKey(name: string): string {
  return `whisper:${name}`;
}

/**
 * Whether the active tab shows a log line. lineTab is the line's tab
 * key, LOCAL_LINES for local chat, or empty for lines every tab shows,
 * such as notices.
 */
export function showsLine(active: string, lineTab: string): boolean {
  return active === ALL_TAB || lineTab === '' || lineTab === active;
}

/** The chat input's placeholder for a target. */
export function placeholder(target: ChatTarget): string {
  switch (target.kind) {
    case 'channel':
      return `Say in ${target.name}`;
    case 'whisper':
      return `Whisper to ${target.name}`;
    case 'party':
      return 'Say to your party';
    default:
      return 'Press Enter to chat';
  }
}

/** The tabs and which one is active. */
export class ChatTabs {
  private list: ChatTab[] = [
    {
      key: ALL_TAB,
      kind: 'all',
      name: '',
      label: 'All',
      alias: '',
      color: 0,
      closable: false,
      unread: false,
    },
  ];

  /** The active tab's key. */
  active = ALL_TAB;

  /** The tabs in order: All, Party, the channels, then the whispers. */
  get tabs(): readonly ChatTab[] {
    return this.list;
  }

  get(key: string): ChatTab | undefined {
    return this.list.find((t) => t.key === key);
  }

  /** The tab of a chat channel, if the player is in it. */
  channel(name: string): ChatTab | undefined {
    return this.get(channelKey(name));
  }

  /**
   * Adds the tab of a channel the player is in, or updates it. Returns
   * its key. alias is what each line shows first, such as [Trade].
   */
  joinChannel(name: string, alias: string, color: number, canLeave: boolean): string {
    const key = channelKey(name);
    const tab = this.get(key);
    if (tab !== undefined) {
      Object.assign(tab, { name, alias, color, closable: canLeave });
      return key;
    }
    // Channels come before whispers.
    let at = this.list.findIndex((t) => t.kind === 'whisper');
    if (at < 0) {
      at = this.list.length;
    }
    this.list.splice(at, 0, {
      key,
      kind: 'channel',
      name,
      label: name,
      alias: alias === '' ? `[${name}]` : alias,
      color,
      closable: canLeave,
      unread: false,
    });
    return key;
  }

  /** Removes a channel's tab. All becomes active if it was. */
  leaveChannel(name: string): void {
    this.close(channelKey(name));
  }

  /** Whether the Party tab shows. */
  get hasParty(): boolean {
    return this.get(PARTY_TAB) !== undefined;
  }

  /** Shows the Party tab, right after All, while the player has a party. */
  showParty(): void {
    if (this.hasParty) {
      return;
    }
    this.list.splice(1, 0, {
      key: PARTY_TAB,
      kind: 'party',
      name: '',
      label: 'Party',
      alias: '',
      color: 0,
      closable: false,
      unread: false,
    });
  }

  /** Removes the Party tab, when the player has no party. All becomes active if it was. */
  hideParty(): void {
    this.close(PARTY_TAB);
  }

  /**
   * Opens the whisper tab of a character, or finds it. Returns its key.
   * Past MAX_WHISPER_TABS the oldest whisper tab that is not active closes.
   */
  openWhisper(name: string): string {
    const key = whisperKey(name);
    if (this.get(key) !== undefined) {
      return key;
    }
    this.list.push({
      key,
      kind: 'whisper',
      name,
      label: name,
      alias: '',
      color: 0,
      closable: true,
      unread: false,
    });
    const whispers = this.list.filter((t) => t.kind === 'whisper');
    if (whispers.length > MAX_WHISPER_TABS) {
      const oldest = whispers.find((t) => t.key !== this.active && t.key !== key);
      if (oldest !== undefined) {
        this.close(oldest.key);
      }
    }
    return key;
  }

  /** Removes a tab. All becomes active if it was. All itself stays. */
  close(key: string): void {
    if (key === ALL_TAB) {
      return;
    }
    this.list = this.list.filter((t) => t.key !== key);
    if (this.active === key) {
      this.active = ALL_TAB;
    }
  }

  /** Makes a tab active and clears its unread mark. Returns false for an unknown key. */
  activate(key: string): boolean {
    const tab = this.get(key);
    if (tab === undefined) {
      return false;
    }
    this.active = key;
    tab.unread = false;
    return true;
  }

  /** Marks a tab unread, unless it is active. Returns whether the mark changed. */
  markUnread(key: string): boolean {
    const tab = this.get(key);
    if (tab === undefined || key === this.active || tab.unread || tab.kind === 'all') {
      return false;
    }
    tab.unread = true;
    return true;
  }

  /** Where the chat input sends a line on the active tab. */
  target(): ChatTarget {
    const tab = this.get(this.active);
    if (tab?.kind === 'channel') {
      return { kind: 'channel', name: tab.name };
    }
    if (tab?.kind === 'whisper') {
      return { kind: 'whisper', name: tab.name };
    }
    if (tab?.kind === 'party') {
      return { kind: 'party' };
    }
    return { kind: 'local' };
  }
}

/**
 * The tab bar on the page. Each tab is a button.chat-tab with its key in
 * data-tab. A closable tab has a button.chat-tab-close after it.
 */
export class ChatTabBar {
  /** Called when the player clicks a tab. */
  onSelect: ((key: string) => void) | undefined;
  /** Called when the player closes a whisper tab, or leaves a channel. */
  onClose: ((tab: ChatTab) => void) | undefined;

  constructor(
    private readonly root: HTMLElement,
    private readonly tabs: ChatTabs,
  ) {}

  render(): void {
    const items: HTMLElement[] = [];
    for (const tab of this.tabs.tabs) {
      const item = document.createElement('span');
      item.className = 'chat-tab-item';
      const button = document.createElement('button');
      button.type = 'button';
      button.className = 'chat-tab';
      button.setAttribute('role', 'tab');
      button.dataset.tab = tab.key;
      const active = tab.key === this.tabs.active;
      button.setAttribute('aria-selected', String(active));
      button.classList.toggle('active', active);
      button.classList.toggle('unread', tab.unread);
      button.textContent = tab.label;
      button.addEventListener('click', () => {
        this.onSelect?.(tab.key);
      });
      item.append(button);
      if (tab.closable) {
        const close = document.createElement('button');
        close.type = 'button';
        close.className = 'chat-tab-close';
        close.dataset.tab = tab.key;
        close.textContent = '×';
        const what = tab.kind === 'channel' ? 'Leave' : 'Close';
        close.title = `${what} ${tab.label}`;
        close.setAttribute('aria-label', `${what} ${tab.label}`);
        close.addEventListener('click', () => {
          this.onClose?.(tab);
        });
        item.append(close);
      }
      items.push(item);
    }
    this.root.replaceChildren(...items);
  }
}

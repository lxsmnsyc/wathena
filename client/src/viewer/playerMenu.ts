// The menu a right click on another player opens: whisper, trade, invite
// to the party or the guild, browse the player's vending shop or buying
// store, and report the player's guild name or emblem. User text is only
// ever set with textContent.

import { reportButton } from './chat';
import { badgeLine } from './guild';

/** A player's guild, as GuildBadge. */
export interface BadgeView {
  guildId: bigint;
  name: string;
  emblemVersion: number;
  position: string;
}

/** What the menu knows about a player and about what the player may do. */
export interface PlayerMenuInfo {
  /** The player's entity ID, for a trade request. */
  id: number;
  name: string;
  badge: BadgeView | undefined;
  /** Whether the player leads a party. */
  partyInvite: boolean;
  /** Whether the player's guild position allows invites. */
  guildInvite: boolean;
  /** The guild of the player who opens the menu, or 0n. */
  ownGuild: bigint;
  /** The player's open shop: a vending shop, a buying store, or none. */
  shop?: 'vending' | 'buying' | undefined;
}

export type PlayerMenuItem =
  | 'whisper'
  | 'trade'
  | 'partyInvite'
  | 'guildInvite'
  | 'browseShop'
  | 'browseStore'
  | 'reportGuildName'
  | 'reportEmblem';

/**
 * The menu's items: whisper and trade always, the party invite for a party leader,
 * the guild invite for a player who may invite someone without a guild,
 * Browse shop or Browse store for a player with an open shop, and reports
 * for another guild's name and its emblem.
 */
export function playerMenuItems(info: PlayerMenuInfo): PlayerMenuItem[] {
  const out: PlayerMenuItem[] = ['whisper', 'trade'];
  if (info.shop === 'vending') {
    out.push('browseShop');
  } else if (info.shop === 'buying') {
    out.push('browseStore');
  }
  if (info.partyInvite) {
    out.push('partyInvite');
  }
  if (info.guildInvite && info.badge === undefined) {
    out.push('guildInvite');
  }
  if (info.badge !== undefined && info.badge.guildId !== info.ownGuild) {
    out.push('reportGuildName');
    if (info.badge.emblemVersion > 0) {
      out.push('reportEmblem');
    }
  }
  return out;
}

export interface PlayerMenuActions {
  whisper: (name: string) => void;
  /** Asks the player with an entity ID to trade. */
  trade: (id: number) => void;
  partyInvite: (name: string) => void;
  guildInvite: (name: string) => void;
  /** Reports a guild's name, or its emblem when emblem is set. */
  reportGuild: (guildId: bigint, emblem: boolean) => void;
  /** Asks for the vending shop of the player with an entity ID. */
  browseShop: (id: number) => void;
  /** Asks for the buying store of the player with an entity ID. */
  browseStore: (id: number) => void;
}

const LABELS: Record<PlayerMenuItem, string> = {
  whisper: 'Whisper',
  trade: 'Trade',
  partyInvite: 'Invite to party',
  guildInvite: 'Invite to guild',
  browseShop: 'Browse shop',
  browseStore: 'Browse store',
  reportGuildName: 'Report guild name',
  reportEmblem: 'Report emblem',
};

/**
 * The menu on the page, div#player-menu. It holds bdi.player-menu-name,
 * div.player-menu-guild, and one button[data-item] per item.
 */
export class PlayerMenu {
  constructor(
    private readonly root: HTMLElement,
    private readonly actions: PlayerMenuActions,
  ) {
    this.root.hidden = true;
    this.root.setAttribute('role', 'menu');
    this.root.addEventListener('keydown', (e) => {
      e.stopPropagation();
      if (e.key === 'Escape') {
        this.close();
      }
    });
    addEventListener('pointerdown', (e) => {
      if (!this.root.hidden && e.target instanceof Node && !this.root.contains(e.target)) {
        this.close();
      }
    });
  }

  get open(): boolean {
    return !this.root.hidden;
  }

  /** Opens the menu for a player at a point of the page. */
  show(info: PlayerMenuInfo, x: number, y: number): void {
    const name = document.createElement('bdi');
    name.className = 'player-menu-name';
    name.textContent = info.name;
    const parts: HTMLElement[] = [name];
    if (info.badge !== undefined) {
      const guild = document.createElement('div');
      guild.className = 'player-menu-guild';
      guild.textContent = badgeLine(info.badge);
      parts.push(guild);
    }
    for (const item of playerMenuItems(info)) {
      parts.push(this.item(item, info));
    }
    this.root.replaceChildren(...parts);
    this.root.style.left = `${String(Math.max(0, Math.min(x, innerWidth - 180)))}px`;
    this.root.style.top = `${String(Math.max(0, Math.min(y, innerHeight - 160)))}px`;
    this.root.hidden = false;
    this.root.querySelector('button')?.focus();
  }

  close(): void {
    this.root.hidden = true;
    this.root.replaceChildren();
  }

  private item(item: PlayerMenuItem, info: PlayerMenuInfo): HTMLButtonElement {
    const badge = info.badge;
    if ((item === 'reportGuildName' || item === 'reportEmblem') && badge !== undefined) {
      const emblem = item === 'reportEmblem';
      const b = reportButton(
        LABELS[item],
        `Report the guild ${emblem ? 'emblem' : 'name'} of ${badge.name} to the staff`,
        () => {
          this.actions.reportGuild(badge.guildId, emblem);
          this.close();
        },
      );
      b.dataset.item = item;
      b.setAttribute('role', 'menuitem');
      return b;
    }
    const b = document.createElement('button');
    b.type = 'button';
    b.dataset.item = item;
    b.setAttribute('role', 'menuitem');
    b.textContent = LABELS[item];
    b.addEventListener('click', () => {
      this.close();
      if (item === 'whisper') {
        this.actions.whisper(info.name);
      } else if (item === 'trade') {
        this.actions.trade(info.id);
      } else if (item === 'browseShop') {
        this.actions.browseShop(info.id);
      } else if (item === 'browseStore') {
        this.actions.browseStore(info.id);
      } else if (item === 'partyInvite') {
        this.actions.partyInvite(info.name);
      } else {
        this.actions.guildInvite(info.name);
      }
    });
    return b;
  }
}

// What the admin screens share: the API, the /me answer, the notice
// line and the navigation between screens.

import type { Me } from '../gen/wathena/admin/v1/admin_pb';
import type { AdminAPI, Result } from './api';

/** A screen of the admin page, with what it opens on. */
export type View =
  | { kind: 'staff' }
  | { kind: 'audit' }
  | { kind: 'reviews' }
  | { kind: 'players' }
  | { kind: 'character'; id: bigint }
  /** characterId is the character the history was opened from, or 0. */
  | { kind: 'item'; serial: string; characterId: bigint }
  | { kind: 'restore'; characterId: bigint; serials: readonly string[] }
  | { kind: 'reports' }
  | { kind: 'report'; id: bigint }
  | { kind: 'bans' };

export interface Context {
  readonly api: AdminAPI;
  readonly me: Me;
  /** Whether the caller's role has a permission. */
  can(permission: string): boolean;
  /** Runs a call from an event, and shows a failure that throws. */
  run(p: Promise<void>): void;
  /** Shows a call's error, with the reauth link when it needs one. */
  fail(r: Result<unknown>): void;
  /** Shows a line in the notice. */
  say(text: string): void;
  /** Opens a screen. */
  open(view: View): void;
}

/** A tab of the navigation. */
export interface Tab {
  kind: 'staff' | 'audit' | 'reviews' | 'players' | 'item' | 'restore' | 'reports' | 'bans';
  label: string;
  /** The permission that shows the tab. */
  permission: string;
}

export const TABS: readonly Tab[] = [
  { kind: 'staff', label: 'Staff', permission: 'staff.assign' },
  { kind: 'audit', label: 'Audit log', permission: 'audit.read' },
  { kind: 'reviews', label: 'Review queue', permission: 'audit.read' },
  { kind: 'players', label: 'Players', permission: 'players.read' },
  { kind: 'item', label: 'Item history', permission: 'items.history' },
  { kind: 'restore', label: 'Restore', permission: 'items.restore' },
  { kind: 'reports', label: 'Reports', permission: 'moderation.reports' },
  { kind: 'bans', label: 'Bans', permission: 'moderation.ban' },
];

/** The tabs a role's permissions show, in order. */
export function tabsFor(permissions: readonly string[]): Tab[] {
  return TABS.filter((t) => permissions.includes(t.permission));
}

/** Whether an account name is the caller's, in any case. */
export function isSelf(me: Me, account: string): boolean {
  return account.toLowerCase() === me.account.toLowerCase();
}

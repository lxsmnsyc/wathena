// Calls to the admin API (decision 0028). Messages are the protobuf
// JSON form of wathena.admin.v1. Each change sends the CSRF token that
// /me returned in the X-CSRF-Token header.

import {
  type DescMessage,
  type JsonValue,
  type MessageShape,
  create,
  fromJson,
  toJson,
} from '@bufbuild/protobuf';
import {
  AnnounceRequestSchema,
  type AnnounceResult,
  AnnounceResultSchema,
  type AuditEntry,
  AuditEntrySchema,
  type AuditPage,
  AuditPageSchema,
  type CharacterDetail,
  CharacterDetailSchema,
  type CharacterLogs,
  CharacterLogsSchema,
  CloseReportRequestSchema,
  ErrorSchema,
  EventActionSchema,
  type EventChange,
  EventChangeSchema,
  type EventInfo,
  EventInfoSchema,
  type EventList,
  EventListSchema,
  GiveRoleRequestSchema,
  type ItemHistory,
  ItemHistorySchema,
  type ModInfo,
  ModInfoSchema,
  type ModList,
  ModListSchema,
  ModSwitchRequestSchema,
  type PlayerSearch,
  PlayerSearchSchema,
  type ReportDetail,
  ReportDetailSchema,
  type ReportPage,
  ReportPageSchema,
  type ReportSummary,
  ReportSummarySchema,
  type RestartPlan,
  RestartPlanSchema,
  type RestartRequest,
  RestartRequestSchema,
  type RestoreRequest,
  RestoreRequestSchema,
  type RestoreResult,
  RestoreResultSchema,
  ReviewRequestSchema,
  type Sanction,
  type SanctionList,
  SanctionListSchema,
  type SanctionRequest,
  SanctionRequestSchema,
  SanctionSchema,
  type StaffList,
  StaffListSchema,
  type StaffMember,
  StaffMemberSchema,
  type WorldView,
  WorldViewSchema,
} from '../gen/wathena/admin/v1/admin_pb';
import { ADMIN_API } from './access';

export type Result<T> =
  | { ok: true; value: T }
  | { ok: false; code: string; message: string; reauthUrl: string };

/** The filters of the audit log viewer. Empty fields do not filter. */
export interface AuditFilter {
  account: string;
  action: string;
  target: string;
  /** Dates, such as 2026-10-12. until is the day after the last. */
  since: string;
  until: string;
  review: '' | 'pending' | 'reviewed';
  /** Only rows that need a review. */
  needsReview: boolean;
}

export const NO_FILTER: AuditFilter = {
  account: '',
  action: '',
  target: '',
  since: '',
  until: '',
  review: '',
  needsReview: false,
};

/** The filter of the review queue: rows that wait for a review. */
export const REVIEW_QUEUE: AuditFilter = { ...NO_FILTER, review: 'pending', needsReview: true };

/** The query of GET /audit for a filter and a page. */
export function auditQuery(filter: AuditFilter, beforeId: bigint, limit: number): string {
  const q = new URLSearchParams();
  for (const key of ['account', 'action', 'target', 'since', 'until', 'review'] as const) {
    const v = filter[key].trim();
    if (v !== '') {
      q.set(key, v);
    }
  }
  if (filter.needsReview) {
    q.set('needs_review', 'true');
  }
  if (beforeId > 0n) {
    q.set('before_id', beforeId.toString());
  }
  q.set('limit', String(limit));
  return q.toString();
}

/** The kinds of player search. ip needs players.read_ip. */
export type SearchKind = 'name' | 'id' | 'serial' | 'ip';

export const SEARCH_KINDS: readonly SearchKind[] = ['name', 'id', 'serial', 'ip'];

/** Rows per page of the player search, from 1 to 50. */
export const PLAYER_PAGE = 20;

/** The query of GET /players for a search and a page. */
export function playersQuery(
  kind: SearchKind,
  value: string,
  cursor: string,
  limit: number,
): string {
  const q = new URLSearchParams();
  q.set(kind, value.trim());
  if (cursor !== '') {
    q.set('cursor', cursor);
  }
  q.set('limit', String(limit));
  return q.toString();
}

/** The dates of a character's log. Empty fields do not filter. */
export interface LogRange {
  /** Dates, such as 2026-10-12. until is the day after the last. */
  since: string;
  until: string;
}

/** Rows per page of a character's log, from 1 to 100. */
export const LOG_PAGE = 50;

/** The query of GET /characters/{id}/logs for a range and a page. */
export function logsQuery(range: LogRange, cursor: string, limit: number): string {
  const q = new URLSearchParams();
  if (cursor !== '') {
    q.set('cursor', cursor);
  }
  q.set('limit', String(limit));
  for (const key of ['since', 'until'] as const) {
    const v = range[key].trim();
    if (v !== '') {
      q.set(key, v);
    }
  }
  return q.toString();
}

/** The kinds of report. */
export type ReportKind = 'player' | 'line' | 'guild_name' | 'guild_emblem';

export const REPORT_KINDS: readonly ReportKind[] = ['player', 'line', 'guild_name', 'guild_emblem'];

/** The filters of the report queue. Empty fields do not filter. */
export interface ReportFilter {
  /** The server shows open reports for an empty status. */
  status: '' | 'open' | 'closed' | 'all';
  kind: '' | ReportKind;
  /** A character or guild name, in any case. */
  target: string;
}

/** The filter of the report queue when it opens: open reports. */
export const OPEN_REPORTS: ReportFilter = { status: 'open', kind: '', target: '' };

/** Rows per page of the report queue, from 1 to 100. */
export const REPORT_PAGE = 50;

/** The query of GET /reports for a filter and a page. */
export function reportsQuery(filter: ReportFilter, beforeId: bigint, limit: number): string {
  const q = new URLSearchParams();
  for (const key of ['status', 'target', 'kind'] as const) {
    const v = filter[key].trim();
    if (v !== '') {
      q.set(key, v);
    }
  }
  if (beforeId > 0n) {
    q.set('before_id', beforeId.toString());
  }
  q.set('limit', String(limit));
  return q.toString();
}

/** The outcomes that close a report. */
export type Outcome = 'no_action' | 'warned' | 'sanctioned';

/** The kinds of sanction. */
export type SanctionKind = 'mute' | 'jail' | 'kick' | 'ban';

export const SANCTION_KINDS: readonly SanctionKind[] = ['mute', 'jail', 'kick', 'ban'];

/** The filters of GET /sanctions. Unset fields do not filter. */
export interface SanctionFilter {
  /** A character: its sanctions and its account's bans. */
  character?: bigint;
  /** An account: every sanction of the account. */
  account?: bigint;
  kind?: SanctionKind;
  /** Only the sanctions in effect. */
  active?: boolean;
}

/** The filter of the list of bans in effect. */
export const ACTIVE_BANS: SanctionFilter = { kind: 'ban', active: true };

/** Rows per page of a sanction list, from 1 to 100. */
export const SANCTION_PAGE = 50;

/** The query of GET /sanctions for a filter and a page. */
export function sanctionsQuery(filter: SanctionFilter, beforeId: bigint, limit: number): string {
  const q = new URLSearchParams();
  if (filter.character !== undefined && filter.character > 0n) {
    q.set('character', filter.character.toString());
  }
  if (filter.account !== undefined && filter.account > 0n) {
    q.set('account', filter.account.toString());
  }
  if (filter.kind !== undefined) {
    q.set('kind', filter.kind);
  }
  if (filter.active === true) {
    q.set('active', 'true');
  }
  if (beforeId > 0n) {
    q.set('before_id', beforeId.toString());
  }
  q.set('limit', String(limit));
  return q.toString();
}

/**
 * Reports whether a parsed JSON answer is a JSON value. JSON.parse only
 * makes JSON values, so a shallow check is enough.
 */
export function isJson(v: unknown): v is JsonValue {
  return (
    v === null ||
    typeof v === 'string' ||
    typeof v === 'number' ||
    typeof v === 'boolean' ||
    typeof v === 'object'
  );
}

/** A failed call's result. */
function failure(code: string, message: string, reauthUrl = ''): Result<never> {
  return { ok: false, code, message, reauthUrl };
}

/** Reads an answer's JSON as a message, or fails. */
function decode<D extends DescMessage>(schema: D, r: Result<JsonValue>): Result<MessageShape<D>> {
  if (!r.ok) {
    return r;
  }
  try {
    return { ok: true, value: fromJson(schema, r.value, { ignoreUnknownFields: true }) };
  } catch {
    return failure('unavailable', 'The server sent an answer this page cannot read.');
  }
}

/** The admin API for one session. */
export class AdminAPI {
  constructor(
    private readonly csrf: string,
    private readonly fetcher: typeof fetch = async (input, init) => fetch(input, init),
  ) {}

  /** Calls the API and returns the answer's JSON, null for none. */
  private async call(method: string, path: string, body?: JsonValue): Promise<Result<JsonValue>> {
    const headers: Record<string, string> = {};
    if (method !== 'GET') {
      headers['X-CSRF-Token'] = this.csrf;
    }
    if (body !== undefined) {
      headers['Content-Type'] = 'application/json';
    }
    let res: Response;
    try {
      res = await this.fetcher(ADMIN_API + path, {
        method,
        headers,
        credentials: 'same-origin',
        cache: 'no-store',
        ...(body === undefined ? {} : { body: JSON.stringify(body) }),
      });
    } catch {
      return failure('unavailable', 'Cannot reach the server.');
    }
    const read: unknown = res.status === 204 ? null : await res.json().catch(() => null);
    const json = isJson(read) ? read : null;
    if (res.ok) {
      return { ok: true, value: json };
    }
    const fallback = `The server answered ${String(res.status)}.`;
    try {
      const e = fromJson(ErrorSchema, json ?? {}, { ignoreUnknownFields: true });
      return failure(e.code || 'unavailable', e.message || fallback, e.reauthUrl);
    } catch {
      return failure('unavailable', fallback);
    }
  }

  /** GET /staff. */
  async staff(): Promise<Result<StaffList>> {
    return decode(StaffListSchema, await this.call('GET', 'staff'));
  }

  /** POST /staff: gives an account a role. */
  async give(account: string, role: string, reason: string): Promise<Result<StaffMember>> {
    const req = create(GiveRoleRequestSchema, { account, role, reason });
    return decode(
      StaffMemberSchema,
      await this.call('POST', 'staff', toJson(GiveRoleRequestSchema, req)),
    );
  }

  /** DELETE /staff/{account}: takes an account's role away. */
  async remove(account: string, reason: string): Promise<Result<null>> {
    const q = reason.trim() === '' ? '' : `?${new URLSearchParams({ reason }).toString()}`;
    const r = await this.call('DELETE', `staff/${encodeURIComponent(account)}${q}`);
    return r.ok ? { ok: true, value: null } : r;
  }

  /** GET /audit: a page of the audit log, newest first. */
  async audit(filter: AuditFilter, beforeId: bigint, limit: number): Promise<Result<AuditPage>> {
    return decode(
      AuditPageSchema,
      await this.call('GET', `audit?${auditQuery(filter, beforeId, limit)}`),
    );
  }

  /** POST /audit/{id}/review: records a verdict of ok or flag. */
  async review(id: bigint, verdict: 'ok' | 'flag', note: string): Promise<Result<AuditEntry>> {
    const req = create(ReviewRequestSchema, { verdict, note });
    return decode(
      AuditEntrySchema,
      await this.call('POST', `audit/${id.toString()}/review`, toJson(ReviewRequestSchema, req)),
    );
  }

  /** GET /players: accounts and their characters. */
  async players(
    kind: SearchKind,
    value: string,
    cursor: string,
    limit: number,
  ): Promise<Result<PlayerSearch>> {
    return decode(
      PlayerSearchSchema,
      await this.call('GET', `players?${playersQuery(kind, value, cursor, limit)}`),
    );
  }

  /** GET /characters/{id}: one character in full. */
  async character(id: bigint): Promise<Result<CharacterDetail>> {
    return decode(CharacterDetailSchema, await this.call('GET', `characters/${id.toString()}`));
  }

  /** GET /characters/{id}/logs: a page of a character's log, newest first. */
  async logs(
    id: bigint,
    range: LogRange,
    cursor: string,
    limit: number,
  ): Promise<Result<CharacterLogs>> {
    return decode(
      CharacterLogsSchema,
      await this.call('GET', `characters/${id.toString()}/logs?${logsQuery(range, cursor, limit)}`),
    );
  }

  /** GET /items/{serial}/history: every move of a serial, oldest first. */
  async history(serial: string): Promise<Result<ItemHistory>> {
    return decode(
      ItemHistorySchema,
      await this.call('GET', `items/${encodeURIComponent(serial.trim())}/history`),
    );
  }

  /** POST /restore: gives items or zeny back by mail. */
  async restore(req: RestoreRequest): Promise<Result<RestoreResult>> {
    return decode(
      RestoreResultSchema,
      await this.call('POST', 'restore', toJson(RestoreRequestSchema, req)),
    );
  }

  /** GET /reports: a page of the report queue, newest first. */
  async reports(
    filter: ReportFilter,
    beforeId: bigint,
    limit: number,
  ): Promise<Result<ReportPage>> {
    return decode(
      ReportPageSchema,
      await this.call('GET', `reports?${reportsQuery(filter, beforeId, limit)}`),
    );
  }

  /** GET /reports/{id}: one report with its chat lines and history. */
  async report(id: bigint): Promise<Result<ReportDetail>> {
    return decode(ReportDetailSchema, await this.call('GET', `reports/${id.toString()}`));
  }

  /**
   * POST /reports/{id}/close: closes a report. sanctionId links the
   * sanction of a sanctioned report, and is 0 otherwise.
   */
  async closeReport(
    id: bigint,
    outcome: Outcome,
    note: string,
    sanctionId: bigint,
  ): Promise<Result<ReportSummary>> {
    const req = create(CloseReportRequestSchema, { outcome, note, sanctionId });
    return decode(
      ReportSummarySchema,
      await this.call(
        'POST',
        `reports/${id.toString()}/close`,
        toJson(CloseReportRequestSchema, req),
      ),
    );
  }

  /** POST /sanctions: gives a mute, a jail, a kick or a ban. */
  async sanction(req: SanctionRequest): Promise<Result<Sanction>> {
    return decode(
      SanctionSchema,
      await this.call('POST', 'sanctions', toJson(SanctionRequestSchema, req)),
    );
  }

  /** DELETE /sanctions/{id}: lifts a mute, a jail or a ban early. */
  async lift(id: bigint, reason: string): Promise<Result<Sanction>> {
    const q = new URLSearchParams({ reason: reason.trim() }).toString();
    return decode(SanctionSchema, await this.call('DELETE', `sanctions/${id.toString()}?${q}`));
  }

  /** GET /sanctions: a page of sanctions, newest first. */
  async sanctions(
    filter: SanctionFilter,
    beforeId: bigint,
    limit: number,
  ): Promise<Result<SanctionList>> {
    return decode(
      SanctionListSchema,
      await this.call('GET', `sanctions?${sanctionsQuery(filter, beforeId, limit)}`),
    );
  }

  /** GET /world: the zones, the process, the players and the restart. */
  async world(): Promise<Result<WorldView>> {
    return decode(WorldViewSchema, await this.call('GET', 'world'));
  }

  /** POST /announce: a line to the world, or to one map's players. */
  async announce(text: string, map: string): Promise<Result<AnnounceResult>> {
    const req = create(AnnounceRequestSchema, { text, map });
    return decode(
      AnnounceResultSchema,
      await this.call('POST', 'announce', toJson(AnnounceRequestSchema, req)),
    );
  }

  /** POST /restart: plans a restart. */
  async planRestart(req: RestartRequest): Promise<Result<RestartPlan>> {
    return decode(
      RestartPlanSchema,
      await this.call('POST', 'restart', toJson(RestartRequestSchema, req)),
    );
  }

  /** DELETE /restart: cancels the planned restart. */
  async cancelRestart(reason: string): Promise<Result<RestartPlan>> {
    const q =
      reason.trim() === '' ? '' : `?${new URLSearchParams({ reason: reason.trim() }).toString()}`;
    return decode(RestartPlanSchema, await this.call('DELETE', `restart${q}`));
  }

  /** GET /events: the world events. */
  async events(): Promise<Result<EventList>> {
    return decode(EventListSchema, await this.call('GET', 'events'));
  }

  /** POST /events/{name}/start, /stop or /reset. */
  async eventAction(
    name: string,
    action: 'start' | 'stop' | 'reset',
    reason: string,
  ): Promise<Result<EventInfo>> {
    const req = create(EventActionSchema, { reason: reason.trim() });
    return decode(
      EventInfoSchema,
      await this.call(
        'POST',
        `events/${encodeURIComponent(name)}/${action}`,
        toJson(EventActionSchema, req),
      ),
    );
  }

  /** PATCH /events/{name}: whether it is on, its trigger and duration. */
  async changeEvent(name: string, change: EventChange): Promise<Result<EventInfo>> {
    return decode(
      EventInfoSchema,
      await this.call(
        'PATCH',
        `events/${encodeURIComponent(name)}`,
        toJson(EventChangeSchema, change),
      ),
    );
  }

  /** GET /mods: the loaded mods with their errors. */
  async mods(): Promise<Result<ModList>> {
    return decode(ModListSchema, await this.call('GET', 'mods'));
  }

  /**
   * POST /mods/{id}/disable or /enable, in one zone, or in every zone
   * when map is empty.
   */
  async switchMod(id: string, off: boolean, map: string, reason: string): Promise<Result<ModInfo>> {
    const req = create(ModSwitchRequestSchema, { map, reason: reason.trim() });
    return decode(
      ModInfoSchema,
      await this.call(
        'POST',
        `mods/${encodeURIComponent(id)}/${off ? 'disable' : 'enable'}`,
        toJson(ModSwitchRequestSchema, req),
      ),
    );
  }
}

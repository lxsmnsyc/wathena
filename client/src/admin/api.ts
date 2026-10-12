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
  type AuditPage,
  AuditPageSchema,
  ErrorSchema,
  GiveRoleRequestSchema,
  type StaffList,
  StaffListSchema,
  type StaffMember,
  StaffMemberSchema,
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
}

export const NO_FILTER: AuditFilter = {
  account: '',
  action: '',
  target: '',
  since: '',
  until: '',
  review: '',
};

/** The query of GET /audit for a filter and a page. */
export function auditQuery(filter: AuditFilter, beforeId: bigint, limit: number): string {
  const q = new URLSearchParams();
  for (const key of ['account', 'action', 'target', 'since', 'until', 'review'] as const) {
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
}

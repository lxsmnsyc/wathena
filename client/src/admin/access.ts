// The check that the admin page runs before it loads the admin screens
// (decision 0028). It asks the admin API's /me who the session is. The
// admin bundle loads only when /me returns a role, so players never
// download it. This file has no protobuf code, so the page stays small.

/** Where the admin API lives on the account service's origin. */
export const ADMIN_API = '/admin/api/v1/';

export type Access =
  /** A staff role counts: load the admin screens with this /me answer. */
  | { kind: 'staff'; me: unknown }
  /** Not logged in. */
  | { kind: 'login'; url: string }
  /** A role that needs two-factor login first. */
  | { kind: 'two_factor'; role: string; url: string }
  /** Logged in, without a role. */
  | { kind: 'none'; account: string }
  | { kind: 'error'; message: string };

/** A string field of a JSON object, or "". */
function field(body: unknown, name: string): string {
  if (typeof body !== 'object' || body === null || !(name in body)) {
    return '';
  }
  const v: unknown = Reflect.get(body, name);
  return typeof v === 'string' ? v : '';
}

/** Asks /me and says what the page shows. */
export async function checkAccess(fetcher: typeof fetch = fetch): Promise<Access> {
  let res: Response;
  try {
    res = await fetcher(`${ADMIN_API}me`, { credentials: 'same-origin', cache: 'no-store' });
  } catch {
    return { kind: 'error', message: 'Cannot reach the server.' };
  }
  const json = res.headers.get('Content-Type')?.startsWith('application/json') === true;
  const body: unknown = json ? await res.json().catch(() => null) : null;
  if (res.status === 401) {
    return { kind: 'login', url: '/account/login' };
  }
  if (res.status === 429) {
    return { kind: 'error', message: 'Too many requests. Wait a moment and reload the page.' };
  }
  if (!res.ok || body === null) {
    return { kind: 'error', message: 'The admin tools are not available on this server.' };
  }
  const role = field(body, 'role');
  if (role !== '') {
    return { kind: 'staff', me: body };
  }
  const assigned = field(body, 'assignedRole');
  if (assigned !== '') {
    return { kind: 'two_factor', role: assigned, url: '/account/settings' };
  }
  return { kind: 'none', account: field(body, 'account') };
}

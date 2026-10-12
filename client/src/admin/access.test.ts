import { describe, expect, it } from 'vitest';
import { checkAccess } from './access';

function urlOf(input: RequestInfo | URL): string {
  if (typeof input === 'string') {
    return input;
  }
  return input instanceof URL ? input.href : input.url;
}

function answer(status: number, body: unknown, type = 'application/json'): typeof fetch {
  return async (input: RequestInfo | URL, init?: RequestInit) => {
    expect(urlOf(input)).toBe('/admin/api/v1/me');
    expect(init?.credentials).toBe('same-origin');
    // /me is a GET without a body.
    expect(await new Response(init?.body ?? null).text()).toBe('');
    return new Response(typeof body === 'string' ? body : JSON.stringify(body), {
      status,
      headers: { 'Content-Type': type },
    });
  };
}

describe('checkAccess', () => {
  it('loads the admin screens only for a role that counts', async () => {
    const me = { account: 'Owner', role: 'owner', permissions: ['staff.assign'], csrfToken: 't' };
    expect(await checkAccess(answer(200, me))).toEqual({ kind: 'staff', me });
  });

  it('says a role needs two-factor login', async () => {
    expect(await checkAccess(answer(200, { account: 'Gm', assignedRole: 'game_master' }))).toEqual({
      kind: 'two_factor',
      role: 'game_master',
      url: '/account/settings',
    });
  });

  it('turns players away', async () => {
    expect(await checkAccess(answer(200, { account: 'Player', csrfToken: 't' }))).toEqual({
      kind: 'none',
      account: 'Player',
    });
  });

  it('sends a visitor without a session to the login page', async () => {
    expect(await checkAccess(answer(401, { code: 'login' }))).toEqual({
      kind: 'login',
      url: '/account/login',
    });
  });

  it('reports a server without the admin API', async () => {
    const got = await checkAccess(answer(404, '<!doctype html>', 'text/html'));
    expect(got.kind).toBe('error');
    expect((await checkAccess(answer(429, { code: 'rate' }))).kind).toBe('error');
    const down: typeof fetch = async () => {
      await Promise.resolve();
      throw new TypeError('offline');
    };
    expect(await checkAccess(down)).toEqual({ kind: 'error', message: 'Cannot reach the server.' });
  });
});

import { describe, expect, it } from 'vitest';
import { AdminAPI, NO_FILTER, auditQuery } from './api';

function urlOf(input: RequestInfo | URL): string {
  if (typeof input === 'string') {
    return input;
  }
  return input instanceof URL ? input.href : input.url;
}

interface Seen {
  url: string;
  method: string;
  headers: Record<string, string>;
  body: string;
}

function server(status: number, body: unknown, seen: Seen[]): typeof fetch {
  return async (input: RequestInfo | URL, init?: RequestInit) => {
    seen.push({
      url: urlOf(input),
      method: init?.method ?? 'GET',
      headers: (init?.headers ?? {}) as Record<string, string>,
      body: await new Response(init?.body ?? null).text(),
    });
    if (status === 204) {
      return new Response(null, { status });
    }
    return new Response(JSON.stringify(body), {
      status,
      headers: { 'Content-Type': 'application/json' },
    });
  };
}

describe('auditQuery', () => {
  it('leaves out empty filters', () => {
    expect(auditQuery(NO_FILTER, 0n, 50)).toBe('limit=50');
    expect(
      auditQuery(
        { ...NO_FILTER, account: ' Gm ', review: 'pending', since: '2026-10-01' },
        42n,
        10,
      ),
    ).toBe('account=Gm&since=2026-10-01&review=pending&before_id=42&limit=10');
  });
});

describe('AdminAPI', () => {
  it('sends the CSRF token and JSON with a change', async () => {
    const seen: Seen[] = [];
    const api = new AdminAPI('tok', server(200, { account: 'Bobby', role: 'moderator' }, seen));
    const r = await api.give('Bobby', 'moderator', 'helps');
    expect(r.ok && r.value.account).toBe('Bobby');
    expect(seen[0]).toEqual({
      url: '/admin/api/v1/staff',
      method: 'POST',
      headers: { 'X-CSRF-Token': 'tok', 'Content-Type': 'application/json' },
      body: '{"account":"Bobby","role":"moderator","reason":"helps"}',
    });
  });

  it('removes with the reason in the query', async () => {
    const seen: Seen[] = [];
    const api = new AdminAPI('tok', server(204, null, seen));
    expect((await api.remove('Bob by', 'left')).ok).toBe(true);
    expect(seen[0]?.url).toBe('/admin/api/v1/staff/Bob%20by?reason=left');
    expect(seen[0]?.method).toBe('DELETE');
  });

  it('sends no token with a read, and reads int64 IDs', async () => {
    const seen: Seen[] = [];
    const page = {
      entries: [{ id: '7', action: 'staff.give', arguments: { role: 'admin' } }],
      nextBeforeId: '7',
    };
    const r = await new AdminAPI('tok', server(200, page, seen)).audit(NO_FILTER, 0n, 1);
    expect(seen[0]?.headers).toEqual({});
    expect(r.ok && r.value.entries[0]?.id).toBe(7n);
    expect(r.ok && r.value.nextBeforeId).toBe(7n);
  });

  it('passes on the error code and the reauth link', async () => {
    const api = new AdminAPI(
      'tok',
      server(
        403,
        {
          code: 'reauth',
          message: 'Confirm it is you.',
          reauthUrl: '/account/reauth?next=%2Fadmin',
        },
        [],
      ),
    );
    expect(await api.give('Bobby', 'admin', '')).toEqual({
      ok: false,
      code: 'reauth',
      message: 'Confirm it is you.',
      reauthUrl: '/account/reauth?next=%2Fadmin',
    });
    const broken = await new AdminAPI('tok', server(500, 'nope', [])).staff();
    expect(broken.ok).toBe(false);
  });
});

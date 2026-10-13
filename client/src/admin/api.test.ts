import { describe, expect, it } from 'vitest';
import { create } from '@bufbuild/protobuf';
import { RestoreRequestSchema } from '../gen/wathena/admin/v1/admin_pb';
import { AdminAPI, NO_FILTER, REVIEW_QUEUE, auditQuery, logsQuery, playersQuery } from './api';

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

describe('review queue query', () => {
  it('asks for pending rows that need a review', () => {
    expect(auditQuery(REVIEW_QUEUE, 0n, 50)).toBe('review=pending&needs_review=true&limit=50');
  });
});

describe('playersQuery', () => {
  it('sends one search kind, the cursor and the limit', () => {
    expect(playersQuery('name', ' Bob ', '', 20)).toBe('name=Bob&limit=20');
    expect(playersQuery('ip', '203.0.113.0/24', 'abc=', 5)).toBe(
      'ip=203.0.113.0%2F24&cursor=abc%3D&limit=5',
    );
    expect(playersQuery('serial', 'a-b', '', 20)).toBe('serial=a-b&limit=20');
  });
});

describe('logsQuery', () => {
  it('leaves out empty dates and an empty cursor', () => {
    expect(logsQuery({ since: '', until: '' }, '', 50)).toBe('limit=50');
    expect(logsQuery({ since: '2026-10-01', until: ' 2026-10-12 ' }, 'next', 10)).toBe(
      'cursor=next&limit=10&since=2026-10-01&until=2026-10-12',
    );
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

  it('reads the player screens from their paths without a token', async () => {
    const seen: Seen[] = [];
    const api = new AdminAPI('tok', server(200, {}, seen));
    await api.players('id', '150000', 'c', 20);
    await api.character(150000n);
    await api.logs(150000n, { since: '2026-10-01', until: '' }, '', 50);
    await api.history('0198a3c4-0000-7000-8000-000000000001');
    expect(seen.map((s) => s.url)).toEqual([
      '/admin/api/v1/players?id=150000&cursor=c&limit=20',
      '/admin/api/v1/characters/150000',
      '/admin/api/v1/characters/150000/logs?limit=50&since=2026-10-01',
      '/admin/api/v1/items/0198a3c4-0000-7000-8000-000000000001/history',
    ]);
    expect(seen.every((s) => s.method === 'GET' && Object.keys(s.headers).length === 0)).toBe(true);
  });

  it('reads a search answer with its holder and cursor', async () => {
    const answer = {
      accounts: [
        {
          accountId: '2000001',
          account: 'bob',
          characters: [{ id: '150000', name: 'Bob', baseLevel: 99, online: true }],
        },
      ],
      nextCursor: 'more',
      holder: { kind: 'inventory', id: '150000', name: 'Bob' },
    };
    const r = await new AdminAPI('tok', server(200, answer, [])).players('serial', 'x', '', 20);
    expect(r.ok && r.value.accounts[0]?.characters[0]?.id).toBe(150000n);
    expect(r.ok && r.value.nextCursor).toBe('more');
    expect(r.ok && r.value.holder?.name).toBe('Bob');
  });

  it('posts a review with the CSRF token', async () => {
    const seen: Seen[] = [];
    const api = new AdminAPI('tok', server(200, { id: '9', verdict: 'flag' }, seen));
    const r = await api.review(9n, 'flag', 'too much zeny');
    expect(r.ok && r.value.verdict).toBe('flag');
    expect(seen[0]).toEqual({
      url: '/admin/api/v1/audit/9/review',
      method: 'POST',
      headers: { 'X-CSRF-Token': 'tok', 'Content-Type': 'application/json' },
      body: '{"verdict":"flag","note":"too much zeny"}',
    });
  });

  it('posts a restore with int64 values as strings', async () => {
    const seen: Seen[] = [];
    const result = {
      mailId: '77',
      items: [{ serial: 's1', newSerial: 's2', copy: true, itemName: 'Knife', amount: 1 }],
    };
    const api = new AdminAPI('tok', server(200, result, seen));
    const req = create(RestoreRequestSchema, {
      characterId: 150000n,
      items: [{ serial: 's1', amount: 1 }],
      zeny: 500n,
      reason: 'lost in a crash',
    });
    const r = await api.restore(req);
    expect(r.ok && r.value.mailId).toBe(77n);
    expect(r.ok && r.value.items[0]?.copy).toBe(true);
    expect(seen[0]?.url).toBe('/admin/api/v1/restore');
    expect(seen[0]?.headers['X-CSRF-Token']).toBe('tok');
    expect(JSON.parse(seen[0]?.body ?? '')).toEqual({
      characterId: '150000',
      items: [{ serial: 's1', amount: 1 }],
      zeny: '500',
      reason: 'lost in a crash',
    });
  });

  it('passes on a restore conflict and a reauth error', async () => {
    const req = create(RestoreRequestSchema, { characterId: 1n, reason: 'r' });
    const moving = await new AdminAPI(
      'tok',
      server(409, { code: 'conflict', message: 'The item is moving. Try again.' }, []),
    ).restore(req);
    expect(moving).toEqual({
      ok: false,
      code: 'conflict',
      message: 'The item is moving. Try again.',
      reauthUrl: '',
    });
    const stale = await new AdminAPI(
      'tok',
      server(403, { code: 'reauth', message: 'Log in again.', reauthUrl: '/account/reauth' }, []),
    ).restore(req);
    expect(!stale.ok && stale.reauthUrl).toBe('/account/reauth');
  });
});

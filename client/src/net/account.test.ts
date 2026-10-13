import { describe, expect, it } from 'vitest';
import { accountPage, accountURL, fetchTicket } from './account';

describe('accountURL', () => {
  it('comes from the game server when not given', () => {
    expect(accountURL('ws://localhost:8300/ws', null)).toBe('http://localhost:8300/account/');
    expect(accountURL('wss://play.example.com/ws', null)).toBe('https://play.example.com/account/');
  });

  it('takes ?account= and ends it in a slash', () => {
    expect(accountURL('ws://a/ws', 'https://acct.example.com/account')).toBe(
      'https://acct.example.com/account/',
    );
  });
});

describe('accountPage', () => {
  it('carries the way back', () => {
    expect(accountPage('http://h/account/', 'login', 'http://c/?server=ws://h/ws')).toBe(
      'http://h/account/login?return=http%3A%2F%2Fc%2F%3Fserver%3Dws%3A%2F%2Fh%2Fws',
    );
  });
});

function answer(status: number, body: unknown = {}): typeof fetch {
  return async (input: RequestInfo | URL, init?: RequestInit) => {
    const request = new Request(input, init);
    expect(request.url).toBe('http://h/account/ticket');
    expect(request.method).toBe('POST');
    expect(request.credentials).toBe('include');
    // The ticket request has no body: the cookie is the credential.
    expect(await request.text()).toBe('');
    return new Response(JSON.stringify(body), { status });
  };
}

describe('fetchTicket', () => {
  const account = 'http://h/account/';
  const back = 'http://c/';

  it('returns the ticket', async () => {
    expect(await fetchTicket(account, back, answer(200, { ticket: 'T' }))).toEqual({
      kind: 'ticket',
      ticket: 'T',
    });
  });

  it('sends a player who is not logged in to the login page', async () => {
    expect(await fetchTicket(account, back, answer(401))).toEqual({
      kind: 'redirect',
      url: 'http://h/account/login?return=http%3A%2F%2Fc%2F',
    });
  });

  it('sends a player without a chosen character to the character list', async () => {
    expect(await fetchTicket(account, back, answer(409))).toEqual({
      kind: 'redirect',
      url: 'http://h/account/characters?return=http%3A%2F%2Fc%2F',
    });
  });

  it('shows a banned account its ban message', async () => {
    const message = 'Your account is banned until 2026-10-20 15:04 UTC. Reason: spam';
    expect(
      await fetchTicket(
        account,
        back,
        answer(403, {
          error: 'banned',
          reason: 'spam',
          ends_at: '2026-10-20T15:04:00Z',
          message,
        }),
      ),
    ).toEqual({ kind: 'error', message });
    expect((await fetchTicket(account, back, answer(403, {}))).kind).toBe('error');
  });

  it('reports other failures', async () => {
    expect((await fetchTicket(account, back, answer(503))).kind).toBe('error');
    expect((await fetchTicket(account, back, answer(200, {}))).kind).toBe('error');
    const offline: typeof fetch = async () => Promise.reject(new TypeError('network'));
    expect((await fetchTicket(account, back, offline)).kind).toBe('error');
  });
});

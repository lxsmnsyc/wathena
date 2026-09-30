// Getting a game ticket from the account service, as
// docs/decisions/0004-account-service-and-game-tickets.md describes.
//
// The player logs in on the account service's own pages, never in the
// client. The client asks the service for a one-time game ticket with
// fetch and the session cookie, then sends the ticket in Hello. The
// ticket never goes in a URL.

/**
 * The account service's URL, ending in "/". It comes from ?account=, or
 * from the game server's URL: ws://host/ws gives http://host/account/.
 */
export function accountURL(serverURL: string, param: string | null): string {
  if (param !== null && param !== '') {
    const u = new URL(param);
    return u.href.endsWith('/') ? u.href : `${u.href}/`;
  }
  const u = new URL(serverURL);
  u.protocol = u.protocol === 'wss:' ? 'https:' : 'http:';
  u.pathname = '/account/';
  u.search = '';
  u.hash = '';
  return u.href;
}

/** Where the player goes on the account service, with a way back here. */
export function accountPage(account: string, page: 'login' | 'characters', back: string): string {
  const u = new URL(page, account);
  u.searchParams.set('return', back);
  return u.href;
}

export type TicketResult =
  | { kind: 'ticket'; ticket: string }
  | { kind: 'redirect'; url: string }
  | { kind: 'error'; message: string };

/**
 * Asks the account service for a game ticket. A player who is not logged
 * in, or has not chosen a character, is sent to the right page, which
 * brings them back to back afterwards.
 */
export async function fetchTicket(
  account: string,
  back: string,
  fetcher: typeof fetch = fetch,
): Promise<TicketResult> {
  let res: Response;
  try {
    res = await fetcher(new URL('ticket', account), {
      method: 'POST',
      credentials: 'include',
      cache: 'no-store',
    });
  } catch {
    return { kind: 'error', message: 'Cannot reach the account service.' };
  }
  switch (res.status) {
    case 200: {
      const body: unknown = await res.json().catch(() => null);
      const ticket =
        typeof body === 'object' && body !== null && 'ticket' in body ? body.ticket : undefined;
      if (typeof ticket !== 'string' || ticket === '') {
        return { kind: 'error', message: 'The account service sent no ticket.' };
      }
      return { kind: 'ticket', ticket };
    }
    case 401:
      return { kind: 'redirect', url: accountPage(account, 'login', back) };
    case 409:
      return { kind: 'redirect', url: accountPage(account, 'characters', back) };
    case 429:
      return { kind: 'error', message: 'Too many tries. Wait a moment and reload the page.' };
    default:
      return { kind: 'error', message: 'The account service cannot give a ticket right now.' };
  }
}

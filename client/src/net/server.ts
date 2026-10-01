// Which game server the page plays on.

/**
 * The game server's WebSocket URL, or null for the offline map viewer:
 *
 * - ?server= names it, such as ws://localhost:8300/ws.
 * - Without it, a production build plays on /ws of the page's own
 *   origin, as `waserver all` serves the client, the account pages and
 *   the game on one origin. ?viewer opens the map viewer instead.
 * - Without it, a development build opens the map viewer.
 */
export default function gameServerURL(
  params: URLSearchParams,
  page: string,
  production: boolean,
): string | null {
  const given = params.get('server');
  if (given !== null && given !== '') {
    return given;
  }
  if (!production || params.has('viewer')) {
    return null;
  }
  const u = new URL('/ws', page);
  u.protocol = u.protocol === 'https:' ? 'wss:' : 'ws:';
  return u.href;
}

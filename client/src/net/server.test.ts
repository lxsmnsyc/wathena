import { describe, expect, it } from 'vitest';
import gameServerURL from './server';

function params(query: string): URLSearchParams {
  return new URLSearchParams(query);
}

describe('gameServerURL', () => {
  it('takes ?server= in every build', () => {
    expect(
      gameServerURL(params('server=ws://localhost:8300/ws'), 'http://localhost:5173/', false),
    ).toBe('ws://localhost:8300/ws');
    expect(gameServerURL(params('server=ws://h/ws'), 'https://game.example.com/', true)).toBe(
      'ws://h/ws',
    );
  });

  it('plays on its own origin in production builds', () => {
    expect(gameServerURL(params(''), 'https://game.example.com/?a=1', true)).toBe(
      'wss://game.example.com/ws',
    );
    expect(gameServerURL(params(''), 'http://localhost:8300/', true)).toBe(
      'ws://localhost:8300/ws',
    );
  });

  it('opens the map viewer in development builds and with ?viewer', () => {
    expect(gameServerURL(params(''), 'http://localhost:5173/', false)).toBeNull();
    expect(
      gameServerURL(params('viewer&map=izlude'), 'https://game.example.com/', true),
    ).toBeNull();
  });
});

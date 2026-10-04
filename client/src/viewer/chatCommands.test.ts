import { describe, expect, it } from 'vitest';
import { MAX_NAME, channelName, cleanName, parseChatInput } from './chatCommands';

describe('parseChatInput', () => {
  it('sends a plain line to the active tab', () => {
    expect(parseChatInput('  hello there ')).toEqual({ kind: 'say', text: 'hello there' });
    expect(parseChatInput(' \t ')).toEqual({ kind: 'empty' });
  });

  it('reads whispers, with quotes for names with spaces', () => {
    expect(parseChatInput('/w Bob hi there')).toEqual({
      kind: 'whisper',
      to: 'Bob',
      text: 'hi there',
    });
    expect(parseChatInput('/w "Big Bob" hi')).toEqual({
      kind: 'whisper',
      to: 'Big Bob',
      text: 'hi',
    });
    expect(parseChatInput('/W Bob')).toEqual({ kind: 'openWhisper', to: 'Bob' });
    expect(parseChatInput('/w').kind).toBe('error');
    expect(parseChatInput(`/w ${'a'.repeat(MAX_NAME + 1)} hi`).kind).toBe('error');
  });

  it('reads a whisper to #name as a channel line', () => {
    expect(parseChatInput('/w #trade selling')).toEqual({
      kind: 'channel',
      name: '#trade',
      text: 'selling',
    });
  });

  it('reads channel lines and channel tabs', () => {
    expect(parseChatInput('#trade S> Knife')).toEqual({
      kind: 'channel',
      name: '#trade',
      text: 'S> Knife',
    });
    expect(parseChatInput('#trade')).toEqual({ kind: 'openChannel', name: '#trade' });
    expect(parseChatInput('#').kind).toBe('error');
  });

  it('reads channel commands', () => {
    expect(parseChatInput('/join #trade')).toEqual({ kind: 'join', name: '#trade' });
    expect(parseChatInput('/join trade')).toEqual({ kind: 'join', name: '#trade' });
    expect(parseChatInput('/leave #map')).toEqual({ kind: 'leave', name: '#map' });
    expect(parseChatInput('/channels')).toEqual({ kind: 'list' });
    expect(parseChatInput('/members #global')).toEqual({ kind: 'members', name: '#global' });
    expect(parseChatInput('/join').kind).toBe('error');
    expect(parseChatInput('/join two words').kind).toBe('error');
  });

  it('reads block commands with the whole rest as the name', () => {
    expect(parseChatInput('/block Big Bob')).toEqual({ kind: 'block', name: 'Big Bob' });
    expect(parseChatInput('/block "Big Bob"')).toEqual({ kind: 'block', name: 'Big Bob' });
    expect(parseChatInput('/unblock Bob')).toEqual({ kind: 'unblock', name: 'Bob' });
    expect(parseChatInput('/block').kind).toBe('error');
  });

  it('reads reports with an optional reason', () => {
    expect(parseChatInput('/report Bob')).toEqual({ kind: 'report', name: 'Bob', reason: '' });
    expect(parseChatInput('/report Bob spam in trade')).toEqual({
      kind: 'report',
      name: 'Bob',
      reason: 'spam in trade',
    });
    expect(parseChatInput('/report "Big Bob" bot')).toEqual({
      kind: 'report',
      name: 'Big Bob',
      reason: 'bot',
    });
    expect(parseChatInput('/report').kind).toBe('error');
  });

  it('reads party lines with /p, /party and %', () => {
    expect(parseChatInput('/p hello all')).toEqual({ kind: 'party', text: 'hello all' });
    expect(parseChatInput('/P  hi ')).toEqual({ kind: 'party', text: 'hi' });
    expect(parseChatInput('/party hi')).toEqual({ kind: 'party', text: 'hi' });
    expect(parseChatInput('%hello')).toEqual({ kind: 'party', text: 'hello' });
    expect(parseChatInput('% hello')).toEqual({ kind: 'party', text: 'hello' });
    expect(parseChatInput('/p')).toEqual({ kind: 'openParty' });
    expect(parseChatInput('%')).toEqual({ kind: 'openParty' });
    // A line that only has % inside is a plain line.
    expect(parseChatInput('50% off')).toEqual({ kind: 'say', text: '50% off' });
    // /pm is not /p.
    expect(parseChatInput('/pm x').kind).toBe('error');
  });

  it('reads guild lines with /g, /guild and $', () => {
    expect(parseChatInput('/g hello all')).toEqual({ kind: 'guild', text: 'hello all' });
    expect(parseChatInput('/G  hi ')).toEqual({ kind: 'guild', text: 'hi' });
    expect(parseChatInput('/guild hi')).toEqual({ kind: 'guild', text: 'hi' });
    expect(parseChatInput('$hello')).toEqual({ kind: 'guild', text: 'hello' });
    expect(parseChatInput('$ hello')).toEqual({ kind: 'guild', text: 'hello' });
    expect(parseChatInput('/g')).toEqual({ kind: 'openGuild' });
    expect(parseChatInput('/guild')).toEqual({ kind: 'openGuild' });
    expect(parseChatInput('$')).toEqual({ kind: 'openGuild' });
    // A line that only has $ inside is a plain line.
    expect(parseChatInput('S> Knife 5$')).toEqual({ kind: 'say', text: 'S> Knife 5$' });
    // /gm is not /g.
    expect(parseChatInput('/gm x').kind).toBe('error');
  });

  it('names unknown commands', () => {
    const action = parseChatInput('/dance now');
    expect(action.kind).toBe('error');
    expect(action.kind === 'error' && action.message.startsWith('Unknown command /dance.')).toBe(
      true,
    );
  });

  it('cleans the text it sends', () => {
    expect(parseChatInput('/w Bob a‮b\tc')).toEqual({ kind: 'whisper', to: 'Bob', text: 'ab c' });
  });
});

describe('cleanName', () => {
  it('keeps names up to MAX_NAME code points', () => {
    expect(cleanName(' Bob ')).toBe('Bob');
    expect(cleanName('포'.repeat(MAX_NAME))).toBe('포'.repeat(MAX_NAME));
    expect(cleanName('a'.repeat(MAX_NAME + 1))).toBeUndefined();
    expect(cleanName('')).toBeUndefined();
  });
});

describe('channelName', () => {
  it('adds the # and refuses spaces', () => {
    expect(channelName('trade')).toBe('#trade');
    expect(channelName('#trade')).toBe('#trade');
    expect(channelName('#')).toBeUndefined();
    expect(channelName('a b')).toBeUndefined();
  });
});

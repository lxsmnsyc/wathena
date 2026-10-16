// The commands a player types in the chat input, such as "/w Name text",
// "/join #trade", "/p text", "/g text" or "/memo". A line that is not a command goes to the
// active chat tab. A line that starts with @ is a GM command for the
// server (decision 0030). It goes as a local chat line in any tab.

import { cleanChat } from './chat';

/** The most characters a character name may have, as Unicode code points. */
export const MAX_NAME = 24;

/** What a line typed in the chat input asks for. */
export type ChatAction =
  /** Nothing to send. */
  | { kind: 'empty' }
  /** A line for the active tab. */
  | { kind: 'say'; text: string }
  /** A GM command, such as "@hide", sent as a local chat line as typed. */
  | { kind: 'gm'; text: string }
  /** A whisper to a character. */
  | { kind: 'whisper'; to: string; text: string }
  /** Opens the whisper tab of a character. */
  | { kind: 'openWhisper'; to: string }
  /** A line in a chat channel. */
  | { kind: 'channel'; name: string; text: string }
  /** A line to the player's party. */
  | { kind: 'party'; text: string }
  /** Shows the party tab. */
  | { kind: 'openParty' }
  /** A line to the player's guild. */
  | { kind: 'guild'; text: string }
  /** Shows the guild tab. */
  | { kind: 'openGuild' }
  /** Shows the tab of a chat channel. */
  | { kind: 'openChannel'; name: string }
  | { kind: 'join'; name: string }
  | { kind: 'leave'; name: string }
  | { kind: 'list' }
  | { kind: 'members'; name: string }
  | { kind: 'block'; name: string }
  | { kind: 'unblock'; name: string }
  | { kind: 'report'; name: string; reason: string }
  /** Saves the player's cell as a Warp Portal memo point, as RO's /memo. */
  | { kind: 'memo' }
  /** A command the client cannot run, with the reason for the player. */
  | { kind: 'error'; message: string };

/**
 * Cleans a character name as chat is cleaned. Returns undefined for an
 * empty name or one longer than MAX_NAME code points.
 */
export function cleanName(text: string): string | undefined {
  const name = cleanChat(text);
  if (name === undefined) {
    return undefined;
  }
  // for...of walks code points, which is what the server counts.
  let count = 0;
  for (const _ of name) {
    count++;
  }
  return count > MAX_NAME ? undefined : name;
}

/**
 * A channel name with its #, as the server expects it. "trade" becomes
 * "#trade". Returns undefined for an empty name or one with spaces.
 */
export function channelName(text: string): string | undefined {
  const name = text.trim();
  if (name === '' || name === '#' || /\s/.test(name)) {
    return undefined;
  }
  return name.startsWith('#') ? name : `#${name}`;
}

/**
 * Splits a name from the start of rest. A name in double quotes may have
 * spaces. Otherwise the name ends at the first space.
 */
function splitName(rest: string): { name: string; after: string } {
  const text = rest.trimStart();
  if (text.startsWith('"')) {
    const end = text.indexOf('"', 1);
    if (end > 0) {
      return { name: text.slice(1, end), after: text.slice(end + 1) };
    }
    return { name: text.slice(1), after: '' };
  }
  const space = text.search(/\s/);
  if (space < 0) {
    return { name: text, after: '' };
  }
  return { name: text.slice(0, space), after: text.slice(space + 1) };
}

/** A whole-line name, such as for /block, with optional double quotes. */
function wholeName(rest: string): string {
  const text = rest.trim();
  if (text.length >= 2 && text.startsWith('"') && text.endsWith('"')) {
    return text.slice(1, -1);
  }
  return text;
}

function usage(text: string): ChatAction {
  return { kind: 'error', message: `Usage: ${text}` };
}

/** Reads a channel name, or says how to use the command. */
function channelCommand(
  kind: 'join' | 'leave' | 'members',
  rest: string,
  help: string,
): ChatAction {
  const name = channelName(rest);
  return name === undefined ? usage(help) : { kind, name };
}

/** Reads a name that the whole rest of the line holds. */
function nameCommand(kind: 'block' | 'unblock', rest: string, help: string): ChatAction {
  const name = cleanName(wholeName(rest));
  return name === undefined ? usage(help) : { kind, name };
}

/** Reads a line typed in the chat input. */
export function parseChatInput(input: string): ChatAction {
  const line = input.trim();
  if (line.startsWith('#')) {
    const space = line.search(/\s/);
    const name = channelName(space < 0 ? line : line.slice(0, space));
    if (name === undefined) {
      return usage('#channel text');
    }
    const text = space < 0 ? undefined : cleanChat(line.slice(space + 1));
    return text === undefined ? { kind: 'openChannel', name } : { kind: 'channel', name, text };
  }
  if (line.startsWith('@')) {
    // The server reads GM commands from local chat and answers on the
    // system channel, such as "Unknown command.". For a player without
    // any command, the server says the line as local chat.
    const text = cleanChat(input);
    return text === undefined || text === '@' ? { kind: 'empty' } : { kind: 'gm', text };
  }
  if (line.startsWith('%')) {
    // rAthena's party prefix: "%text" says text to the party.
    const text = cleanChat(line.slice(1));
    return text === undefined ? { kind: 'openParty' } : { kind: 'party', text };
  }
  if (line.startsWith('$')) {
    // RO's guild prefix: "$text" says text to the guild.
    const text = cleanChat(line.slice(1));
    return text === undefined ? { kind: 'openGuild' } : { kind: 'guild', text };
  }
  if (!line.startsWith('/')) {
    const text = cleanChat(input);
    return text === undefined ? { kind: 'empty' } : { kind: 'say', text };
  }
  const space = line.search(/\s/);
  const command = (space < 0 ? line : line.slice(0, space)).toLowerCase();
  const rest = space < 0 ? '' : line.slice(space + 1);
  switch (command) {
    case '/w':
    case '/whisper': {
      const { name: raw, after } = splitName(rest);
      if (raw.startsWith('#')) {
        // A whisper to #name is a line in that channel, as in rAthena.
        return parseChatInput(`${raw} ${after}`);
      }
      const to = cleanName(raw);
      if (to === undefined) {
        return usage('/w Name text, or /w "Name with spaces" text');
      }
      const text = cleanChat(after);
      return text === undefined ? { kind: 'openWhisper', to } : { kind: 'whisper', to, text };
    }
    case '/p':
    case '/party': {
      const text = cleanChat(rest);
      return text === undefined ? { kind: 'openParty' } : { kind: 'party', text };
    }
    case '/g':
    case '/guild': {
      const text = cleanChat(rest);
      return text === undefined ? { kind: 'openGuild' } : { kind: 'guild', text };
    }
    case '/join':
      return channelCommand('join', rest, '/join #channel');
    case '/leave':
      return channelCommand('leave', rest, '/leave #channel');
    case '/channels':
      return { kind: 'list' };
    case '/members':
      return channelCommand('members', rest, '/members #channel');
    case '/block':
      return nameCommand('block', rest, '/block Name');
    case '/unblock':
      return nameCommand('unblock', rest, '/unblock Name');
    case '/report': {
      const { name: raw, after } = splitName(rest);
      const name = cleanName(raw);
      if (name === undefined) {
        return usage('/report Name [reason]');
      }
      return { kind: 'report', name, reason: cleanChat(after) ?? '' };
    }
    case '/memo':
      // RO's /memo takes no arguments, and the server answers.
      return { kind: 'memo' };
    default:
      return {
        kind: 'error',
        message: `Unknown command ${command}. Commands: /w, /p, /g, /join, /leave, /channels, /members, /block, /unblock, /report, /memo.`,
      };
  }
}
